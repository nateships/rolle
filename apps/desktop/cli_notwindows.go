//go:build !windows

package main

import "os/exec"

func userPathList() (string, error) { return "", nil }
func setUserPathList(string) error  { return nil }
func hideWindow(*exec.Cmd)          {}
