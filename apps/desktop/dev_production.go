//go:build production

package main

// Release builds (wails3 build uses -tags production) hide the dev menu.
const devMode = false
