//go:build !windows

package main

import "syscall"

// writable reports whether this process may create and remove entries in
// path. access(2) honours ownership and ACLs, so a standard macOS account
// gets false for /Applications.
func writable(path string) bool { return syscall.Access(path, 0x2) == nil }
