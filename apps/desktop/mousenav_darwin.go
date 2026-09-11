//go:build darwin

package main

/*
#cgo CFLAGS: -x objective-c
#cgo LDFLAGS: -framework Cocoa
#import <Cocoa/Cocoa.h>

extern void rolleMouseNav(int button);

// WebKit does not forward mouse buttons 3 and 4 to the page, so watch for them
// at the application level and hand the button number to Go.
static void rolleInstallMouseNavMonitor(void) {
	[NSEvent addLocalMonitorForEventsMatchingMask:NSEventMaskOtherMouseUp
	                                      handler:^NSEvent *(NSEvent *event) {
		NSInteger button = [event buttonNumber];
		if (button == 3 || button == 4) {
			rolleMouseNav((int)button);
		}
		return event;
	}];
}
*/
import "C"

import "github.com/wailsapp/wails/v3/pkg/application"

const (
	// EventNavBack fires when the mouse back button is released.
	EventNavBack = "nav:back"
	// EventNavForward fires when the mouse forward button is released.
	EventNavForward = "nav:forward"
)

var navApp *application.App

func init() {
	application.RegisterEvent[struct{}](EventNavBack)
	application.RegisterEvent[struct{}](EventNavForward)
}

// installMouseNav starts forwarding mouse back and forward buttons to the frontend.
func installMouseNav(a *application.App) {
	navApp = a
	C.rolleInstallMouseNavMonitor()
}

//export rolleMouseNav
func rolleMouseNav(button C.int) {
	if navApp == nil {
		return
	}
	switch button {
	case 3:
		navApp.Event.Emit(EventNavBack, struct{}{})
	case 4:
		navApp.Event.Emit(EventNavForward, struct{}{})
	}
}
