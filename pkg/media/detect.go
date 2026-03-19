package media

import (
	"fmt"
	"mime"
	"net/http"
	"path/filepath"
	"strings"
)

// DetectMIMEType sniffs the MIME type from the first 512 bytes of data.
// Falls back to extension-based detection, then "application/octet-stream".
// Never trusts the MIME type claimed by the platform.
func DetectMIMEType(data []byte, filename string) string {
	// Sniff from content — most reliable
	if len(data) > 0 {
		sniffBuf := data
		if len(sniffBuf) > 512 {
			sniffBuf = data[:512]
		}
		detected := http.DetectContentType(sniffBuf)
		// http.DetectContentType can return "application/octet-stream" as a
		// fallback — in that case prefer extension-based detection below.
		if detected != "application/octet-stream" {
			return detected
		}
	}

	// Fall back to file extension
	if filename != "" {
		ext := strings.ToLower(filepath.Ext(filename))
		if ext != "" {
			if mtype := mime.TypeByExtension(ext); mtype != "" {
				// Strip parameters (e.g. "text/html; charset=utf-8" → "text/html")
				mediaType, _, err := mime.ParseMediaType(mtype)
				if err != nil {
					fmt.Printf("failed to parse media type %s: %s", mtype, err)
					return mtype
				}
				if mediaType != "" {
					return mediaType
				}
			}
		}
	}

	return "application/octet-stream"
}

// IsImage reports whether a MIME type represents an image a multimodal LLM can reason about.
func IsImage(mimeType string) bool {
	switch mimeType {
	case "image/jpeg", "image/png", "image/gif", "image/webp":
		return true
	}
	return false
}

// IsDocument reports whether a MIME type represents a document the LLM can read.
func IsDocument(mimeType string) bool {
	switch mimeType {
	case "application/pdf",
		"text/plain",
		"text/markdown",
		"text/csv":
		return true
	}
	return false
}

// IsSupportedByLLM reports whether an attachment of this MIME type can be
// sent directly to an LLM. Unsupported types can still be stored and
// referenced by path, but the agent will need a tool (e.g. read_file) to act on them.
func IsSupportedByLLM(mimeType string) bool {
	return IsImage(mimeType) || IsDocument(mimeType)
}
