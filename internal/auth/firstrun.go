package auth

import (
	"fmt"
	"os"
	"path/filepath"

	"golang.org/x/crypto/bcrypt"
	"gopkg.in/yaml.v3"
)

const (
	bcryptCost   = 12
	DefaultUser  = "admin"
	authFileName = "auth.yaml"
)

type authFile struct {
	Username     string `yaml:"username"`
	PasswordHash string `yaml:"password_hash"`
}

func authFilePath(dataDir string) string {
	return filepath.Join(dataDir, authFileName)
}

func CreateUser(dataDir, username, password string) error {
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcryptCost)
	if err != nil {
		return fmt.Errorf("hash password: %w", err)
	}

	af := authFile{
		Username:     username,
		PasswordHash: string(hash),
	}

	return writeAuthFile(authFilePath(dataDir), af)
}

// SetPassword creates or overwrites the stored password.
// Used during initial setup.
func SetPassword(dataDir, password string) error {
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcryptCost)
	if err != nil {
		return fmt.Errorf("hash password: %w", err)
	}

	af := authFile{
		Username:     DefaultUser,
		PasswordHash: string(hash),
	}

	return writeAuthFile(authFilePath(dataDir), af)
}

// ChangePassword verifies the current password and updates it.
func ChangePassword(dataDir, username, current, new string) error {
	af, err := readAuthFile(authFilePath(dataDir))
	if err != nil {
		return err
	}

	if af.Username != username {
		return ErrBadCredentials
	}

	if err := bcrypt.CompareHashAndPassword(
		[]byte(af.PasswordHash),
		[]byte(current),
	); err != nil {
		return ErrBadCredentials
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(new), bcryptCost)
	if err != nil {
		return err
	}

	af.PasswordHash = string(hash)
	return writeAuthFile(authFilePath(dataDir), af)
}

// VerifyPassword checks login credentials.
func VerifyPassword(dataDir, username, password string) error {
	af, err := readAuthFile(authFilePath(dataDir))
	if err != nil {
		return err
	}

	if af.Username != username {
		return ErrBadCredentials
	}

	return bcrypt.CompareHashAndPassword(
		[]byte(af.PasswordHash),
		[]byte(password),
	)
}

func GetUsername(dataDir string) (string, error) {
	af, err := readAuthFile(authFilePath(dataDir))
	if err != nil {
		return "", err
	}
	return af.Username, nil
}

// ── helpers ─────────────────────────────────

func readAuthFile(path string) (authFile, error) {
	clean := filepath.Clean(path)
	data, err := os.ReadFile(clean) //nolint:gosec
	if err != nil {
		return authFile{}, err
	}

	var af authFile
	if err = yaml.Unmarshal(data, &af); err != nil {
		return authFile{}, err
	}

	return af, nil
}

func writeAuthFile(path string, af authFile) error {
	data, err := yaml.Marshal(af)
	if err != nil {
		return err
	}

	if err = os.WriteFile(path, data, 0o600); err != nil {
		return err
	}

	return nil
}
