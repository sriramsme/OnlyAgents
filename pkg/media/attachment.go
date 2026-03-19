package media

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/google/uuid"
)

// Attachment represents a file received from a channel or produced by an agent.
// By the time an Attachment appears in any payload, the file is already on disk
// at LocalPath — no raw bytes travel beyond the channel layer.
type Attachment struct {
	// ID is a UUID assigned at download time. Stable across all payload hops.
	ID string `json:"id"`

	// LocalPath is the absolute path to the file on disk.
	// Set by the channel layer; read by the LLM provider at call time.
	LocalPath string `json:"local_path"`

	// Filename is the original name from the platform (best-effort).
	Filename string `json:"filename"`

	// MIMEType is detected from file bytes, not trusted from the platform.
	MIMEType string `json:"mime_type"`

	// Size in bytes.
	Size int64 `json:"size"`

	// CreatedAt is when the file was saved to disk.
	CreatedAt time.Time `json:"created_at"`
}

// IsImage reports whether this attachment is an image the LLM can reason about visually.
func (a *Attachment) IsImage() bool {
	return IsImage(a.MIMEType)
}

// IsDocument reports whether this attachment is a document (PDF etc.).
func (a *Attachment) IsDocument() bool {
	return IsDocument(a.MIMEType)
}

// attachmentFromPath builds an Attachment from a path a skill wrote to disk.
// The file must already exist — skills are responsible for writing it.
func AttachmentFromPath(path string) (*Attachment, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, fmt.Errorf("stat %s: %w", path, err)
	}
	safePath := filepath.Clean(path)
	data, err := os.ReadFile(safePath) //nolint:gosec
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	return &Attachment{
		ID:        uuid.NewString(),
		LocalPath: path,
		Filename:  filepath.Base(path),
		MIMEType:  DetectMIMEType(data, path),
		Size:      info.Size(),
		CreatedAt: time.Now(),
	}, nil
}
