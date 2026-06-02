package format

import (
	"regexp"
	"strings"
	"unicode/utf8"
)

// Temporary markers (invalid UTF-8 sequences, safe from collisions)
// \xff alone is invalid UTF-8, guarantees no collision with real text
const (
	boldStart = "\xff\x01"
	boldEnd   = "\xff\x02"
	codeStart = "\xff\x03"
	codeEnd   = "\xff\x04"
)

var (
	// Must be in order: multi-char patterns first to avoid partial matches
	boldRe          = regexp.MustCompile(`\*\*(.+?)\*\*`)
	italicRe        = regexp.MustCompile(`\*(.+?)\*`)
	strikethroughRe = regexp.MustCompile(`~~(.+?)~~`)
	codeBlockRe     = regexp.MustCompile("(?s)```(.+?)```")
	inlineCodeRe    = regexp.MustCompile("`(.+?)`")
	linkRe          = regexp.MustCompile(`\[([^\]]+)\]\(([^)]+)\)`)
	headerRe        = regexp.MustCompile(`(?m)^#{1,6}\s+(.+)$`)
	hrRe            = regexp.MustCompile(`(?m)^[-*_]{3,}\s*$`)
	bulletRe        = regexp.MustCompile(`(?m)^\s*[-*+]\s+(.+)$`)
	blockquoteRe    = regexp.MustCompile(`(?m)^>\s?(.*)$`)
	multiNewlineRe  = regexp.MustCompile(`\n{3,}`)
)

// ToWhatsApp converts Markdown text to WhatsApp-compatible formatting.
// WhatsApp supports: *bold*, _italic_, ~strikethrough~, `monospace`
func ToWhatsApp(md string) string {
	if md == "" {
		return ""
	}

	// 1. Protect code blocks/inline from other conversions
	md = codeBlockRe.ReplaceAllString(md, codeStart+"$1"+codeEnd)
	md = inlineCodeRe.ReplaceAllString(md, codeStart+"$1"+codeEnd)

	// 2. Bold: **text** → temporary marker → *text*
	md = boldRe.ReplaceAllString(md, boldStart+"$1"+boldEnd)

	// 3. Italic: *text* → _text_ (only catches remaining single *)
	md = italicRe.ReplaceAllString(md, "_$1_")

	// 4. Restore bold markers → WhatsApp bold *text*
	md = strings.ReplaceAll(md, boldStart, "*")
	md = strings.ReplaceAll(md, boldEnd, "*")

	// 5. Strikethrough: ~~text~~ → ~text~
	md = strikethroughRe.ReplaceAllString(md, "~$1~")

	// 6. Links: [text](url) → text (url)
	md = linkRe.ReplaceAllString(md, "$1 ($2)")

	// 7. Headers: # text → *text* (bold in WA)
	md = headerRe.ReplaceAllString(md, "*$1*")

	// 8. Horizontal rules → empty
	md = hrRe.ReplaceAllString(md, "")

	// 9. Bullet points: - text → • text
	md = bulletRe.ReplaceAllString(md, "• $1")

	// 10. Blockquotes: > text → text
	md = blockquoteRe.ReplaceAllString(md, "$1")

	// 11. Restore code markers → backticks
	md = strings.ReplaceAll(md, codeStart, "`")
	md = strings.ReplaceAll(md, codeEnd, "`")

	// Clean up
	md = strings.TrimSpace(md)
	md = multiNewlineRe.ReplaceAllString(md, "\n\n")

	return md
}

func init() {
	// Verify markers are single rune safe
	if utf8.ValidString(boldStart) || utf8.ValidString(boldEnd) ||
		utf8.ValidString(codeStart) || utf8.ValidString(codeEnd) {
		panic("format: markers must be invalid UTF-8 to avoid collisions")
	}
}
