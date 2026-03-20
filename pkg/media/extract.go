package media

import "unicode/utf8"

// MaxInlineBytes is the ceiling for inlining file content into an LLM message.
// Beyond this size content is truncated — models have context limits and very
// large files should be chunked or summarised by a skill instead.
const MaxInlineBytes = 100_000 // ~25k tokens

// ExtractText returns file content as a plain string when data is valid UTF-8.
// Returns false for binary files — callers should handle those with a native
// file part (images, PDFs) or an unsupported note.
//
// Covers all text-based formats without enumerating MIME types:
// source code, JSON, YAML, CSV, Markdown, plain text, config files, etc.
func ExtractText(data []byte) (string, bool) {
	if !utf8.Valid(data) {
		return "", false
	}
	text := string(data)
	if len(text) > MaxInlineBytes {
		text = text[:MaxInlineBytes] + "\n\n[...truncated — showing first 100KB of content]"
	}
	return text, true
}
