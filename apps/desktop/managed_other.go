//go:build !darwin

package main

// managedByProfile is false off macOS; only macOS reads configuration profiles.
func managedByProfile() bool { return false }
