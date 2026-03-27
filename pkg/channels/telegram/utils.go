package telegram

import (
	"context"
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"

	"github.com/mymmrac/telego"
	tu "github.com/mymmrac/telego/telegoutil"
	"github.com/sriramsme/OnlyAgents/pkg/channels"
	"github.com/sriramsme/OnlyAgents/pkg/media"
)

// ParseChatID converts a string chatID into int64 for Telego API usage.
func parseChatID(chatID string) (int64, error) {
	id, err := strconv.ParseInt(chatID, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid chatID %q: %w", chatID, err)
	}
	return id, nil
}

// markdownToTelegramHTML converts Markdown to Telegram HTML
func markdownToTelegramHTML(text string) string {
	if text == "" {
		return ""
	}

	// Extract and preserve code blocks
	codeBlocks := extractCodeBlocks(text)
	text = codeBlocks.text

	// Extract and preserve inline code
	inlineCodes := extractInlineCodes(text)
	text = inlineCodes.text

	// Escape HTML entities
	text = escapeHTML(text)

	// Convert Markdown formatting to HTML
	// Bold: **text** or __text__ -> <b>text</b>
	text = regexp.MustCompile(`\*\*(.+?)\*\*`).ReplaceAllString(text, "<b>$1</b>")
	text = regexp.MustCompile(`__(.+?)__`).ReplaceAllString(text, "<b>$1</b>")

	// Italic: *text* or _text_ -> <i>text</i>
	text = regexp.MustCompile(`\*([^\*]+)\*`).ReplaceAllString(text, "<i>$1</i>")
	text = regexp.MustCompile(`_([^_]+)_`).ReplaceAllString(text, "<i>$1</i>")

	// Strikethrough: ~~text~~ -> <s>text</s>
	text = regexp.MustCompile(`~~(.+?)~~`).ReplaceAllString(text, "<s>$1</s>")

	// Links: [text](url) -> <a href="url">text</a>
	text = regexp.MustCompile(`\[([^\]]+)\]\(([^)]+)\)`).ReplaceAllString(text, `<a href="$2">$1</a>`)

	// Restore inline code
	for i, code := range inlineCodes.codes {
		escaped := escapeHTML(code)
		text = strings.ReplaceAll(text, fmt.Sprintf("\x00IC%d\x00", i), fmt.Sprintf("<code>%s</code>", escaped))
	}

	// Restore code blocks
	for i, code := range codeBlocks.codes {
		escaped := escapeHTML(code)
		text = strings.ReplaceAll(text, fmt.Sprintf("\x00CB%d\x00", i), fmt.Sprintf("<pre>%s</pre>", escaped))
	}

	return text
}

type codeBlockMatch struct {
	text  string
	codes []string
}

func extractCodeBlocks(text string) codeBlockMatch {
	re := regexp.MustCompile("```[\\w]*\\n?([\\s\\S]*?)```")
	matches := re.FindAllStringSubmatch(text, -1)

	codes := make([]string, 0, len(matches))
	for _, match := range matches {
		codes = append(codes, match[1])
	}

	i := 0
	text = re.ReplaceAllStringFunc(text, func(m string) string {
		placeholder := fmt.Sprintf("\x00CB%d\x00", i)
		i++
		return placeholder
	})

	return codeBlockMatch{text: text, codes: codes}
}

type inlineCodeMatch struct {
	text  string
	codes []string
}

func extractInlineCodes(text string) inlineCodeMatch {
	re := regexp.MustCompile("`([^`]+)`")
	matches := re.FindAllStringSubmatch(text, -1)

	codes := make([]string, 0, len(matches))
	for _, match := range matches {
		codes = append(codes, match[1])
	}

	i := 0
	text = re.ReplaceAllStringFunc(text, func(m string) string {
		placeholder := fmt.Sprintf("\x00IC%d\x00", i)
		i++
		return placeholder
	})

	return inlineCodeMatch{text: text, codes: codes}
}

