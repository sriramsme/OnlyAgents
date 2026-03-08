package auth

import (
	"crypto/rand"
	"fmt"
	"math/big"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/crypto/bcrypt"
	"gopkg.in/yaml.v3"
)

const (
	bcryptCost   = 12
	DefaultUser  = "admin"
	authFileName = "auth.yaml"
)

// authFile is the on-disk representation of auth.yaml
type authFile struct {
	Username     string `yaml:"username"`
	PasswordHash string `yaml:"password_hash"`
}

func authFilePath(dataDir string) string {
	return filepath.Join(dataDir, authFileName)
}

// LoadOrCreate reads auth.yaml from dataDir.
// If the file does not exist, it generates a random password, writes the hash,
// prints the credentials to stdout ONCE, and returns.
//
// Returns (username, plaintextPassword, error).
// plaintextPassword is non-empty only on first run — never stored, just printed.
func LoadOrCreate(dataDir string) (username, firstRunPassword string, err error) {
	path := authFilePath(dataDir)

	if _, statErr := os.Stat(path); os.IsNotExist(statErr) {
		return firstRun(dataDir, path)
	}

	af, err := readAuthFile(path)
	if err != nil {
		return "", "", fmt.Errorf("reading auth.yaml: %w", err)
	}
	return af.Username, "", nil
}

// firstRun generates credentials, persists them, and returns the plaintext
// password so the caller can print it.
func firstRun(dataDir, path string) (username, password string, err error) {
	if err = os.MkdirAll(dataDir, 0700); err != nil {
		return "", "", fmt.Errorf("creating data dir: %w", err)
	}

	password, err = generatePassword()
	if err != nil {
		return "", "", fmt.Errorf("generating password: %w", err)
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcryptCost)
	if err != nil {
		return "", "", fmt.Errorf("hashing password: %w", err)
	}

	af := authFile{
		Username:     DefaultUser,
		PasswordHash: string(hash),
	}
	if err = writeAuthFile(path, af); err != nil {
		return "", "", err
	}

	return DefaultUser, password, nil
}

// UpdatePassword hashes the new password and writes it to auth.yaml.
// Does NOT invalidate sessions — the caller must do that.
func UpdatePassword(dataDir, newPassword string) error {
	path := authFilePath(dataDir)

	af, err := readAuthFile(path)
	if err != nil {
		return err
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(newPassword), bcryptCost)
	if err != nil {
		return fmt.Errorf("hashing password: %w", err)
	}

	af.PasswordHash = string(hash)
	return writeAuthFile(path, af)
}

// ResetPassword generates a new random password, writes it, and returns it
// so the CLI can print it. Used for "forgot password" recovery.
func ResetPassword(dataDir string) (string, error) {
	newPass, err := generatePassword()
	if err != nil {
		return "", err
	}
	if err = UpdatePassword(dataDir, newPass); err != nil {
		return "", err
	}
	return newPass, nil
}

// VerifyPassword checks a plaintext password against the stored hash.
func VerifyPassword(dataDir, password string) error {
	path := authFilePath(dataDir)
	af, err := readAuthFile(path)
	if err != nil {
		return err
	}
	return bcrypt.CompareHashAndPassword([]byte(af.PasswordHash), []byte(password))
}

// GetUsername returns the configured username.
func GetUsername(dataDir string) (string, error) {
	af, err := readAuthFile(authFilePath(dataDir))
	if err != nil {
		return "", err
	}
	return af.Username, nil
}

// ─── helpers ──────────────────────────────────────────────────────────────────

func readAuthFile(path string) (authFile, error) {
	data, err := os.ReadFile(path) // nolint:gosec
	if err != nil {
		return authFile{}, fmt.Errorf("reading %s: %w", path, err)
	}
	var af authFile
	if err = yaml.Unmarshal(data, &af); err != nil {
		return authFile{}, fmt.Errorf("parsing %s: %w", path, err)
	}
	return af, nil
}

func writeAuthFile(path string, af authFile) error {
	data, err := yaml.Marshal(af)
	if err != nil {
		return fmt.Errorf("marshalling auth file: %w", err)
	}
	// 0600 — owner read/write only. No other user can read the hash.
	if err = os.WriteFile(path, data, 0600); err != nil {
		return fmt.Errorf("writing %s: %w", path, err)
	}
	return nil
}

// generatePassword produces a human-typeable random password.
// Format: word-XXXX-word  e.g. "agent-7k2m-council"
// Avoids ambiguous chars (0/O, 1/l/I).
func generatePassword() (string, error) {
	const charset = "abcdefghjkmnpqrstuvwxyz23456789"
	const segLen = 4

	words := []string{
		"agent", "model", "kernel", "route", "signal",
		"forge", "relay", "nexus", "proxy", "vault",
	}

	pick := func(n int) (string, error) {
		_, err := rand.Int(rand.Reader, big.NewInt(int64(n)))
		if err != nil {
			return "", err
		}
		return "", err // placeholder — see below
	}
	_ = pick

	// Pick two random words
	w1idx, err := rand.Int(rand.Reader, big.NewInt(int64(len(words))))
	if err != nil {
		return "", err
	}
	w2idx, err := rand.Int(rand.Reader, big.NewInt(int64(len(words))))
	if err != nil {
		return "", err
	}

	// Generate random segment
	seg := make([]byte, segLen)
	for i := range seg {
		n, err := rand.Int(rand.Reader, big.NewInt(int64(len(charset))))
		if err != nil {
			return "", err
		}
		seg[i] = charset[n.Int64()]
	}

	return strings.Join([]string{
		words[w1idx.Int64()],
		string(seg),
		words[w2idx.Int64()],
	}, "-"), nil
}
