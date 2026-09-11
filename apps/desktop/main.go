// Command desktop is the Rolle desktop app.
package main

import (
	"embed"
	"log"

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

	a := application.New(application.Options{
		Name:        "Rolle",
		Description: "Assume any role, any cloud",
		Services: []application.Service{
			application.NewService(NewRolleService(svc)),
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

	if err := a.Run(); err != nil {
		log.Fatal(err)
	}
}
