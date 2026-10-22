// Package export renders the Markdown of a digest in other forms (the
// markup of the screen is one) and saves it as a file. Tables become lists.
package export

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

// Format renders the inline and block marks of Markdown.
type Format struct {
	Esc             func(string) string
	Code, Bold, Ita func(string) string // the argument is rendered already, except Code
	Link            func(text, url string) string
	Quote           func(string) string
	PreOpen, PreEnd string
	PreLine         func(string) string
}

func isWord(b byte) bool {
	return b >= 'a' && b <= 'z' || b >= 'A' && b <= 'Z' || b >= '0' && b <= '9' || b >= 0x80
}

// Inline renders the inline marks of a line: `code`, [text](url), **bold**, _italic_, \-escapes.
func Inline(s string, f Format) string {
	var out, lit strings.Builder
	flush := func() { out.WriteString(f.Esc(lit.String())); lit.Reset() }
	for i := 0; i < len(s); {
		c := s[i]
		switch {
		case c == '\\' && i+1 < len(s) && strings.IndexByte("[]|_*`", s[i+1]) >= 0:
			lit.WriteByte(s[i+1])
			i += 2
			continue
		case c == '`':
			if j := strings.IndexByte(s[i+1:], '`'); j >= 0 {
				flush()
				out.WriteString(f.Code(s[i+1 : i+1+j]))
				i += j + 2
				continue
			}
		case c == '[':
			if j := strings.Index(s[i:], "]("); j > 0 {
				if k := strings.IndexByte(s[i+j+2:], ')'); k > 0 && !strings.ContainsAny(s[i+j+2:i+j+2+k], " \t") {
					flush()
					out.WriteString(f.Link(Inline(s[i+1:i+j], f), s[i+j+2:i+j+2+k]))
					i += j + 3 + k
					continue
				}
			}
		case c == '*' && strings.HasPrefix(s[i:], "**"):
			if j := strings.Index(s[i+2:], "**"); j > 0 {
				flush()
				out.WriteString(f.Bold(Inline(s[i+2:i+2+j], f)))
				i += j + 4
				continue
			}
		case c == '_' && (i == 0 || !isWord(s[i-1]) && s[i-1] != '\\'):
			if j := strings.IndexByte(s[i+1:], '_'); j > 0 && !strings.Contains(s[i+1:i+1+j], "\n") {
				end := i + 2 + j
				if end >= len(s) || !isWord(s[end]) {
					flush()
					out.WriteString(f.Ita(Inline(s[i+1:i+1+j], f)))
					i = end
					continue
				}
			}
		}
		lit.WriteByte(c)
		i++
	}
	flush()
	return out.String()
}

var (
	headRe  = regexp.MustCompile(`^(#{1,6})\s+(.*)`)
	sepRe   = regexp.MustCompile(`^\|?\s*:?-{3,}`)
	itemRe  = regexp.MustCompile(`^[-*]\s+`)
	cellsRe = regexp.MustCompile(`(^|[^\\])\|`)
)

func cells(row string) []string {
	row = strings.Trim(strings.TrimSpace(row), "|")
	var out []string
	last := 0
	for _, m := range cellsRe.FindAllStringIndex(row, -1) {
		end := m[1] - 1
		out = append(out, strings.TrimSpace(row[last:end]))
		last = m[1]
	}
	out = append(out, strings.TrimSpace(row[last:]))
	return out
}

// Convert renders Markdown as lines of the format; tables become list items.
func Convert(md string, f Format) []string {
	lines := strings.Split(md, "\n")
	var out []string
	inCode := false
	for i := 0; i < len(lines); i++ {
		ln := lines[i]
		s := strings.TrimSpace(ln)
		if strings.HasPrefix(s, "```") {
			inCode = !inCode
			if inCode {
				out = append(out, f.PreOpen)
			} else {
				out = append(out, f.PreEnd)
			}
			continue
		}
		if inCode {
			out = append(out, f.PreLine(ln))
			continue
		}
		switch m := headRe.FindStringSubmatch(s); {
		case strings.HasPrefix(s, "|"):
			// the header row and the separator are dropped, the rows become items
			if i+1 < len(lines) && sepRe.MatchString(strings.TrimSpace(lines[i+1])) {
				i++
				continue
			}
			var parts []string
			for _, c := range cells(s) {
				if c != "" {
					parts = append(parts, Inline(c, f))
				}
			}
			out = append(out, "• "+strings.Join(parts, " · "))
		case m != nil:
			if m[1] != "#" {
				out = append(out, "")
			}
			out = append(out, f.Bold(Inline(m[2], f)))
		case s == "---" || s == "***":
			out = append(out, "──────────")
		case strings.HasPrefix(s, ">"):
			out = append(out, f.Quote(Inline(strings.TrimLeft(s, "> "), f)))
		case itemRe.MatchString(s):
			indent := ""
			if strings.HasPrefix(ln, "  ") {
				indent = "   "
			}
			out = append(out, indent+"• "+Inline(itemRe.ReplaceAllString(s, ""), f))
		default:
			out = append(out, Inline(s, f))
		}
	}
	// no runs of empty lines
	var res []string
	for _, ln := range out {
		if ln == "" && (len(res) == 0 || res[len(res)-1] == "") {
			continue
		}
		res = append(res, ln)
	}
	return res
}

// SaveMarkdown writes md to dir/exports/<name>-<time>.md.
func SaveMarkdown(dir, md, name string) (string, error) {
	d := filepath.Join(dir, "exports")
	if err := os.MkdirAll(d, 0o755); err != nil {
		return "", err
	}
	p := filepath.Join(d, name+"-"+time.Now().Format("2006-01-02_1504")+".md")
	return p, os.WriteFile(p, []byte(md), 0o644)
}
