package ui

import (
	"strconv"
	"strings"
	"time"

	"github.com/rivo/tview"

	"github.com/napadailo/voip-tui/tui"
	"github.com/napadailo/voip-tui/tui/theme"

	"github.com/napadailo/voip-info/internal/digest"
	"github.com/napadailo/voip-info/internal/export"
)

// markup renders Markdown as tview markup: links stay clickable where the terminal can.
var markup = export.Format{
	Esc:     tview.Escape,
	Code:    func(s string) string { return tui.Paint(theme.Accent, s) },
	Bold:    func(s string) string { return "[::b]" + s + "[::-]" },
	Ita:     func(s string) string { return theme.Tag(theme.Dim) + s + "[-]" },
	Link:    func(t, u string) string { return "[:::" + u + "]" + t + "[:::-]" },
	Quote:   func(s string) string { return theme.Tag(theme.Dim) + "│ " + s + "[-]" },
	PreOpen: "", PreEnd: "",
	PreLine: tview.Escape,
}

// mdMarkup renders a Markdown page; headings get the title color.
func mdMarkup(md string) string {
	f := markup
	f.Bold = func(s string) string { return "[::b]" + s + "[::-]" }
	lines := export.Convert(md, f)
	for i, ln := range lines {
		if strings.HasPrefix(ln, "[::b]") && strings.HasSuffix(ln, "[::-]") {
			lines[i] = theme.Tag(theme.Title) + ln + "[-]"
		}
	}
	return strings.Join(lines, "\n")
}

// digestOptions reads "[days] [mine|all]".
func (a *App) digestOptions(args []string) (o digest.Options, name string) {
	days, mine := a.cfg.Digest.Days, a.cfg.Digest.Mine
	for _, w := range args {
		switch w {
		case "mine", "my":
			mine = true
		case "all":
			mine = false
		default:
			if n, err := strconv.Atoi(w); err == nil && n > 0 {
				days = n
			}
		}
	}
	name = "digest"
	if mine {
		name = "digest-mine"
	}
	return digest.Options{Since: time.Now().AddDate(0, 0, -days), Label: digest.Label(days), Mine: mine}, name
}

// digestCmd: ":digest [days] [mine|all]" shows the digest; e saves it.
func (a *App) digestCmd(args string) error {
	o, name := a.digestOptions(strings.Fields(args))
	md := digest.Build(a.hub, o)
	tv := tui.NewTextViewer(a.App, T("Digest"), "markup")
	tv.SetWrap(true)
	tv.SetContent(mdMarkup(md), "markup")
	tv.Keys['e'] = func() { a.saveDigest(md, name) }
	tv.Extra = []Hint{{Key: "e", Desc: N("export .md")}}
	a.Push(tv)
	return nil
}

func (a *App) saveDigest(md, name string) {
	p, err := export.SaveMarkdown(a.hub.DataDir, md, name)
	if err != nil {
		a.Flash().Err(err)
		return
	}
	a.Flash().Info(Tf("saved %s", p))
}

// exportCmd: ":export [days] [mine|all]" saves the digest as Markdown.
func (a *App) exportCmd(args string) error {
	o, name := a.digestOptions(strings.Fields(args))
	a.saveDigest(digest.Build(a.hub, o), name)
	return nil
}
