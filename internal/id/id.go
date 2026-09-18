// Package id generates sortable unique identifiers (UUIDv7) for domain entities.
package id

import "github.com/google/uuid"

func New() string {
	return uuid.Must(uuid.NewV7()).String()
}

func Parse(s string) (uuid.UUID, error) {
	return uuid.Parse(s)
}

func Valid(s string) bool {
	_, err := uuid.Parse(s)
	return err == nil
}