func escapeHTML(text string) string {
	text = strings.ReplaceAll(text, "&", "&amp;")
	text = strings.ReplaceAll(text, "<", "&lt;")
	text = strings.ReplaceAll(text, ">", "&gt;")
	return text
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}

const (
	telegramSafeChunkLen  = 3000  // raw markdown chars; HTML won't exceed 4096 after conversion
	telegramFileThreshold = 10000 // above this, send as .md file instead of chunking
)

// sendSingle sends or edits a single message. Existing logic, now extracted.
func (c *TelegramChannel) sendSingle(ctx context.Context, chatID int64, chatIDStr, htmlContent, agentID string) (channels.SendResult, error) {
	if msgID, ok := c.placeholders.LoadAndDelete(chatIDStr); ok {
		editMsg := tu.EditMessageText(tu.ID(chatID), msgID.(int), htmlContent).WithParseMode(telego.ModeHTML)
		if edited, err := c.bot.EditMessageText(ctx, editMsg); err == nil {
			return channels.SendResult{PlatformMessageID: fmt.Sprintf("%d", edited.MessageID)}, nil
		}
		c.logger.Debug("failed to edit placeholder, sending new message")
	}
	outMsg := tu.Message(tu.ID(chatID), htmlContent).WithParseMode(telego.ModeHTML)
	sent, err := c.bot.SendMessage(ctx, outMsg)
	if err != nil {
		outMsg.ParseMode = ""
		sent, err = c.bot.SendMessage(ctx, outMsg)
		if err != nil {
			return channels.SendResult{}, err
		}
	}
	return channels.SendResult{PlatformMessageID: fmt.Sprintf("%d", sent.MessageID)}, nil
}

// sendChunked splits long markdown into paragraph-aware chunks and sends each.
// The placeholder is used for the first chunk; remaining chunks are fresh messages.

func (c *TelegramChannel) sendChunked(ctx context.Context, chatID int64, chatIDStr, markdown, agentID string) (channels.SendResult, error) {
	chunks := splitMarkdown(markdown, telegramSafeChunkLen)
	var lastMsgID string
	for i, chunk := range chunks {
		html := markdownToTelegramHTML(chunk)
		if i == 0 {
			if msgID, ok := c.placeholders.LoadAndDelete(chatIDStr); ok {
				if edited, err := c.bot.EditMessageText(ctx, tu.EditMessageText(tu.ID(chatID), msgID.(int), html).WithParseMode(telego.ModeHTML)); err == nil {
					lastMsgID = fmt.Sprintf("%d", edited.MessageID)
					continue
				}
				c.logger.Debug("placeholder edit failed for chunk 0, sending fresh")
			}
		}
		outMsg := tu.Message(tu.ID(chatID), html).WithParseMode(telego.ModeHTML)
		sent, err := c.bot.SendMessage(ctx, outMsg)
		if err != nil {
			outMsg.ParseMode = ""
			sent, err = c.bot.SendMessage(ctx, outMsg)
			if err != nil {
				return channels.SendResult{}, fmt.Errorf("send chunk %d: %w", i, err)
			}
		}
		lastMsgID = fmt.Sprintf("%d", sent.MessageID)
	}
	return channels.SendResult{PlatformMessageID: lastMsgID}, nil
}

