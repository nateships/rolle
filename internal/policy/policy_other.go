//go:build !darwin

package policy

import "os"

// sources is empty off macOS. Windows can read the registry here later.
func sources() []string { return nil }

func trusted(os.FileInfo) bool { return false }
