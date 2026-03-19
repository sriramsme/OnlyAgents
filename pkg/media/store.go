package media

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/google/uuid"
)

var mediaDir string

// Init sets the media staging directory. Must be called once at kernel startup
// before any channel begins handling messages.
func Init(dir string) error {
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return fmt.Errorf("media: ensure dir %s: %w", dir, err)
	}
	mediaDir = dir
	return nil
}

// Save writes data to disk and returns a fully populated Attachment.
// filename is the original name from the platform — used for extension-based
// MIME fallback and as a human-readable suffix in the stored filename.
func Save(data []byte, filename string) (*Attachment, error) {
	if mediaDir == "" {
		return nil, fmt.Errorf("media: Init has not been called")
	}
	if len(data) == 0 {
		return nil, fmt.Errorf("media: refusing to save empty file")
	}

	id := uuid.NewString()
	mimeType := DetectMIMEType(data, filename)
	storedName := id + "_" + sanitizeFilename(filename)
	localPath := filepath.Join(mediaDir, storedName)

	if err := os.WriteFile(localPath, data, 0o640); err != nil { //nolint:gosec
		return nil, fmt.Errorf("media: write %s: %w", localPath, err)
	}

	return &Attachment{
		ID:        id,
		LocalPath: localPath,
		Filename:  filename,
		MIMEType:  mimeType,
		Size:      int64(len(data)),
		CreatedAt: time.Now(),
	}, nil
}

// Remove deletes an attachment file from disk.
// Safe to call even if the file no longer exists.
func Remove(a *Attachment) error {
	if a == nil || a.LocalPath == "" {
		return nil
	}
	if err := os.Remove(a.LocalPath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("media: remove %s: %w", a.LocalPath, err)
	}
	return nil
}

// TODO: Cleanup(ttl time.Duration) error
// Delete files in mediaDir older than ttl.
// Intended to run as a background goroutine in the kernel on startup.

// sanitizeFilename strips path separators and unsafe characters from a filename.
func sanitizeFilename(name string) string {
	if name == "" {
		return "file"
	}
	name = filepath.Base(name)
	safe := make([]byte, 0, len(name))
	for i := range len(name) {
		c := name[i]
		if (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') ||
			(c >= '0' && c <= '9') || c == '.' || c == '-' || c == '_' {
			safe = append(safe, c)
		} else {
			safe = append(safe, '_')
		}
	}
	if len(safe) == 0 {
		return "file"
	}
	return string(safe)
}
