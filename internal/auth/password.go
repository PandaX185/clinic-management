package auth

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"

	"golang.org/x/crypto/argon2"
)

// Argon2id parameters. Memory/time cost tuned for interactive login.
const (
	argonTime    = 3
	argonMemory  = 64 * 1024 // KiB
	argonThreads = 4
	argonKeyLen  = 32
	argonSaltLen = 16
)

var ErrInvalidHash = errors.New("auth: invalid hash format")

// HashPassword derives an argon2id hash of password and returns it encoded
// in PHC string format: $argon2id$v=19$m=65536,t=3,p=4$<salt>$<hash>.
func HashPassword(password string) (string, error) {
	salt := make([]byte, argonSaltLen)
	if _, err := rand.Read(salt); err != nil {
		return "", fmt.Errorf("generate salt: %w", err)
	}
	key := argon2.IDKey([]byte(password), salt, argonTime, argonMemory, argonThreads, argonKeyLen)
	return encodePHC(salt, key), nil
}

// VerifyPassword checks password against a PHC-encoded argon2id hash,
// honoring the parameters stored in the string. It returns nil on match.
func VerifyPassword(password, encodedHash string) error {
	return verifyWithParams(password, encodedHash)
}

func encodePHC(salt, key []byte) string {
	b64 := base64.RawStdEncoding
	return fmt.Sprintf("$argon2id$v=%d$m=%d,t=%d,p=%d$%s$%s",
		argon2.Version, argonMemory, argonTime, argonThreads,
		b64.EncodeToString(salt), b64.EncodeToString(key))
}

func decodePHC(phc string) (salt, key []byte, err error) {
	parts := strings.Split(phc, "$")
	if len(parts) != 6 || parts[0] != "" || parts[1] != "argon2id" {
		return nil, nil, ErrInvalidHash
	}
	var version int
	if _, err := fmt.Sscanf(parts[2], "v=%d", &version); err != nil || version != argon2.Version {
		return nil, nil, ErrInvalidHash
	}
	var m uint32
	var t uint32
	var p uint8
	if _, err := fmt.Sscanf(parts[3], "m=%d,t=%d,p=%d", &m, &t, &p); err != nil {
		return nil, nil, ErrInvalidHash
	}
	b64 := base64.RawStdEncoding
	if salt, err = b64.DecodeString(parts[4]); err != nil {
		return nil, nil, ErrInvalidHash
	}
	if key, err = b64.DecodeString(parts[5]); err != nil {
		return nil, nil, ErrInvalidHash
	}
	return salt, key, nil
}

// verifyWithParams re-derives the key using the parameters stored in the PHC
// string so old hashes stay verifiable if we ever raise the defaults.
func verifyWithParams(password string, phc string) error {
	salt, want, err := decodePHC(phc)
	if err != nil {
		return err
	}
	parts := strings.Split(phc, "$")
	var m, t uint32
	var p uint8
	fmt.Sscanf(parts[3], "m=%d,t=%d,p=%d", &m, &t, &p)
	got := argon2.IDKey([]byte(password), salt, t, m, p, uint32(len(want)))
	if subtle.ConstantTimeCompare(got, want) != 1 {
		return errors.New("auth: invalid credentials")
	}
	return nil
}
