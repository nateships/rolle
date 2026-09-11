//go:build darwin

package main

/*
#cgo CFLAGS: -x objective-c
#cgo LDFLAGS: -framework Cocoa
#import <Cocoa/Cocoa.h>

extern void rolleMouseNav(int kind, int button, double deltaX, int keyCode, unsigned long flags);

// WebKit does not forward mouse buttons 3 and 4 to the page, so watch at the
// application level. Also watch swipes and modified keys, because some mouse
// drivers deliver "back" and "forward" as gestures or as Cmd+[ / Cmd+].
static void rolleInstallMouseNavMonitor(void) {
	NSEventMask mask = NSEventMaskOtherMouseDown | NSEventMaskOtherMouseUp | NSEventMaskSwipe | NSEventMaskKeyDown;
	[NSEvent addLocalMonitorForEventsMatchingMask:mask handler:^NSEvent *(NSEvent *event) {
		switch ([event type]) {
		case NSEventTypeOtherMouseDown:
			rolleMouseNav(1, (int)[event buttonNumber], 0, 0, 0);
			break;
		case NSEventTypeOtherMouseUp:
			rolleMouseNav(2, (int)[event buttonNumber], 0, 0, 0);
			break;
		case NSEventTypeSwipe:
			rolleMouseNav(3, 0, [event deltaX], 0, 0);
			break;
		case NSEventTypeKeyDown:
			if ([event modifierFlags] & (NSEventModifierFlagCommand | NSEventModifierFlagOption)) {
				rolleMouseNav(4, 0, 0, (int)[event keyCode], (unsigned long)[event modifierFlags]);
			}
			break;
		default:
			break;
		}
		return event;
	}];
}
*/
import "C"

import (
	"github.com/wailsapp/wails/v3/pkg/application"

	"github.com/nateships/rolle/internal/debug"
)

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
	debug.Logf("mousenav", "monitor installed")
}

//export rolleMouseNav
func rolleMouseNav(kind, button C.int, deltaX C.double, keyCode C.int, flags C.ulong) {
	debug.Logf("mousenav", "kind=%d button=%d deltaX=%.2f keyCode=%d flags=%#x", int(kind), int(button), float64(deltaX), int(keyCode), uint64(flags))
	if navApp == nil {
		return
	}
	switch int(kind) {
	case 2: // other mouse up: 3 is back, 4 is forward, higher buttons alternate the same way
		switch {
		case int(button) == 3 || (int(button) > 4 && int(button)%2 == 1):
			navApp.Event.Emit(EventNavBack, struct{}{})
		case int(button) >= 4:
			navApp.Event.Emit(EventNavForward, struct{}{})
		}
	case 3: // swipe: positive deltaX is a leftward swipe, which macOS treats as back
		switch {
		case float64(deltaX) > 0:
			navApp.Event.Emit(EventNavBack, struct{}{})
		case float64(deltaX) < 0:
			navApp.Event.Emit(EventNavForward, struct{}{})
		}
	}
}