// sendAsFile sends the response as a .md file when it exceeds the file threshold.
// Updates the placeholder with a brief notice first.
func (c *TelegramChannel) sendAsFile(ctx context.Context, chatID int64, chatIDStr, content string) (channels.SendResult, error) {
	if msgID, ok := c.placeholders.LoadAndDelete(chatIDStr); ok {
		notice := "📄 Response is too long — sending as a file..."
		if _, err := c.bot.EditMessageText(ctx, tu.EditMessageText(tu.ID(chatID), msgID.(int), notice)); err != nil {
			c.logger.Debug("placeholder edit failed for file send", "error", err)
		}
	}
	tmp, err := os.CreateTemp("", "onlyagents-*.md")
	if err != nil {
		return channels.SendResult{}, fmt.Errorf("create temp file: %w", err)
	}
	tmpPath := tmp.Name()
	defer func() {
		if err := os.Remove(tmpPath); err != nil {
			c.logger.Warn("failed to remove temp file", "error", err)
		}
	}()
	if _, err := tmp.WriteString(content); err != nil {
		if err := tmp.Close(); err != nil {
			c.logger.Warn("failed to close temp file", "error", err)
		}
		return channels.SendResult{}, fmt.Errorf("write temp file: %w", err)
	}

	if err := tmp.Close(); err != nil {
		c.logger.Warn("failed to close temp file", "error", err)
	}

	f, err := os.Open(tmpPath) // nolint:gosec
	if err != nil {
		return channels.SendResult{}, fmt.Errorf("open temp file for send: %w", err)
	}

	defer func() {
		if err := f.Close(); err != nil {
			c.logger.Warn("failed to close temp file", "error", err)
		}
	}()

	sent, err := c.bot.SendDocument(ctx, tu.Document(tu.ID(chatID), tu.File(f)))
	if err != nil {
		return channels.SendResult{}, err
	}
	return channels.SendResult{PlatformMessageID: fmt.Sprintf("%d", sent.MessageID)}, nil
}

func (c *TelegramChannel) sendAttachment(ctx context.Context, chatID int64, att *media.Attachment) error {
	file, err := os.Open(att.LocalPath)
	if err != nil {
		return fmt.Errorf("open %s: %w", att.LocalPath, err)
	}
	defer func() {
		if err := file.Close(); err != nil {
			fmt.Printf("failed to close file %s", err)
		}
	}()

	inputFile := tu.File(file)

	if att.IsImage() {
		_, err = c.bot.SendPhoto(ctx, tu.Photo(
			telego.ChatID{ID: chatID},
			inputFile,
		).WithCaption(att.Filename))
	} else {
		_, err = c.bot.SendDocument(ctx, tu.Document(
			telego.ChatID{ID: chatID},
			inputFile,
		).WithCaption(att.Filename))
	}
	return err
}

// splitMarkdown splits text into chunks where each chunk's markdown length
// stays within maxLen, preferring paragraph then line boundaries.
func splitMarkdown(text string, maxLen int) []string {
	if len(text) <= maxLen {
		return []string{text}
	}
	var chunks []string
	var buf strings.Builder

	for para := range strings.SplitSeq(text, "\n\n") {
		sep := ""
		if buf.Len() > 0 {
			sep = "\n\n"
		}

		if buf.Len()+len(sep)+len(para) > maxLen {
			if buf.Len() > 0 {
				chunks = append(chunks, buf.String())
				buf.Reset()
			}

			if len(para) > maxLen {
				chunks = append(chunks, splitLongParagraph(para, maxLen)...)
				continue
			}
		}

		if buf.Len() > 0 {
			buf.WriteString("\n\n")
		}
		buf.WriteString(para)
	}

	if buf.Len() > 0 {
		chunks = append(chunks, buf.String())
	}
	return chunks
}

// splitLongParagraph handles paragraphs that are individually too long,
// splitting by line then hard-cutting as a last resort.
func splitLongParagraph(text string, maxLen int) []string {
	var chunks []string
	var buf strings.Builder

	for _, line := range strings.Split(text, "\n") {
		sep := ""
		if buf.Len() > 0 {
			sep = "\n"
		}
		if buf.Len()+len(sep)+len(line) > maxLen {
			if buf.Len() > 0 {
				chunks = append(chunks, buf.String())
				buf.Reset()
			}
			// Hard-cut lines that are themselves too long (e.g. minified code)
			for len(line) > maxLen {
				chunks = append(chunks, line[:maxLen])
				line = line[maxLen:]
			}
			if len(line) > 0 {
				buf.WriteString(line)
			}
		} else {
			buf.WriteString(sep)
			buf.WriteString(line)
		}
	}
	if buf.Len() > 0 {
		chunks = append(chunks, buf.String())
	}
	return chunks
}
