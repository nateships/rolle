//go:build production

package main

import "errors"

// Release builds (wails3 build uses -tags production) keep the method names so
// the generated bindings match, but refuse to act.
const devMode = false

var errNoDev = errors.New("dev tools are not available in release builds")

// DevReset is disabled in release builds.
func (r *RolleService) DevReset() error { return errNoDev }

// DevReplayOnboarding is disabled in release builds.
func (r *RolleService) DevReplayOnboarding() error { return errNoDev }
