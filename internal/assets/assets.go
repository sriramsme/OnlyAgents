package assets

import (
	"embed"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/sriramsme/OnlyAgents/internal/paths"
)

//go:embed defaults/*
var FS embed.FS

func Seed(p *paths.Paths) error {
	return copyDir("defaults", p.Home)
}

// copyDir recursively copies embedded files from embed FS to destination.
func copyDir(src, dest string) error {
	entries, err := FS.ReadDir(src)
	if err != nil {
		return err
	}

	for _, entry := range entries {
		srcPath := filepath.Join(src, entry.Name())
		destPath := filepath.Join(dest, entry.Name())

		if entry.IsDir() {
			if err := os.MkdirAll(destPath, 0o750); err != nil {
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

	in, err := FS.Open(src)
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
