package bootstrap

import (
	"embed"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

//go:embed defaults/*
var defaultsFS embed.FS

// Paths holds all the canonical paths in ~/.onlyagents
type Paths struct {
	Home        string
	Agents      string
	Connectors  string
	Channels    string
	Skills      string
	Logs        string
	Cache       string
	Marketplace string
	DBPath      string
	ConfigPath  string
	ServerPath  string
	UserPath    string
	VaultPath   string
	SkillCache  string
}

// Init ensures the home directory exists, creates subdirectories,
// and seeds default files on first run.
func Init() (*Paths, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("resolve user home: %w", err)
	}

	root := filepath.Join(homeDir, ".onlyagents")

	paths := &Paths{
		Home:        root,
		Agents:      filepath.Join(root, "agents"),
		Connectors:  filepath.Join(root, "connectors"),
		Channels:    filepath.Join(root, "channels"),
		Skills:      filepath.Join(root, "skills"),
		Logs:        filepath.Join(root, "logs"),
		Cache:       filepath.Join(root, "cache"),
		Marketplace: filepath.Join(root, "marketplace"),
		DBPath:      filepath.Join(root, "onlyagents.db"),
		ConfigPath:  filepath.Join(root, "config.yaml"),
		ServerPath:  filepath.Join(root, "server.yaml"),
		UserPath:    filepath.Join(root, "user.yaml"),
		VaultPath:   filepath.Join(root, "vault.yaml"),
		SkillCache:  filepath.Join(root, "cache", "skills"),
	}

	//Create directories
	dirs := []string{
		paths.Home,
		paths.Agents,
		paths.Connectors,
		paths.Channels,
		paths.Skills,
		paths.Logs,
		paths.Cache,
		paths.Marketplace,
	}

	for _, dir := range dirs {
		if err := os.MkdirAll(dir, 0750); err != nil {
			return nil, fmt.Errorf("create dir %s: %w", dir, err)
		}
	}

	//Seed defaults if missing
	if err := seedDefaults(paths); err != nil {
		return nil, fmt.Errorf("seed defaults: %w", err)
	}

	return paths, nil
}

// seedDefaults copies embedded default files/directories into ~/.onlyagents
// without overwriting existing user files.
func seedDefaults(paths *Paths) error {
	return copyDir("defaults", paths.Home)
}

// copyDir recursively copies embedded files from embed FS to destination.
func copyDir(src, dest string) error {
	entries, err := defaultsFS.ReadDir(src)
	if err != nil {
		return err
	}

	for _, entry := range entries {
		srcPath := filepath.Join(src, entry.Name())
		destPath := filepath.Join(dest, entry.Name())

		if entry.IsDir() {
			if err := os.MkdirAll(destPath, 0750); err != nil {
				return err
			}
			if err := copyDir(srcPath, destPath); err != nil {
				return err
			}
			continue
		}

		if err := copyFile(srcPath, destPath); err != nil {
			return err
		}
	}

	return nil
}

// copyFile copies a single file from embed FS to disk.
// It does NOT overwrite existing files.
func copyFile(src, dest string) error {
	if _, err := os.Stat(dest); err == nil {
		// File already exists, skip
		return nil
	}

	in, err := defaultsFS.Open(src)
	if err != nil {
		return err
	}
	defer func() {
		if err := in.Close(); err != nil {
			fmt.Println("error closing file:", err)
		}
	}()

	out, err := os.Create(dest) //nolint:gosec
	if err != nil {
		return err
	}
	defer func() {
		if err := out.Close(); err != nil {
			fmt.Println("error closing file:", err)
		}
	}()

	_, err = io.Copy(out, in)
	return err
}
