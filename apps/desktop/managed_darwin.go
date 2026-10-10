//go:build darwin

package main

/*
#cgo LDFLAGS: -framework CoreFoundation
#include <CoreFoundation/CoreFoundation.h>

// Returns 1 when a configuration profile forces DisableUpdates to true in the
// com.getrolle.app domain. A value the user writes with defaults is not
// forced, so it has no effect.
static int rolleUpdatesManaged(void) {
	CFStringRef key = CFSTR("DisableUpdates");
	CFStringRef app = CFSTR("com.getrolle.app");
	if (!CFPreferencesAppValueIsForced(key, app)) return 0;
	CFPropertyListRef v = CFPreferencesCopyAppValue(key, app);
	if (v == NULL) return 0;
	int on = CFGetTypeID(v) == CFBooleanGetTypeID() && CFBooleanGetValue((CFBooleanRef)v);
	CFRelease(v);
	return on;
}
*/
import "C"

// managedByProfile reads the DisableUpdates key that an MDM configuration
// profile sets.
func managedByProfile() bool { return C.rolleUpdatesManaged() == 1 }
