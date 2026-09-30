// Package storage writes protected local state, replacing only after validation.
package storage

import (
	"crypto/rand"
	"encoding/hex"
	"networkroute/internal/identity"
	"os"
	"path/filepath"
)

func Write(path string, data []byte, validate func(string) error) error {
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	var nonce [12]byte
	if _, err := rand.Read(nonce[:]); err != nil {
		return err
	}
	temp := path + ".tmp-" + hex.EncodeToString(nonce[:])
	if err := identity.WritePrivateFile(temp, data); err != nil {
		return err
	}
	defer os.Remove(temp)
	if validate != nil {
		if err := validate(temp); err != nil {
			return err
		}
	}
	return replace(temp, path)
}
