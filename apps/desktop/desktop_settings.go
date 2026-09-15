package main

import (
	"errors"
	"os"
	"runtime"

	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/services/dock"

	"github.com/nateships/rolle/internal/core"
)

// errAppImageLoginItem explains why the AppImage cannot register itself:
// Wails records the executable, which for an AppImage is a path inside a
// temporary mount that is gone at the next login.
var errAppImageLoginItem = errors.New("start at login is not available from the AppImage yet; add it to your desktop's startup applications")

// loginItemBlocked reports whether this build must not write a login item.
func loginItemBlocked() bool {
	return runtime.GOOS == "linux" && os.Getenv("APPIMAGE") != ""
}

// dockPolicy maps the Dock preference to the macOS activation policy the app
// starts with. Accessory apps have a tray icon and windows but no Dock icon.
func dockPolicy(hideDock bool) application.ActivationPolicy {
	if hideDock {
		return application.ActivationPolicyAccessory
	}
	return application.ActivationPolicyRegular
}

// applyDesktopSettings puts the preferences that live in the OS, not the
// workspace, into effect: the Dock icon and the login item. It runs before
// the save, so an error leaves the stored settings as they were.
func applyDesktopSettings(a *application.App, dockIcon *dock.DockService, window *application.WebviewWindow, prev, next core.Settings) error {
	if prev.LoginItem != next.LoginItem {
		if loginItemBlocked() {
			return errAppImageLoginItem
		}
		var err error
		if next.LoginItem {
			err = a.Autostart.Enable()
		} else {
			err = a.Autostart.Disable()
		}
		if err != nil {
			return err
		}
	}
	// Only macOS has a Dock; the switch shows only there.
	if runtime.GOOS == "darwin" && prev.HideDock != next.HideDock {
		if next.HideDock {
			dockIcon.HideAppIcon()
			// Leaving the Dock deactivates the app, and the window the user
			// is in drops behind the others. Bring it back.
			window.Focus()
		} else {
			dockIcon.ShowAppIcon()
		}
	}
	return nil
}
