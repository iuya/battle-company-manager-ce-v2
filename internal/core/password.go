package core

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"

	"golang.org/x/crypto/argon2"
)

// argonParams are the OWASP minimum recommendation for argon2id (19 MiB, 2 iterations, 1 lane).
type argonParams struct {
	memoryKiB uint32
	time      uint32
	threads   uint8
}

var defaultArgon = argonParams{memoryKiB: 19 * 1024, time: 2, threads: 1}

const (
	saltLen = 16
	keyLen  = 32
)

// hashPassword returns a PHC string: $argon2id$v=19$m=...,t=...,p=...$salt$hash
func hashPassword(password string, p argonParams) (string, error) {
	salt := make([]byte, saltLen)
	if _, err := rand.Read(salt); err != nil {
		return "", err
	}
	key := argon2.IDKey([]byte(password), salt, p.time, p.memoryKiB, p.threads, keyLen)
	b64 := base64.RawStdEncoding
	return fmt.Sprintf("$argon2id$v=%d$m=%d,t=%d,p=%d$%s$%s", argon2.Version, p.memoryKiB, p.time, p.threads,
		b64.EncodeToString(salt), b64.EncodeToString(key)), nil
}

var errBadHash = errors.New("unsupported password hash")

// verifyPassword compares in constant time. The parameters come from the stored hash, so they can be raised later
// without invalidating existing passwords.
func verifyPassword(password, encoded string) (bool, error) {
	parts := strings.Split(encoded, "$")
	if len(parts) != 6 || parts[1] != "argon2id" {
		return false, errBadHash
	}
	var version int
	var p argonParams
	if _, err := fmt.Sscanf(parts[2], "v=%d", &version); err != nil || version != argon2.Version {
		return false, errBadHash
	}
	if _, err := fmt.Sscanf(parts[3], "m=%d,t=%d,p=%d", &p.memoryKiB, &p.time, &p.threads); err != nil {
		return false, errBadHash
	}
	salt, err := base64.RawStdEncoding.DecodeString(parts[4])
	if err != nil {
		return false, errBadHash
	}
	want, err := base64.RawStdEncoding.DecodeString(parts[5])
	if err != nil {
		return false, errBadHash
	}
	got := argon2.IDKey([]byte(password), salt, p.time, p.memoryKiB, p.threads, uint32(len(want)))
	return subtle.ConstantTimeCompare(got, want) == 1, nil
}
