package format

import (
	"regexp"
	"strings"
)

var (
	boldRe          = regexp.MustCompile(`\*\*(.+?)\*\*`)
	italicRe        = regexp.MustCompile(`(?<!\*)\*(?!\*)(.+?)(?<!\*)\*(?!\*)`)
	strikethroughRe = regexp.MustCompile(`~~(.+?)~~`)
	codeBlockRe     = regexp.MustCompile("```[^`\n]*\n?([\\s\\S]*?)```")
	linkRe          = regexp.MustCompile(`\[([^\]]+)\]\(([^)]+)\)`)
	headerRe        = regexp.MustCompile(`(?m)^#{1,6}\s+(.+)$`)
	hrRe            = regexp.MustCompile(`(?m)^[-*_]{3,}\s*$`)
	bulletRe        = regexp.MustCompile(`(?m)^\s*[-*+]\s+(.+)$`)
	blockquoteRe    = regexp.MustCompile(`(?m)^>\s?(.*)$`)
)

// ToWhatsApp converts Markdown text to WhatsApp-compatible formatting.
func ToWhatsApp(md string) string {
	if md == "" {
		return ""
	}

	// 1. Code blocks → inline code (WA doesn't support blocks)
	md = codeBlockRe.ReplaceAllString(md, "`$1`")

	// 2. Bold: **text** → *text*
	md = boldRe.ReplaceAllString(md, "*$1*")

	// 3. Italic: *text* → _text_
	md = italicRe.ReplaceAllString(md, "_$1_")

	// 4. Strikethrough: ~~text~~ → ~text~
	md = strikethroughRe.ReplaceAllString(md, "~$1~")

	// 5. Links: [text](url) → text (url)
	md = linkRe.ReplaceAllString(md, "$1 ($2)")

	// 6. Headers: # text → **text** (bold in WA)
	md = headerRe.ReplaceAllString(md, "*$1*")

	// 7. Horizontal rules → empty
	md = hrRe.ReplaceAllString(md, "")

	// 8. Bullet points: - text → • text
	md = bulletRe.ReplaceAllString(md, "• $1")

	// 9. Blockquotes: > text → text
	md = blockquoteRe.ReplaceAllString(md, "$1")

	// Clean up extra blank lines
	md = strings.TrimSpace(md)
	md = regexp.MustCompile(`\n{3,}`).ReplaceAllString(md, "\n\n")

	return md
}
