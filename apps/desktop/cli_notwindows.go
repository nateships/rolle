//go:build !windows

package main

func userPathList() string         { return "" }
func setUserPathList(string) error { return nil }
