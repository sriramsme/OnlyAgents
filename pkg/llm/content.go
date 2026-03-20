package llm

// ContentPartType identifies what kind of content a part carries.
type ContentPartType string

const (
	ContentPartTypeText     ContentPartType = "text"
	ContentPartTypeImage    ContentPartType = "image"
	ContentPartTypeDocument ContentPartType = "document"
)

// ContentPart is a single typed block within a multimodal message.
// By the time a ContentPart is constructed, Data is already populated —
// file bytes are read from disk in prepareExecution, not lazily by the provider.
type ContentPart struct {
	Type     ContentPartType
	Text     string // set when Type == ContentPartTypeText
	Filename string // original filename — used by providers for file parts
	MIMEType string // set when Type == ContentPartTypeImage or ContentPartTypeDocument
	Data     []byte // raw file bytes; provider base64-encodes as needed
}

// TextPart is a convenience constructor for a plain text content part.
func TextPart(text string) ContentPart {
	return ContentPart{
		Type: ContentPartTypeText,
		Text: text,
	}
}

// ImagePart is a convenience constructor for an image content part.
func ImagePart(filename, mimeType string, data []byte) ContentPart {
	return ContentPart{
		Type:     ContentPartTypeImage,
		Filename: filename,
		MIMEType: mimeType,
		Data:     data,
	}
}

// DocumentPart is a convenience constructor for a document content part.
func DocumentPart(filename, mimeType string, data []byte) ContentPart {
	return ContentPart{
		Type:     ContentPartTypeDocument,
		Filename: filename,
		MIMEType: mimeType,
		Data:     data,
	}
}
