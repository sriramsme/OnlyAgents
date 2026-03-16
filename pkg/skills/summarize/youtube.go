package summarize

import (
	"encoding/json"
	"encoding/xml"
	"fmt"
	"html"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/sriramsme/OnlyAgents/pkg/logger"
)

var (
	ytVideoIDRegex    = regexp.MustCompile(`(?:v=|youtu\.be/|/embed/|/v/)([a-zA-Z0-9_-]{11})`)
	ytCaptionURLRegex = regexp.MustCompile(`"captionTracks":\s*(\[.*?\])`)
)

type ytCaptionTrack struct {
	BaseURL      string `json:"baseUrl"`
	LanguageCode string `json:"languageCode"`
	Kind         string `json:"kind"`
}

type timedTextTranscript struct {
	XMLName xml.Name `xml:"transcript"`
	Texts   []struct {
		Start string `xml:"start,attr"`
		Dur   string `xml:"dur,attr"`
		Text  string `xml:",chardata"`
	} `xml:"text"`
}

// fetchYouTubeTranscript extracts a plain-text transcript from a YouTube URL.
// It tries the page's embedded caption tracks first (no API key needed).
// Returns the transcript text and a boolean indicating whether a transcript was found.
func fetchYouTubeTranscript(videoURL string) (transcript string, found bool, err error) {
	videoID, err := extractVideoID(videoURL)
	if err != nil {
		return "", false, err
	}

	// Fetch the YouTube watch page
	client := &http.Client{Timeout: 15 * time.Second}
	req, err := http.NewRequest(http.MethodGet,
		"https://www.youtube.com/watch?v="+videoID, nil)
	if err != nil {
		return "", false, fmt.Errorf("youtube: build request: %w", err)
	}
	req.Header.Set("User-Agent",
		"Mozilla/5.0 (compatible; OnlyAgents/1.0)")
	req.Header.Set("Accept-Language", "en-US,en;q=0.9")

	resp, err := client.Do(req)
	if err != nil {
		return "", false, fmt.Errorf("youtube: fetch page: %w", err)
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			logger.Log.Error("failed to close response body", "error", err)
		}
	}()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return "", false, fmt.Errorf("youtube: read page: %w", err)
	}

	// Extract caption tracks from the ytInitialPlayerResponse embedded in the page
	tracks, err := extractCaptionTracks(string(body))
	if err != nil || len(tracks) == 0 {
		return "", false, nil // No transcript available — not an error
	}

	track := selectTrack(tracks)
	if track == nil {
		return "", false, nil
	}

	text, err := fetchCaptionText(client, track.BaseURL)
	if err != nil {
		return "", false, fmt.Errorf("youtube: fetch captions: %w", err)
	}

	return text, true, nil
}

func extractVideoID(rawURL string) (string, error) {
	matches := ytVideoIDRegex.FindStringSubmatch(rawURL)
	if len(matches) < 2 {
		return "", fmt.Errorf("youtube: could not extract video ID from %q", rawURL)
	}
	return matches[1], nil
}

func extractCaptionTracks(pageHTML string) ([]ytCaptionTrack, error) {
	matches := ytCaptionURLRegex.FindStringSubmatch(pageHTML)
	if len(matches) < 2 {
		return nil, fmt.Errorf("youtube: no captionTracks found in page")
	}

	// The captured JSON may have escaped unicode — unescape it
	raw := strings.ReplaceAll(matches[1], `\u0026`, "&")

	var tracks []ytCaptionTrack
	if err := json.Unmarshal([]byte(raw), &tracks); err != nil {
		return nil, fmt.Errorf("youtube: parse captionTracks: %w", err)
	}
	return tracks, nil
}

// selectTrack prefers English manual captions, then English auto, then first available.
func selectTrack(tracks []ytCaptionTrack) *ytCaptionTrack {
	for i, t := range tracks {
		if t.LanguageCode == "en" && t.Kind != "asr" {
			return &tracks[i]
		}
	}
	for i, t := range tracks {
		if t.LanguageCode == "en" {
			return &tracks[i]
		}
	}
	if len(tracks) > 0 {
		return &tracks[0]
	}
	return nil
}

func fetchCaptionText(client *http.Client, baseURL string) (string, error) {
	// Request plain text format
	u, err := url.Parse(baseURL)
	if err != nil {
		return "", err
	}
	q := u.Query()
	q.Set("fmt", "xml")
	u.RawQuery = q.Encode()

	resp, err := client.Get(u.String())
	if err != nil {
		return "", err
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			logger.Log.Error("failed to close response body", "error", err)
		}
	}()

	data, err := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
	if err != nil {
		return "", err
	}

	var tt timedTextTranscript
	if err := xml.Unmarshal(data, &tt); err != nil {
		return "", fmt.Errorf("parse caption xml: %w", err)
	}

	var sb strings.Builder
	for _, seg := range tt.Texts {
		text := html.UnescapeString(strings.TrimSpace(seg.Text))
		if text != "" && text != "[Music]" && text != "[Applause]" {
			sb.WriteString(text)
			sb.WriteByte(' ')
		}
	}

	return strings.TrimSpace(sb.String()), nil
}
