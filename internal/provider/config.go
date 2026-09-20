package provider

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/Wayshard/wayshard/internal/sandbox"
)

// ShimArg is the internal argv marker that runs the in-namespace provider shim.
const ShimArg = "--wayshard-provider-shim"

// ShimConfig is written by the server and read once by the shim before it is
// deleted. It carries only per-attempt transport data, never provider secrets.
type ShimConfig struct {
	BrokerSocket   string         `json:"brokerSocket"`
	Bearer         string         `json:"bearer"`
	ProxyPort      int            `json:"proxyPort"`
	Policy         sandbox.Policy `json:"policy"`
	HarnessCommand string         `json:"harnessCommand"`
	HarnessArgs    []string       `json:"harnessArgs"`
	HarnessDir     string         `json:"harnessDir"`
}

// WriteShimConfig writes the shim configuration with owner-only permissions and
// returns its path.
func WriteShimConfig(dir string, cfg ShimConfig) (string, error) {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", err
	}
	b, err := json.Marshal(cfg)
	if err != nil {
		return "", err
	}
	path := filepath.Join(dir, "c.json")
	if err := os.WriteFile(path, b, 0o600); err != nil {
		return "", err
	}
	return path, nil
}

// RandomBearer returns a fresh per-attempt capability used to authorize the
// shim to the broker.
func RandomBearer() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

// RandomPort returns a per-attempt proxy port. The provider namespace is empty
// except for loopback, so no listener can already own the port there.
func RandomPort() (int, error) {
	var b [2]byte
	for i := 0; i < 16; i++ {
		if _, err := rand.Read(b[:]); err != nil {
			return 0, err
		}
		p := 20000 + int(b[0])<<8 + int(b[1])
		if p >= 20000 && p <= 60999 {
			return p, nil
		}
	}
	return 0, fmt.Errorf("could not allocate provider proxy port")
}

// BrokerDir returns a short per-attempt private directory for the broker socket
// and shim config. Filesystem Unix sockets are limited to ~108 bytes, so the
// attempt identity is hashed and the directory names are deliberately short.
func BrokerDir(root, key string) string {
	sum := sha256.Sum256([]byte(key))
	return filepath.Join(root, "runtime", "p", hex.EncodeToString(sum[:8]))
}
