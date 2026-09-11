// Command desktop is the Rolle desktop app.
package main

import (
	"embed"
	"fmt"
	"log"
	"log/slog"
	"os"
	"time"

	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"

	"github.com/nateships/rolle/cmd/rolle/cli"
	"github.com/nateships/rolle/internal/app"
	"github.com/nateships/rolle/internal/debug"
)

// assets holds the built frontend from frontend/dist.
//
//go:embed all:frontend/dist
var assets embed.FS

func init() {
	application.RegisterEvent[struct{}](EventWorkspaceChanged)
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

	svc, err := app.Default()
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
	a := application.New(application.Options{
		Name:        "Rolle",
		Description: "Assume any role, any cloud",
		LogLevel:    logLevel,
		Services: []application.Service{
			application.NewService(rolle),
		},
		Assets: application.AssetOptions{
			Handler: application.AssetFileServerFS(assets),
		},
		Mac: application.MacOptions{
			// The app keeps running in the tray; Quit lives in the tray menu.
			ApplicationShouldTerminateAfterLastWindowClosed: false,
		},
	})

	window := a.Window.NewWithOptions(application.WebviewWindowOptions{
		Title:     "Rolle",
		Width:     1120,
		Height:    720,
		MinWidth:  820,
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
	newTray(a, svc, window)
	installMouseNav(a)
	if err := setupUpdater(a, svc); err != nil {
		log.Println("updater:", err)
	}

	// Renew or deactivate expired sessions. Refresh saves, and so notifies
	// the UI, only when a session changed.
	go func() {
		for range time.Tick(30 * time.Second) {
			_, _ = svc.Refresh()
		}
	}()

	if err := a.Run(); err != nil {
		log.Fatal(err)
	}
}
