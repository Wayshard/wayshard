// Package crypto provides credential hashing and identity helpers used by auth and stored credentials.
package crypto

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base32"
	"encoding/hex"
	"fmt"
	"io"

	"golang.org/x/crypto/argon2"
)

const (
	argonTime    = 1
	argonMemory  = 64 * 1024
	argonThreads = 4
	argonKeyLen  = 32
	saltLen      = 16
)

func Random(n int) ([]byte, error) {
	b := make([]byte, n)
	if _, err := io.ReadFull(rand.Reader, b); err != nil {
		return nil, err
	}
	return b, nil
}

func HashSHA256(b []byte) string {
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

func HashBytes(b []byte) []byte {
	sum := sha256.Sum256(b)
	return sum[:]
}

func ArgonHash(secret, salt []byte) []byte {
	return argon2.IDKey(secret, salt, argonTime, argonMemory, argonThreads, argonKeyLen)
}

func NewSalt() ([]byte, error) {
	return Random(saltLen)
}

func Equal(a, b []byte) bool {
	return subtle.ConstantTimeCompare(a, b) == 1
}

var crockford = base32.NewEncoding("0123456789ABCDEFGHJKMNPQRSTVWXYZ").WithPadding(base32.NoPadding)

func EncodeCode(b []byte) string {
	return crockford.EncodeToString(b)
}

func DecodeCode(s string) ([]byte, error) {
	return crockford.DecodeString(s)
}

func NewKeypair() (ed25519.PublicKey, ed25519.PrivateKey, error) {
	return ed25519.GenerateKey(rand.Reader)
}

func Sign(priv ed25519.PrivateKey, msg []byte) []byte {
	return ed25519.Sign(priv, msg)
}

func Verify(pub ed25519.PublicKey, msg, sig []byte) bool {
	return ed25519.Verify(pub, msg, sig)
}

func Fingerprint(pub ed25519.PublicKey) string {
	return HashSHA256(pub)[:16]
}

func MustRandom(n int) []byte {
	b, err := Random(n)
	if err != nil {
		panic(fmt.Errorf("crypto/rand: %w", err))
	}
	return b
}
