package main

import (
	"strings"
	"unicode/utf8"
)

// The untrusted-content boundary.
//
// A SKILL.md is third-party input. Anything read off disk from a skills
// directory can carry terminal control bytes, and a terminal will act on
// them: ESC[2J clears the screen, ESC[1;1H homes the cursor, OSC 0
// renames the window, OSC 52 rewrites the clipboard. A skill directory
// name is worse than a file, because it is not YAML at all: it is
// whatever the user unzipped, and it reaches the list row, the status
// bar and the JSON output unchanged.
//
// So every string that came from a skill file passes through here once,
// inside ScanSkills, before the TUI, the CLI or the JSON encoder can see
// it. Escaping rather than deleting is deliberate: the user is often
// debugging a skill whose frontmatter is broken, and "the name is
// 27 bytes of junk" is only actionable if the bytes are visible.

// safeText escapes a string for single-line display: a skill name, a
// description, a category, an issue message. Newlines, carriage returns
// and tabs all become visible escapes, because a raw tab in a directory
// name is enough to break every line-oriented consumer of `list`.
func safeText(s string) string { return safeString(s, false) }

// safeBody escapes a string that keeps its line structure: a markdown
// body and a multi-line issue. Newlines stay, because they are the
// content; tabs stay, because markdown allows them. Everything else is
// escaped.
func safeBody(s string) string { return safeString(s, true) }

// safeString is the shared implementation. keepNewlines controls whether
// LF survives; a TAB never survives into single-line text.
func safeString(s string, keepNewlines bool) string {
	if isCleanText(s, keepNewlines) {
		return s
	}
	var b strings.Builder
	b.Grow(len(s) + 8)
	for i := 0; i < len(s); {
		c := s[i]
		if c >= utf8.RuneSelf {
			r, size := utf8.DecodeRuneInString(s[i:])
			switch {
			case r == utf8.RuneError && size == 1:
				// A byte that is not valid UTF-8. Left alone it makes
				// runewidth and the renderer disagree about how wide the
				// string is, which is how a frame ends up wider than the
				// terminal.
				b.WriteString("�")
				i++
			case isNonPrinting(r):
				writeEscapedRune(&b, r)
				i += size
			default:
				b.WriteString(s[i : i+size])
				i += size
			}
			continue
		}
		switch {
		case c == '\n' && keepNewlines:
			b.WriteByte('\n')
		case c == '\t' && keepNewlines:
			b.WriteByte('\t')
		case c < 0x20 || c == 0x7f:
			const hex = "0123456789abcdef"
			b.WriteString(`\x`)
			b.WriteByte(hex[c>>4])
			b.WriteByte(hex[c&0xf])
		default:
			b.WriteByte(c)
		}
		i++
	}
	return b.String()
}

// isCleanText is the fast path: most skill content has nothing to
// escape, and rebuilding every string on every scan is wasted work.
func isCleanText(s string, keepNewlines bool) bool {
	for i := 0; i < len(s); {
		c := s[i]
		switch {
		case c == '\n' || c == '\t':
			if !keepNewlines {
				return false
			}
			i++
		case c < 0x20 || c == 0x7f:
			return false
		case c >= utf8.RuneSelf:
			r, size := utf8.DecodeRuneInString(s[i:])
			if r == utf8.RuneError && size == 1 {
				return false
			}
			if isNonPrinting(r) {
				return false
			}
			i += size
		default:
			i++
		}
	}
	return true
}

// isNonPrinting reports runes that are valid UTF-8 but still not safe to
// hand a terminal or a human reader.
//
// The C1 range (U+0080-U+009F) holds 8-bit control codes, including CSI
// and OSC. A terminal in UTF-8 mode usually ignores them, but plenty of
// terminals and every multiplexer in between does not, and U+009B is
// exactly the 8-bit form of the ESC[ sequence a skill can already send.
//
// The rest are invisible: a zero-width space makes two different names
// look identical in the list, a byte-order mark hides a prefix, and the
// bidi overrides can reverse how a name reads. All of them are ways to
// make the user misread which skill they are about to delete.
func isNonPrinting(r rune) bool {
	switch {
	case r >= 0x80 && r <= 0x9f: // C1 controls
		return true
	case r == 0x200b || r == 0x200c || r == 0x200d: // zero-width space/joiners
		return true
	case r == 0x200e || r == 0x200f: // LTR/RTL marks
		return true
	case r == 0x2028 || r == 0x2029: // line/paragraph separators
		return true
	case r == 0xfeff: // byte-order mark
		return true
	case r >= 0x202a && r <= 0x202e: // bidi embedding and override
		return true
	case r >= 0x2066 && r <= 0x2069: // bidi isolates
		return true
	}
	return false
}

// writeEscapedRune writes a non-printing rune as \uXXXX so it is visible
// to the user instead of silently absent.
func writeEscapedRune(b *strings.Builder, r rune) {
	const hex = "0123456789abcdef"
	b.WriteString(`\u`)
	for shift := 12; shift >= 0; shift -= 4 {
		b.WriteByte(hex[(r>>uint(shift))&0xf])
	}
}

// safeName escapes a skill name and guarantees a non-empty result, so a
// directory called only control bytes still renders as one row with a
// readable name instead of a blank row.
func safeName(s string) string {
	out := safeText(s)
	if strings.TrimSpace(out) == "" {
		return "(unnamed)"
	}
	return out
}
