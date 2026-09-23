package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/zalando/go-keyring"
)

// Device-credential storage for the Wayshard CLI.
//
// The CLI is a native client, so in normal operation it stores its device
// credential in platform-secure storage (macOS Keychain, Windows Credential
// Manager, Linux Secret Service). It fails closed when that storage is
// unavailable: it never silently downgrades to a plaintext credential file. The
// protected user-private file fallback is used only when the operator explicitly
// opts into headless mode with WAYSHARD_HEADLESS=1. WAYSHARD_TOKEN always takes
// precedence.
const (
	keyringService = "wayshard"
	keyringUser    = "device-credential"
)

// keyringSet/keyringGet are indirection points so tests can simulate an
// unavailable or failing OS keyring deterministically.
var (
	keyringSet = keyring.Set
	keyringGet = keyring.Get
)

// headlessForced reports whether the operator explicitly asked to skip the OS
// keychain and use the protected file fallback.
func headlessForced() bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv("WAYSHARD_HEADLESS"))) {
	case "1", "true", "yes", "on":
		return true
	}
	return false
}

// tokenPath is the secure headless fallback location (0600 on Unix).
func tokenPath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".config", "wayshard", "device.token")
}

// keyringUnavailable wraps a keyring failure with the explicit opt-in the
// operator needs to use the protected file fallback instead.
func keyringUnavailable(err error) error {
	return fmt.Errorf(
		"OS keyring unavailable (%v); refusing to store a plaintext credential — set WAYSHARD_HEADLESS=1 to use the protected file fallback",
		err,
	)
}

// saveToken stores the device credential in platform-secure storage. It fails
// closed when the OS keyring is unavailable; the protected file fallback is used
// only in explicit headless mode.
func saveToken(t string) error {
	if headlessForced() {
		return saveTokenFile(t)
	}
	if err := keyringSet(keyringService, keyringUser, t); err != nil {
		return keyringUnavailable(err)
	}
	return nil
}

// loadToken resolves the device credential: explicit environment first, then
// platform-secure storage (normal mode) or the protected file (headless mode).
// A missing entry is not an error; an unavailable keyring is.
func loadToken() (string, error) {
	if v := strings.TrimSpace(os.Getenv("WAYSHARD_TOKEN")); v != "" {
		return v, nil
	}
	if headlessForced() {
		t, err := loadTokenFile()
		if errors.Is(err, os.ErrNotExist) {
			return "", nil
		}
		return t, err
	}
	t, err := keyringGet(keyringService, keyringUser)
	if err != nil {
		if errors.Is(err, keyring.ErrNotFound) {
			return "", nil
		}
		return "", keyringUnavailable(err)
	}
	return t, nil
}

func saveTokenFile(t string) error {
	p := tokenPath()
	_ = os.MkdirAll(filepath.Dir(p), 0o700)
	return os.WriteFile(p, []byte(t), 0o600)
}

func loadTokenFile() (string, error) {
	b, err := os.ReadFile(tokenPath())
	return strings.TrimSpace(string(b)), err
}
