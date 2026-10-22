package ui

import (
	"fmt"
	"strings"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"

	"github.com/napadailo/voip-tui/tui"
	"github.com/napadailo/voip-tui/tui/theme"

	"github.com/napadailo/voip-info/internal/model"
)

// vulnRow is a vulnerability in a list with what it means here: where it
// is fixed, why it is listed.
type vulnRow struct {
	vu     *model.Vuln
	status string
	tone   tui.Tone
}

// vulnListView is a summary on top and vulnerabilities under it, a row each:
// enter opens one, b its advisory.
type vulnListView struct {
	*tview.Flex
	app     *App
	name    string
	head    *tview.TextView
	table   *TableView
	summary func() string
	rows    func() []vulnRow
}

func newVulnListView(a *App, name string, summary func() string, rows func() []vulnRow) *vulnListView {
	v := &vulnListView{app: a, name: name, summary: summary, rows: rows}
	v.head = tview.NewTextView().SetDynamicColors(true).SetWrap(true).SetWordWrap(true)
	v.head.SetBorder(true).SetBorderPadding(0, 0, 1, 1)
	v.table = tui.NewTableView(a.App, N("Vulnerabilities"), []string{N("PUBLISHED"), N("SEVERITY"), "CVSS", N("EPSS"), "KEV", "ID",
		N("STATUS"), N("SUMMARY")})
	v.table.OnEnter(func(r *Row) { a.showVuln(r.Data.(vulnRow).vu) })
	v.table.Bind('b', func() {
		if r := v.table.Selected(); r != nil {
			a.OpenURL(r.Data.(vulnRow).vu.URL)
		}
	})
	v.Flex = tview.NewFlex().SetDirection(tview.FlexRow).AddItem(v.head, 3, 0, false).AddItem(v.table, 0, 1, true)
	v.Rebuild()
	return v
}

func (v *vulnListView) Name() string      { return v.name }
func (v *vulnListView) Table() *TableView { return v.table }
func (v *vulnListView) Close()            {}
func (v *vulnListView) Refresh()          { v.Rebuild() }
func (v *vulnListView) Rerender()         { v.Rebuild() }

func (v *vulnListView) Hints() []Hint {
	return []Hint{{Key: "enter", Desc: N("open")}, {Key: "b", Desc: N("browser")}}
}

// Focus goes to the list.
func (v *vulnListView) Focus(delegate func(p tview.Primitive)) { delegate(v.table) }

// Draw applies the current theme.
func (v *vulnListView) Draw(screen tcell.Screen) {
	v.head.SetBackgroundColor(theme.Bg).SetBorderColor(theme.Border)
	v.head.SetTextColor(theme.Fg)
	v.Flex.SetBackgroundColor(theme.Bg)
	v.Flex.Draw(screen)
}

// Rebuild refreshes the summary and the list; the summary pane is as tall as its text.
func (v *vulnListView) Rebuild() {
	text := strings.TrimRight(v.summary(), "\n")
	v.head.SetText(text)
	v.Flex.ResizeItem(v.head, min(strings.Count(text, "\n")+3, 14), 0)
	v.table.SetRows(v.app.vulnRows(v.rows()))
}

// vulnRows are the table rows of vulnerabilities; the row data is a vulnRow.
func (a *App) vulnRows(list []vulnRow) []Row {
	h := a.hub
	rows := make([]Row, 0, len(list))
	for _, x := range list {
		vu := x.vu
		score := "-"
		if vu.Score > 0 {
			score = fmt.Sprintf("%.1f", vu.Score)
		}
		cells := []string{vu.Published.Format("2006-01-02"), vu.Severity, score, "", "", vu.ID, x.status, tui.Trunc(vu.Summary, 100)}
		tones := map[int]tui.Tone{1: severityTone(vu.Severity), 5: tui.ToneAccent, 6: x.tone}
		srt := map[int]float64{}
		cells[3], tones[3] = epssCell(h.EPSSOf(vu))
		if e, ok := h.EPSSOf(vu); ok {
			srt[3] = e.Score
		}
		if h.KEVOf(vu) != nil {
			cells[4], tones[4] = T("yes"), tui.ToneErr
		}
		rows = append(rows, Row{Cells: cells, CellTones: tones, Sort: srt, Data: x, Key: vu.ID + "\t" + vu.Product})
	}
	return rows
}
