//go:build !darwin

package main

import "github.com/wailsapp/wails/v3/pkg/application"

const (
	// EventNavBack fires when the mouse back button is released.
	EventNavBack = "nav:back"
	// EventNavForward fires when the mouse forward button is released.
	EventNavForward = "nav:forward"
)

func init() {
	application.RegisterEvent[struct{}](EventNavBack)
	application.RegisterEvent[struct{}](EventNavForward)
}

// installMouseNav is a no-op off macOS; the webview delivers these buttons itself.
func installMouseNav(*application.App) {}
