//go:build windows

package main

import (
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows/registry"
)

// userPathList returns the user's PATH from the registry, which is what new
// terminals read. The process environment is stale after an install.
func userPathList() string {
	k, err := registry.OpenKey(registry.CURRENT_USER, "Environment", registry.QUERY_VALUE)
	if err != nil {
		return ""
	}
	defer func() { _ = k.Close() }()
	v, _, err := k.GetStringValue("Path")
	if err != nil {
		return ""
	}
	return v
}

// setUserPathList writes the user's PATH and tells open windows about it.
func setUserPathList(list string) error {
	k, err := registry.OpenKey(registry.CURRENT_USER, "Environment", registry.SET_VALUE)
	if err != nil {
		return err
	}
	defer func() { _ = k.Close() }()
	if err := k.SetExpandStringValue("Path", list); err != nil {
		return err
	}
	broadcastEnvironmentChange()
	return nil
}

// broadcastEnvironmentChange sends WM_SETTINGCHANGE "Environment", so Explorer
// and new terminals pick up the PATH without a sign-out.
func broadcastEnvironmentChange() {
	const (
		hwndBroadcast   = 0xffff
		wmSettingChange = 0x001A
		smtoAbortIfHung = 0x0002
	)
	user32 := syscall.NewLazyDLL("user32.dll")
	proc := user32.NewProc("SendMessageTimeoutW")
	env, _ := syscall.UTF16PtrFromString("Environment")
	var result uintptr
	_, _, _ = proc.Call(hwndBroadcast, wmSettingChange, 0, uintptr(unsafe.Pointer(env)), smtoAbortIfHung, 5000, uintptr(unsafe.Pointer(&result)))
}
