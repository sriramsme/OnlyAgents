package summarize

import (
	"fmt"
	"strings"

	"github.com/sriramsme/OnlyAgents/pkg/tools"
)

// lengthTarget maps SummarizeLength to a target character count and prose description.
type lengthTarget struct {
	chars int
	label string
}

var lengthTargets = map[tools.SummarizeLength]lengthTarget{
	tools.SummarizeLengthShort:  {900, "concise (~900 characters). Use 1-3 short paragraphs. No headers."},
	tools.SummarizeLengthMedium: {1800, "moderate (~1800 characters). Use 3-5 paragraphs. Light structure where helpful."},
	tools.SummarizeLengthLong:   {4200, "detailed (~4200 characters). Use headers and sections. Cover all key points."},
	tools.SummarizeLengthXL:     {9000, "comprehensive (~9000 characters). Full structured breakdown with headers, sections, and key takeaways."},
}

func resolveLength(l tools.SummarizeLength) lengthTarget {
	if t, ok := lengthTargets[l]; ok {
		return t
	}
	return lengthTargets[tools.SummarizeLengthMedium]
}

func buildPrompt(content, sourceDesc string, length tools.SummarizeLength, language, focus string) string {
	target := resolveLength(length)
	var sb strings.Builder
	sb.WriteString("You are a precise summarizer. Summarize the following content.\n\n")
	fmt.Fprintf(&sb, "Length: %s\n", target.label)
	if language != "" && language != "auto" {
		fmt.Fprintf(&sb, "Output language: %s\n", language)
	} else {
		sb.WriteString("Output language: match the source language\n")
	}
	if focus != "" {
		fmt.Fprintf(&sb, "Focus: %s\n", focus)
	}
	if sourceDesc != "" {
		fmt.Fprintf(&sb, "Source: %s\n", sourceDesc)
	}

	sb.WriteString("\nRules:\n")
	sb.WriteString("- If content is shorter than the target length, return it as-is without padding.\n")
	sb.WriteString("- Do not invent information not present in the source.\n")
	sb.WriteString("- Do not include meta-commentary like 'This article discusses...' — just summarize.\n")
	sb.WriteString("- Preserve technical terms, names, and numbers exactly.\n")

	sb.WriteString("\n---\n")
	sb.WriteString(content)

	return sb.String()
}

// chunkText splits text into chunks of maxChars, respecting paragraph breaks where possible.
func chunkText(text string, maxChars int) []string {
	if len(text) <= maxChars {
		return []string{text}
	}
	var chunks []string
	for len(text) > 0 {
		if len(text) <= maxChars {
			chunks = append(chunks, text)
			break
		}
		// Break on newline boundary near the limit.
		cut := strings.LastIndexByte(text[:maxChars], '\n')
		if cut <= 0 {
			cut = maxChars
		}
		chunks = append(chunks, strings.TrimSpace(text[:cut]))
		text = strings.TrimSpace(text[cut:])
	}
	return chunks
}

// mergePrompt builds the final reduction prompt when multiple chunks were summarized.
func mergePrompt(partials []string, length tools.SummarizeLength, language, focus string) string {
	combined := strings.Join(partials, "\n\n---\n\n")
	return buildPrompt(
		combined,
		"(merged partial summaries — produce one cohesive final summary)",
		length,
		language,
		focus,
	)
}
