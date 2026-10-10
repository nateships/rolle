//go:build !darwin

package main

import "errors"

func runAsAdmin(string, string) error {
	return errors.New("the administrator prompt is not available here")
}
