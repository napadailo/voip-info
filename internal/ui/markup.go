package ui

import (
	"fmt"
	"strings"

	"github.com/rivo/tview"

	"github.com/napadailo/voip-tui/tui"
	"github.com/napadailo/voip-tui/tui/theme"
)

// textTable renders aligned columns as markup; cells may hold markup
// themselves, their width is that of the visible text. Headers are
// translated.
func textTable(headers []string, rows [][]string) string {
	widths := make([]int, len(headers))
	for i, h := range headers {
		widths[i] = tui.StrWidth(T(h))
	}
	for _, r := range rows {
		for i := range headers {
			if i < len(r) {
				widths[i] = max(widths[i], tui.StrWidth(tui.StripTags(r[i])))
			}
		}
	}
	var b strings.Builder
	for i, h := range headers {
		b.WriteString(theme.Tag(theme.Header) + "[::b]" + tview.Escape(tui.Pad(T(h), widths[i])) + "[::-][-]  ")
	}
	b.WriteString("\n")
	for _, r := range rows {
		for i := range headers {
			c := ""
			if i < len(r) {
				c = r[i]
			}
			b.WriteString(c + strings.Repeat(" ", max(0, widths[i]-tui.StrWidth(tui.StripTags(c)))) + "  ")
		}
		b.WriteString("\n")
	}
	return b.String()
}

// section is a title line of a text pane (the title is translated and escaped).
func section(title string) string { return sectionMarkup(tview.Escape(T(title))) }

// sectionMarkup is a title line holding markup already.
func sectionMarkup(markup string) string {
	return "\n" + theme.Tag(theme.Title) + "[::b]" + markup + "[::-][-]\n"
}

// esc escapes text for markup.
func esc(s string) string { return tview.Escape(s) }

// bar is a progress bar of done out of total.
func bar(done, total, width int) string {
	if total <= 0 {
		return strings.Repeat("░", width)
	}
	fill := min(width, done*width/total)
	return strings.Repeat("█", fill) + strings.Repeat("░", width-fill)
}

// gradeColor colours a health grade or a 0..1 score.
func gradeColor(grade string) string {
	switch grade {
	case "A", "B":
		return theme.Tag(theme.OK)
	case "C":
		return theme.Tag(theme.Warn)
	case "D", "F":
		return theme.Tag(theme.Err)
	}
	return theme.Tag(theme.Dim)
}

// num shortens a number: 950, 3.6k, 24k, 1.2M.
func num(n int) string {
	switch {
	case n >= 1_000_000:
		return fmt.Sprintf("%.1fM", float64(n)/1e6)
	case n >= 10_000:
		return fmt.Sprintf("%.0fk", float64(n)/1e3)
	case n >= 1_000:
		return fmt.Sprintf("%.1fk", float64(n)/1e3)
	}
	return fmt.Sprint(n)
}

// tr translates a format and fills it in (args may be nil).
func tr(format string, args []any) string {
	if len(args) == 0 {
		return T(format)
	}
	return Tf(format, args...)
}
