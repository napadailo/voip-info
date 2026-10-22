package ui

import (
	"strings"
	"time"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"

	"github.com/napadailo/voip-tui/tui"
	"github.com/napadailo/voip-tui/tui/theme"
)

// mention is a row of a mentions view: a news item, a release, a document.
type mention struct {
	date                   time.Time
	source, title, product string
	url                    string
}

// mentionsView is a summary on top and the mentions under it, a row each:
// b opens one in the browser.
type mentionsView struct {
	*tview.Flex
	app     *App
	name    string
	head    *tview.TextView
	table   *TableView
	summary func() string    // card markup
	rows    func() []mention // newest first
}

func newMentionsView(a *App, name string, summary func() string, rows func() []mention) *mentionsView {
	v := &mentionsView{app: a, name: name, summary: summary, rows: rows}
	v.head = tview.NewTextView().SetDynamicColors(true).SetWrap(true).SetWordWrap(true)
	v.head.SetBorder(true).SetBorderPadding(0, 0, 1, 1)
	v.table = tui.NewTableView(a.App, N("Mentions"), []string{N("DATE"), N("SOURCE"), N("TITLE"), N("PRODUCT")})
	open := func() {
		if r := v.table.Selected(); r != nil {
			a.OpenURL(r.Data.(mention).url)
		}
	}
	v.table.Bind('b', open)
	v.Flex = tview.NewFlex().SetDirection(tview.FlexRow).AddItem(v.head, 3, 0, false).AddItem(v.table, 0, 1, true)
	v.Rebuild()
	return v
}

func (v *mentionsView) Name() string      { return v.name }
func (v *mentionsView) Table() *TableView { return v.table }
func (v *mentionsView) Close()            {}
func (v *mentionsView) Refresh()          { v.Rebuild() }
func (v *mentionsView) Rerender()         { v.Rebuild() }

func (v *mentionsView) Hints() []Hint {
	return []Hint{{Key: "b", Desc: N("browser")}}
}

// Focus goes to the list.
func (v *mentionsView) Focus(delegate func(p tview.Primitive)) { delegate(v.table) }

// Draw applies the current theme.
func (v *mentionsView) Draw(screen tcell.Screen) {
	v.head.SetBackgroundColor(theme.Bg).SetBorderColor(theme.Border)
	v.head.SetTextColor(theme.Fg)
	v.Flex.SetBackgroundColor(theme.Bg)
	v.Flex.Draw(screen)
}

// Rebuild refreshes the summary and the list; the summary pane is as tall as its text.
func (v *mentionsView) Rebuild() {
	text := strings.TrimRight(v.summary(), "\n")
	v.head.SetText(text)
	v.Flex.ResizeItem(v.head, min(strings.Count(text, "\n")+3, 14), 0)
	var rows []Row
	for _, m := range v.rows() {
		d := int(time.Since(m.date).Hours() / 24)
		rows = append(rows, Row{Cells: []string{m.date.Format("2006-01-02"), tui.Trunc(m.source, 28), tui.Trunc(m.title, 120), m.product},
			CellTones: map[int]tui.Tone{0: ageTone(d), 1: tui.ToneDim, 3: tui.ToneAccent}, Data: m, Key: m.url})
	}
	v.table.SetRows(rows)
}
