package paths

import (
	"os"
	"path/filepath"
	"runtime"
)

const DefaultPort = 7420

func DataDir() string {
	if d := os.Getenv("WAYSHARD_DATA"); d != "" {
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
		if xdg := os.Getenv("XDG_DATA_HOME"); xdg != "" {
			return filepath.Join(xdg, "wayshard")
		}
		return filepath.Join(home, ".local", "share", "wayshard")
	}
}

func RuntimeDir(root string) string {
	return filepath.Join(root, "runtime")
}

func WorkspacesDir(root string) string {
	return filepath.Join(root, "runtime", "workspaces")
}
