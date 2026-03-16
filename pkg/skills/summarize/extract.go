package summarize

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/pdfcpu/pdfcpu/pkg/api"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
	"github.com/sriramsme/OnlyAgents/pkg/logger"
	"golang.org/x/net/html"
)

const (
	fetchTimeout = 20 * time.Second
	maxBodyBytes = 4 << 20 // 4MB
	maxTextChars = 64000
)

// fetchURL retrieves and extracts readable text from a URL.
func fetchURL(url string) (title, text string, err error) {
	client := &http.Client{Timeout: fetchTimeout}
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return "", "", fmt.Errorf("invalid url %q: %w", url, err)
	}
	req.Header.Set("User-Agent", "OnlyAgents/1.0 (+https://github.com/sriramsme/OnlyAgents)")
	req.Header.Set("Accept", "text/html,application/xhtml+xml,text/plain;q=0.9")

	resp, err := client.Do(req)
	if err != nil {
		return "", "", fmt.Errorf("fetch failed: %w", err)
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			logger.Log.Error("failed to close response body", "error", err)
		}
	}()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", "", fmt.Errorf("HTTP %d from %s", resp.StatusCode, url)
	}

	ct := resp.Header.Get("Content-Type")
	if strings.Contains(ct, "application/pdf") {
		body, err := io.ReadAll(io.LimitReader(resp.Body, maxBodyBytes))
		if err != nil {
			return "", "", fmt.Errorf("read pdf body: %w", err)
		}
		text, err := extractPDFBytes(body)
		return url, text, err
	}

	if !strings.Contains(ct, "text/") && !strings.Contains(ct, "application/xhtml") {
		return "", "", fmt.Errorf("unsupported content type %q — only HTML and PDF are supported", ct)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, maxBodyBytes))
	if err != nil {
		return "", "", fmt.Errorf("read body: %w", err)
	}

	title, text = extractHTML(body)
	return title, truncate(text, maxTextChars), nil
}

// readFile reads a local .txt, .md, or .pdf file and returns its text content.
func readFile(path string) (string, error) {
	ext := strings.ToLower(filepath.Ext(path))

	safePath := filepath.Clean(path)
	switch ext {
	case ".txt", ".md":
		data, err := os.ReadFile(safePath) // #nosec G304
		if err != nil {
			return "", fmt.Errorf("read file: %w", err)
		}
		return truncate(string(data), maxTextChars), nil

	case ".pdf":
		data, err := os.ReadFile(safePath) // #nosec G304
		if err != nil {
			return "", fmt.Errorf("read pdf: %w", err)
		}
		return extractPDFBytes(data)

	default:
		return "", fmt.Errorf("unsupported file type %q — supported: .txt, .md, .pdf", ext)
	}
}

// extractHTML parses HTML bytes and returns title + readable text.
func extractHTML(body []byte) (title, text string) {
	doc, err := html.Parse(bytes.NewReader(body))
	if err != nil {
		return "", strings.TrimSpace(stripTags(string(body)))
	}

	var sb strings.Builder
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.ElementNode {
			switch n.Data {
			case "script", "style", "noscript", "nav", "footer",
				"header", "aside", "form", "button", "iframe":
				return
			case "title":
				if n.FirstChild != nil {
					title = strings.TrimSpace(n.FirstChild.Data)
				}
			}
		}
		if n.Type == html.TextNode {
			t := strings.TrimSpace(n.Data)
			if t != "" {
				sb.WriteString(t)
				sb.WriteByte('\n')
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(doc)

	return title, strings.TrimSpace(sb.String())
}

// extractPDFBytes extracts text from a PDF byte slice using pdfcpu.
func extractPDFBytes(data []byte) (string, error) {
	conf := model.NewDefaultConfiguration()
	conf.ValidationMode = model.ValidationRelaxed

	rs := bytes.NewReader(data)
	ctx, err := api.ReadContext(rs, conf)
	if err != nil {
		return "", fmt.Errorf("pdf read failed: %w", err)
	}

	var sb strings.Builder
	for pageNr := 1; pageNr <= ctx.PageCount; pageNr++ {
		reader, err := pdfcpu.ExtractPageContent(ctx, pageNr)
		if err != nil {
			return "", fmt.Errorf("pdf extraction failed for page %d: %w", pageNr, err)
		}

		if reader == nil {
			continue
		}

		content, err := io.ReadAll(reader)
		if err != nil {
			return "", fmt.Errorf("failed to read page content %d: %w", pageNr, err)
		}

		sb.WriteString(strings.TrimSpace(string(content)))
		sb.WriteString("\n\n")
	}

	return truncate(strings.TrimSpace(sb.String()), maxTextChars), nil
}

func stripTags(s string) string {
	inTag := false
	var sb strings.Builder
	for _, r := range s {
		switch {
		case r == '<':
			inTag = true
		case r == '>':
			inTag = false
		case !inTag:
			sb.WriteRune(r)
		}
	}
	return sb.String()
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max]
}
