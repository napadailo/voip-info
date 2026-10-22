// Package htmltext renders the HTML of release notes as tview markup in the
// colors of the current theme. The standard XML decoder in non-strict mode
// reads HTML well enough: unknown tags are walked through, text is escaped.
package htmltext

import (
	"encoding/xml"
	"regexp"
	"strings"

	"github.com/rivo/tview"

	"github.com/napadailo/voip-tui/tui/theme"
)

var spaces = regexp.MustCompile(`[ \t\r\n]+`)

type renderer struct {
	plain   bool // text only: no tags, no escaping
	b       strings.Builder
	pre     int
	list    []int // per open list: -1 bullets, n = next number
	href    []string
	inLink  int
	lineLen int
}

func (r *renderer) raw(s string) {
	r.b.WriteString(s)
	if i := strings.LastIndexByte(s, '\n'); i >= 0 {
		r.lineLen = len(s) - i - 1
	} else {
		r.lineLen += len(s)
	}
}

func (r *renderer) text(s string) {
	if r.pre == 0 {
		s = spaces.ReplaceAllString(s, " ")
		if r.lineLen == 0 {
			s = strings.TrimLeft(s, " ")
		}
		if s == "" {
			return
		}
	}
	if r.plain {
		r.raw(s)
		return
	}
	r.raw(tview.Escape(s))
}

// tag writes markup (nothing in plain mode).
func (r *renderer) tag(s string) {
	if !r.plain {
		r.raw(s)
	}
}

// newline ends the current line; blank adds an empty line (paragraphs).
func (r *renderer) newline(blank bool) {
	out := r.b.String()
	if out == "" {
		return
	}
	if !strings.HasSuffix(out, "\n") {
		r.raw("\n")
	}
	if blank && !strings.HasSuffix(r.b.String(), "\n\n") {
		r.raw("\n")
	}
}

// Render converts HTML to tview markup.
func Render(html string) string { return render(html, false) }

// Text converts HTML to plain text with the same layout (lists, paragraphs).
func Text(html string) string { return render(html, true) }

func render(html string, plain bool) string {
	r := &renderer{plain: plain}
	d := xml.NewDecoder(strings.NewReader("<root>" + html + "</root>"))
	d.Strict = false
	d.AutoClose = xml.HTMLAutoClose
	d.Entity = xml.HTMLEntity
	for {
		tok, err := d.Token()
		if err != nil {
			break
		}
		switch t := tok.(type) {
		case xml.StartElement:
			r.start(t)
		case xml.EndElement:
			r.end(t.Name.Local)
		case xml.CharData:
			r.text(string(t))
		}
	}
	return strings.TrimSpace(r.b.String())
}

func attr(t xml.StartElement, name string) string {
	for _, a := range t.Attr {
		if a.Name.Local == name {
			return a.Value
		}
	}
	return ""
}

func (r *renderer) start(t xml.StartElement) {
	switch t.Name.Local {
	case "h1", "h2", "h3", "h4", "h5", "h6":
		r.newline(true)
		r.tag(theme.Tag(theme.Title) + "[::b]")
	case "p", "div", "table", "blockquote", "details":
		r.newline(true)
	case "br", "tr":
		r.newline(false)
	case "td", "th":
		if r.lineLen > 0 {
			r.raw("  ")
		}
	case "ul":
		r.newline(false)
		r.list = append(r.list, -1)
	case "ol":
		r.newline(false)
		r.list = append(r.list, 1)
	case "li":
		r.newline(false)
		indent := strings.Repeat("  ", max(0, len(r.list)-1))
		bullet := "•"
		if n := len(r.list); n > 0 && r.list[n-1] > 0 {
			bullet = itoa(r.list[n-1]) + "."
			r.list[n-1]++
		}
		r.raw(indent)
		r.tag(theme.Tag(theme.Dim))
		r.raw(bullet)
		r.tag("[-]")
		r.raw(" ")
	case "strong", "b":
		r.tag("[::b]")
	case "em", "i":
		r.tag("[::i]")
	case "code":
		if r.pre == 0 {
			r.tag(theme.Tag(theme.Accent))
		}
	case "pre":
		r.newline(true)
		r.pre++
		r.tag(theme.Tag(theme.Accent))
	case "a":
		r.href = append(r.href, attr(t, "href"))
		r.inLink++
		r.tag(theme.Tag(theme.Key))
	case "hr":
		r.newline(true)
		r.tag(theme.Tag(theme.Dim) + "──────────[-]")
		r.newline(true)
	case "img":
		if alt := attr(t, "alt"); alt != "" {
			r.text("[" + alt + "]")
		}
	}
}

func (r *renderer) end(name string) {
	switch name {
	case "h1", "h2", "h3", "h4", "h5", "h6":
		r.tag("[::-][-]")
		r.newline(false)
	case "p", "div", "table", "blockquote", "details":
		r.newline(true)
	case "ul", "ol":
		if len(r.list) > 0 {
			r.list = r.list[:len(r.list)-1]
		}
		r.newline(len(r.list) == 0)
	case "strong", "b", "em", "i":
		r.tag("[::-]")
	case "code":
		if r.pre == 0 {
			r.tag("[-]")
		}
	case "pre":
		r.pre = max(0, r.pre-1)
		r.tag("[-]")
		r.newline(true)
	case "a":
		r.tag("[-]")
		if n := len(r.href); n > 0 {
			r.href = r.href[:n-1]
		}
		r.inLink = max(0, r.inLink-1)
	}
}

func itoa(n int) string {
	s := ""
	for ; n > 0; n /= 10 {
		s = string(rune('0'+n%10)) + s
	}
	if s == "" {
		return "0"
	}
	return s
}
