//go:build !linux

package sandbox

func systemReadOnlyRoots() []string { return nil }

func systemDeviceRoots() []string { return nil }