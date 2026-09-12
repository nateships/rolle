//go:build !windows

package main

import "syscall"

// writable reports whether this process may create and remove entries in
// path. A standard macOS account cannot in /Applications.
func writable(path string) bool { return syscall.Access(path, 0x2) == nil }
