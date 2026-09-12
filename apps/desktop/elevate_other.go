//go:build !darwin && !windows

package main

import "errors"

func elevatedSwap(string, string) error { return errors.New("elevated install is not available here") }
func relaunchAfterExit(string) error    { return nil }
