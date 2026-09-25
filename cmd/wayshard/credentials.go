package main

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// Device-credential storage for the Wayshard CLI.
//
// The CLI stores its device credential in a restricted user config file
// (directory 0700, file 0600 on Unix; the user-profile ACL on Windows), or the
// credential is supplied through WAYSHARD_TOKEN, which takes precedence. No OS
// keyring is required, so the CLI works on headless and minimal systems.
//
// WAYSHARD_CONFIG overrides the config directory (used by tests).

func configDir() string {
	if d := strings.TrimSpace(os.Getenv("WAYSHARD_CONFIG")); d != "" {
		return d
	}
	home, _ := os.UserHomeDir()
	switch runtime.GOOS {
	case "darwin":
		return filepath.Join(home, "Library", "Application Support", "Wayshard")
	case "windows":
		if app := os.Getenv("APPDATA"); app != "" {
			return filepath.Join(app, "Wayshard")
		}
		return filepath.Join(home, "AppData", "Roaming", "Wayshard")
	default:
		if xdg := os.Getenv("XDG_CONFIG_HOME"); xdg != "" {
			return filepath.Join(xdg, "wayshard")
		}
		return filepath.Join(home, ".config", "wayshard")
	}
}

// tokenPath is the restricted device-credential file location.
func tokenPath() string {
	return filepath.Join(configDir(), "device.token")
}

// saveToken stores the device credential in the restricted config file.
func saveToken(t string) error {
	p := tokenPath()
	if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
		return err
	}
	return os.WriteFile(p, []byte(t), 0o600)
}

// loadToken resolves the device credential: an explicit environment credential
// takes precedence, then the restricted config file. A missing file is not an
// error.
func loadToken() (string, error) {
	if v := strings.TrimSpace(os.Getenv("WAYSHARD_TOKEN")); v != "" {
		return v, nil
	}
	b, err := os.ReadFile(tokenPath())
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return "", nil
		}
		return "", err
	}
	return strings.TrimSpace(string(b)), nil
}
