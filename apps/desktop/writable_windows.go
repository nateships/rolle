//go:build windows

package main

import "os"

// writable reports whether this process may create and remove entries in
// dir. It tries, because ACLs decide this and no mode bit shows them.
func writable(dir string) bool {
	f, err := os.CreateTemp(dir, ".rolle-*")
	if err != nil {
		return false
	}
	name := f.Name()
	_ = f.Close()
	return os.Remove(name) == nil
}
