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
	"github.com/wailsapp/wails/v3/pkg/services/dock"
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

	// Read the stored preferences first. ROLLE_DEBUG overrides the logging one when set.
	st := currentSettings(svc)
	debug.Set(st.VerboseLogging)
	background := application.NewRGB(0x10, 0x11, 0x14)
	if st.Theme == "light" {
		background = application.NewRGB(0xF4, 0xF0, 0xE8)
	}

	// ROLLE_DEBUG=1 also raises the Wails runtime log level.
	logLevel := slog.LevelWarn
	if debug.Enabled() {
		logLevel = slog.LevelDebug
		debug.Logf("app", "debug mode on")
	}

	rolle := NewRolleService(svc)
	// The Dock service shows the expiring count as a badge; Windows needs
	// its startup for the taskbar.
	dockIcon := dock.New()
	services := []application.Service{application.NewService(rolle), application.NewService(dockIcon)}
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
			ActivationPolicy: dockPolicy(st.HideDock),
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
	// Every workspace write tells the UI and the tray to reload, and wakes the
	// expiry loop so a new lead time or session takes effect at once. The
	// loop itself only tells the others, so it does not wake itself.
	changed := func() {
		debug.Logf("ui", "workspace changed")
		a.Event.Emit(EventWorkspaceChanged, struct{}{})
	}
	wake := make(chan struct{}, 1)
	svc.OnChange = func() {
		changed()
		select {
		case wake <- struct{}{}:
		default:
		}
	}
	tr := newTray(a, svc, window, dockIcon)
	// The save that follows rebuilds the tray, which puts the badge back on
	// a Dock icon that just came back.
	rolle.applySettings = func(prev, next core.Settings) error {
		return applyDesktopSettings(a, dockIcon, window, prev, next)
	}
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
		// The app may have moved too. A login item that no longer points at
		// this binary is written again; one that does is left alone.
		if st.LoginItem && !loginItemBlocked() {
			if on, err := a.Autostart.IsEnabled(); err != nil || !on {
				if err := a.Autostart.Enable(); err != nil {
					debug.Logf("app", "login item: %v", err)
				}
			}
		}
		alerts := newNotifier(notify, tr.onNotification)
		w, _ := svc.Refresh()
		st := settingsOf(w)
		alerts.tick(w, st)
		// The shared credentials file matters too: the shadow marks in the
		// table come from it, and the CLI or an editor can change it.
		credPath := awsconfig.CredentialsPath(svc.AWSConfigPath)
		seen, seenCred := modTime(svc.WorkspacePath), modTime(credPath)
		// The tray flag and the Dock badge follow the same count as the
		// warnings. A timer set for the next crossing rebuilds the tray on
		// the mark; a workspace write rebuilds it through OnChange, and the
		// 30 second poll catches renewals and edits from outside.
		poll := time.NewTicker(30 * time.Second)
		next := time.NewTimer(untilNextCrossing(w, st, time.Now()))
		for {
			crossed := false
			select {
			case <-poll.C:
			case <-next.C:
				crossed = true
			case <-wake:
			}
			if now, nowCred := modTime(svc.WorkspacePath), modTime(credPath); !now.Equal(seen) || !nowCred.Equal(seenCred) {
				changed()
			}
			w, _ = svc.Refresh()
			st = settingsOf(w)
			alerts.tick(w, st)
			if crossed {
				tr.rebuild()
			}
			seen, seenCred = modTime(svc.WorkspacePath), modTime(credPath)
			next.Reset(untilNextCrossing(w, st, time.Now()))
		}
	}()

	if err := a.Run(); err != nil {
		log.Fatal(err)
	}
}

// settingsOf returns the settings in w, or the defaults when w could not be read.
func settingsOf(w *core.Workspace) core.Settings {
	if w == nil {
		return core.DefaultSettings()
	}
	return w.EffectiveSettings()
}

// untilNextCrossing is the wait until something in w changes state: an active
// session enters the warning lead or expires, or a sign-in with active
// sessions enters its own lead. A second past the moment keeps the tick on
// the right side of the boundary. Nothing ahead means a long wait; the poll
// still runs.
func untilNextCrossing(w *core.Workspace, st core.Settings, now time.Time) time.Duration {
	const idle = time.Hour
	if w == nil {
		return idle
	}
	var next time.Time
	consider := func(t time.Time) {
		if t.After(now) && (next.IsZero() || t.Before(next)) {
			next = t
		}
	}
	for _, s := range w.Sessions {
		if s.Status != core.StatusActive || s.Expires == nil {
			continue
		}
		consider(s.Expires.Add(-st.NotifyLead()))
		consider(*s.Expires)
	}
	for _, in := range w.Integrations {
		// portalDue knows which sign-ins the tray watches; zero means none.
		if left, _ := portalDue(w, in, now); left > 0 {
			consider(now.Add(left - portalWarnBefore))
			consider(now.Add(left))
		}
	}
	if next.IsZero() {
		return idle
	}
	return next.Sub(now) + time.Second
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
