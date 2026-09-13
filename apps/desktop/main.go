// Command desktop is the rolle desktop app.
package main

import (
	"embed"
	"fmt"
	"log"
	"log/slog"
	"os"
	"runtime"
	"strings"
	"time"

	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"
	"github.com/wailsapp/wails/v3/pkg/services/notifications"

	"github.com/nateships/rolle/cmd/rolle/cli"
	"github.com/nateships/rolle/internal/app"
	"github.com/nateships/rolle/internal/awsconfig"
	"github.com/nateships/rolle/internal/core"
	"github.com/nateships/rolle/internal/debug"
)

// modTime returns a file's modification time, or the zero time when it is missing.
func modTime(path string) time.Time {
	fi, err := os.Stat(path)
	if err != nil {
		return time.Time{}
	}
	return fi.ModTime()
}

// assets holds the built frontend from frontend/dist.
//
//go:embed all:frontend/dist
var assets embed.FS

func init() {
	application.RegisterEvent[struct{}](EventWorkspaceChanged)
	application.RegisterEvent[struct{}](EventOpenSettings)
	application.RegisterEvent[StartRequest](EventStartNeedsLogin)
	application.RegisterEvent[UpdateInfo](EventUpdateAvailable)
}

func main() {
	// AWS profiles written by this app name this binary in credential_process.
	// Hand that call to the CLI instead of opening a window.
	if len(os.Args) > 1 && os.Args[1] == "creds" {
		if err := cli.Root().Execute(); err != nil {
			fmt.Fprintln(os.Stderr, "rolle:", err)
			os.Exit(1)
		}
		return
	}

	// ROLLE_DEMO=1 runs on fictional data in a temp directory: no real
	// workspace, keychain, or AWS config. For screenshots and UI work.
	newService := app.Default
	title := "rolle"
	if os.Getenv("ROLLE_DEMO") == "1" {
		newService = app.Demo
		title = "rolle · demo data"
	}
	svc, err := newService()
	if err != nil {
		log.Fatal(err)
	}

	// Read the stored preference first. ROLLE_DEBUG overrides it when set.
	background := application.NewRGB(0x10, 0x11, 0x14)
	if st, err := svc.Settings(); err == nil {
		debug.Set(st.VerboseLogging)
		if st.Theme == "light" {
			background = application.NewRGB(0xF4, 0xF0, 0xE8)
		}
	}

	// ROLLE_DEBUG=1 also raises the Wails runtime log level.
	logLevel := slog.LevelWarn
	if debug.Enabled() {
		logLevel = slog.LevelDebug
		debug.Logf("app", "debug mode on")
	}

	rolle := NewRolleService(svc)
	services := []application.Service{application.NewService(rolle)}
	// macOS delivers notifications only from an app bundle; a bare binary aborts on init.
	var notify *notifications.NotificationService
	if inBundle() {
		notify = notifications.New()
		services = append(services, application.NewService(notify))
	}
	a := application.New(application.Options{
		Name:        "rolle",
		Description: "Assume any role, any cloud",
		LogLevel:    logLevel,
		Services:    services,
		Assets: application.AssetOptions{
			Handler: application.AssetFileServerFS(assets),
		},
		Mac: application.MacOptions{
			// The app keeps running in the tray; Quit lives in the tray menu.
			ApplicationShouldTerminateAfterLastWindowClosed: false,
		},
	})

	window := a.Window.NewWithOptions(application.WebviewWindowOptions{
		Title:     title,
		Width:     1280,
		Height:    720,
		MinWidth:  980,
		MinHeight: 560,
		Mac: application.MacWindow{
			InvisibleTitleBarHeight: 44,
			Backdrop:                application.MacBackdropTranslucent,
			TitleBar:                application.MacTitleBarHiddenInset,
		},
		BackgroundColour: background,
		URL:              "/",
	})

	// Closing the window hides it while the tray keeps sessions alive, unless
	// the user turned that off in settings.
	window.RegisterHook(events.Common.WindowClosing, func(e *application.WindowEvent) {
		if st, err := svc.Settings(); err == nil && !st.HideOnClose {
			return
		}
		window.Hide()
		e.Cancel()
	})
	// Every workspace write tells the UI and the tray to reload.
	svc.OnChange = func() {
		debug.Logf("ui", "workspace changed")
		a.Event.Emit(EventWorkspaceChanged, struct{}{})
	}
	tr := newTray(a, svc, window)
	installMouseNav(a)
	if err := setupUpdater(a, svc); err != nil {
		log.Println("updater:", err)
	}

	// Renew or deactivate expired sessions, now and every 30 seconds. Refresh
	// saves, and so notifies the UI, only when a session changed. A write by
	// another process (the CLI, a second app) changes the file but not this
	// process, so the tick also compares the file's modification time.
	go func() {
		// The app may have moved since a profile was written; point the
		// active profiles at this binary before anything reads them.
		if err := svc.ReconcileProfiles(); err != nil {
			debug.Logf("app", "reconcile profiles: %v", err)
		}
		// An update replaced the app; bring the app's copy of the command along.
		refreshCLI()
		alerts := newNotifier(notify, tr.onNotification)
		w, _ := svc.Refresh()
		alerts.tick(w, currentSettings(svc))
		// The shared credentials file matters too: the shadow marks in the
		// table come from it, and the CLI or an editor can change it.
		credPath := awsconfig.CredentialsPath(svc.AWSConfigPath)
		seen, seenCred := modTime(svc.WorkspacePath), modTime(credPath)
		for range time.Tick(30 * time.Second) {
			if now, nowCred := modTime(svc.WorkspacePath), modTime(credPath); !now.Equal(seen) || !nowCred.Equal(seenCred) {
				svc.OnChange()
			}
			w, _ := svc.Refresh()
			alerts.tick(w, currentSettings(svc))
			seen, seenCred = modTime(svc.WorkspacePath), modTime(credPath)
		}
	}()

	if err := a.Run(); err != nil {
		log.Fatal(err)
	}
}

// currentSettings reads the settings, falling back to defaults when the
// workspace cannot be read.
func currentSettings(svc *app.Service) core.Settings {
	st, err := svc.Settings()
	if err != nil {
		return core.DefaultSettings()
	}
	return st
}

// inBundle reports whether this process runs from a macOS .app bundle. Other
// platforms always qualify.
func inBundle() bool {
	if runtime.GOOS != "darwin" {
		return true
	}
	exe, err := os.Executable()
	return err == nil && strings.Contains(exe, ".app/Contents/MacOS/")
}
