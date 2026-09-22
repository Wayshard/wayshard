// Package process provides ownership tracking for server-launched process
// trees so startup reconciliation can terminate stale descendants after an
// unexpected server death without guessing from reused PIDs.
package process

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
)

// TokenEnv is the environment variable that carries a per-attempt ownership
// token into the supervised process and all of its descendants. Only the token
// hash is persisted; the raw token is never stored.
const TokenEnv = "WAYSHARD_OWNER_TOKEN"

// ToolTokenEnv carries a per-tool-session ownership token in addition to the
// attempt token, so a single tool session's detached descendants can be
// reconciled independently without terminating the running harness.
const ToolTokenEnv = "WAYSHARD_TOOL_TOKEN"

// NewToken returns a fresh high-entropy ownership token.
func NewToken() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

// HashToken returns the persisted digest of an ownership token.
func HashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}
