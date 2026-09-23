package main

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/zalando/go-keyring"
)

// Device-credential storage for the Wayshard CLI.
//
// The CLI is a native client, so it stores its device credential in
// platform-secure storage (macOS Keychain, Windows Credential Manager, Linux
// Secret Service). When that is unavailable — or when the operator explicitly
// opts into headless mode — it falls back to a 0600 file in the user config
// directory, which is the documented secure headless fallback.
const (
	keyringService = "wayshard"
	keyringUser    = "device-credential"
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

// tokenPath is the secure headless fallback location (0600).
func tokenPath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".config", "wayshard", "device.token")
}

// saveToken stores the device credential in platform-secure storage, falling
// back to the protected file only when the OS keychain is unavailable or
// headless mode is explicitly requested.
func saveToken(t string) error {
	if !headlessForced() {
		if err := keyring.Set(keyringService, keyringUser, t); err == nil {
			return nil
		}
	}
	return saveTokenFile(t)
}

// loadToken resolves the device credential: explicit environment, then
// platform-secure storage, then the protected file fallback.
func loadToken() (string, error) {
	if v := strings.TrimSpace(os.Getenv("WAYSHARD_TOKEN")); v != "" {
		return v, nil
	}
	if !headlessForced() {
		if t, err := keyring.Get(keyringService, keyringUser); err == nil && t != "" {
			return t, nil
		}
	}
	return loadTokenFile()
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
