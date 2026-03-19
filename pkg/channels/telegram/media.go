package telegram

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/mymmrac/telego"
	"github.com/sriramsme/OnlyAgents/pkg/media"
)

// extractAttachments detects any files in a Telegram message, downloads them,
// saves them to the media store, and returns the resulting Attachments.
// Text-only messages return (nil, nil).
// Partial failures are logged and skipped — one bad file does not kill the message.
func (c *TelegramChannel) extractAttachments(ctx context.Context, message *telego.Message) ([]*media.Attachment, error) {
	specs := collectFileSpecs(message)
	if len(specs) == 0 {
		return nil, nil
	}

	attachments := make([]*media.Attachment, 0, len(specs))
	for _, spec := range specs {
		att, err := c.downloadAndSave(ctx, spec)
		if err != nil {
			// Log and continue — don't fail the whole message for one bad file.
			c.logger.Warn("failed to download attachment",
				"file_id", spec.fileID,
				"filename", spec.filename,
				"err", err)
			continue
		}
		attachments = append(attachments, att)
	}

	return attachments, nil
}

// fileSpec is an internal descriptor for a single file in a Telegram message.
type fileSpec struct {
	fileID   string
	filename string // best-effort; empty for photos
}

// collectFileSpecs inspects a Telegram message and returns a fileSpec for each
// file present. Handles: photos (largest size), documents, audio, video, voice,
// video notes, and stickers.
func collectFileSpecs(message *telego.Message) []fileSpec {
	var specs []fileSpec

	// Photo — Telegram sends multiple resolutions; take the largest (last).
	if len(message.Photo) > 0 {
		largest := message.Photo[len(message.Photo)-1]
		specs = append(specs, fileSpec{
			fileID:   largest.FileID,
			filename: largest.FileID + ".jpg", // Telegram doesn't give photo filenames
		})
	}

	// Document (includes GIFs sent as files, PDFs, code files, etc.)
	if message.Document != nil {
		filename := message.Document.FileName
		if filename == "" {
			filename = message.Document.FileID
		}
		specs = append(specs, fileSpec{
			fileID:   message.Document.FileID,
			filename: filename,
		})
	}

	// Audio
	if message.Audio != nil {
		filename := message.Audio.FileName
		if filename == "" {
			filename = message.Audio.FileID + ".mp3"
		}
		specs = append(specs, fileSpec{
			fileID:   message.Audio.FileID,
			filename: filename,
		})
	}

	// Video
	if message.Video != nil {
		filename := message.Video.FileName
		if filename == "" {
			filename = message.Video.FileID + ".mp4"
		}
		specs = append(specs, fileSpec{
			fileID:   message.Video.FileID,
			filename: filename,
		})
	}

	// Voice note
	if message.Voice != nil {
		specs = append(specs, fileSpec{
			fileID:   message.Voice.FileID,
			filename: message.Voice.FileID + ".ogg",
		})
	}

	// Video note (round video)
	if message.VideoNote != nil {
		specs = append(specs, fileSpec{
			fileID:   message.VideoNote.FileID,
			filename: message.VideoNote.FileID + ".mp4",
		})
	}

	// Sticker
	if message.Sticker != nil {
		ext := ".webp"
		if message.Sticker.IsAnimated {
			ext = ".tgs"
		}
		if message.Sticker.IsVideo {
			ext = ".webm"
		}
		specs = append(specs, fileSpec{
			fileID:   message.Sticker.FileID,
			filename: message.Sticker.FileID + ext,
		})
	}

	return specs
}

// downloadAndSave resolves a Telegram file_id to a download URL, fetches the
// bytes, and persists them via the media store.
func (c *TelegramChannel) downloadAndSave(ctx context.Context, spec fileSpec) (*media.Attachment, error) {
	// Step 1: resolve file_id → FilePath via Telegram's getFile API
	telegoFile, err := c.bot.GetFile(ctx, &telego.GetFileParams{FileID: spec.fileID})
	if err != nil {
		return nil, fmt.Errorf("getFile %s: %w", spec.fileID, err)
	}

	if telegoFile.FilePath == "" {
		return nil, fmt.Errorf("getFile returned empty path for %s", spec.fileID)
	}

	// Step 2: build the download URL and fetch bytes
	downloadURL := c.bot.FileDownloadURL(telegoFile.FilePath)
	data, err := fetchBytes(ctx, downloadURL)
	if err != nil {
		return nil, fmt.Errorf("download %s: %w", spec.filename, err)
	}

	// Step 3: save to disk via media store
	att, err := media.Save(data, spec.filename)
	if err != nil {
		return nil, fmt.Errorf("save %s: %w", spec.filename, err)
	}

	c.logger.Debug("attachment saved",
		"id", att.ID,
		"filename", att.Filename,
		"mime_type", att.MIMEType,
		"size_bytes", att.Size)

	return att, nil
}

// fetchBytes downloads the content at url and returns the raw bytes.
// Uses a dedicated http.Client with a generous timeout for large files.
func fetchBytes(ctx context.Context, url string) ([]byte, error) {
	httpClient := &http.Client{Timeout: 60 * time.Second}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("http get: %w", err)
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			fmt.Printf("failed to close response body %s", err)
		}
	}()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status %d", resp.StatusCode)
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read body: %w", err)
	}

	return data, nil
}
