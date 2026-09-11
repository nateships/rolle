// Command desktop is the Rolle desktop app.
package main

import (
	"embed"
	"log"
	"time"

	"github.com/wailsapp/wails/v3/pkg/application"

	"github.com/nateships/rolle/internal/app"
)

// Frontend files are built into frontend/dist and embedded here.
//
//go:embed all:frontend/dist
var assets embed.FS

func init() {
	application.RegisterEvent[struct{}](EventWorkspaceChanged)
}

func main() {
	svc, err := app.Default()
	if err != nil {
		log.Fatal(err)
	}

	rolle := NewRolleService(svc)
	a := application.New(application.Options{
		Name:        "Rolle",
		Description: "Assume any role, any cloud",
		Services: []application.Service{
			application.NewService(rolle),
		},
		Assets: application.AssetOptions{
			Handler: application.AssetFileServerFS(assets),
		},
		Mac: application.MacOptions{
			ApplicationShouldTerminateAfterLastWindowClosed: true,
		},
	})

	a.Window.NewWithOptions(application.WebviewWindowOptions{
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
		BackgroundColour: application.NewRGB(9, 9, 11),
		URL:              "/",
	})

	// Reconcile expiring sessions and nudge the UI so countdowns stay honest.
	go func() {
		for range time.Tick(30 * time.Second) {
			if _, err := svc.Refresh(); err == nil {
				rolle.changed()
			}
		}
	}()

	if err := a.Run(); err != nil {
		log.Fatal(err)
	}
}
