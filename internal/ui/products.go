package ui

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/napadailo/voip-tui/tui"

	"github.com/napadailo/voip-info/internal/analytics"
	"github.com/napadailo/voip-info/internal/model"
)

// ProductsView lists the products.
type ProductsView struct {
	*TableView
	app *App
}

func newProductsView(a *App) *ProductsView {
	v := &ProductsView{app: a, TableView: tui.NewTableView(a.App, N("Products"), []string{
		N("NAME"), N("CATEGORY"), N("LATEST"), N("CHANGES"), N("PRE"), N("AGE"), N("NEW"), N("CADENCE,D"),
		N("STARS"), N("COMMITS"), N("CVE 1Y"), N("CVE DEPS"), N("HEALTH"), N("STATUS"),
	})}
	v.OnEnter(func(r *Row) { a.Push(newProductView(a, r.Data.(*model.ProductState), "", "")) })
	a.bindBrowser(v.TableView)
	v.Bind('r', a.Reload)
	v.Bind('n', a.MarkAllSeen)
	a.bindCategories(v.TableView)
	a.bindStack(v.TableView)
	v.Rebuild()
	return v
}

func (v *ProductsView) Name() string      { return "products" }
func (v *ProductsView) Table() *TableView { return v.TableView }
func (v *ProductsView) Close()            {}
func (v *ProductsView) Refresh()          { v.Rebuild() }

func (v *ProductsView) Hints() []Hint {
	return append(categoryHints(), Hint{Key: "enter", Desc: N("card")}, Hint{Key: "b", Desc: N("browser")}, Hint{Key: "r", Desc: N("reload")},
		Hint{Key: "n", Desc: N("mark seen")}, Hint{Key: "o/O", Desc: N("sort")}, stackHints[0])
}

// ageTone colours an age: fresh green, old yellow, abandoned red.
func ageTone(days int) tui.Tone {
	switch {
	case days <= 30:
		return tui.ToneOK
	case days <= 180:
		return tui.ToneFg
	case days <= 365:
		return tui.ToneWarn
	}
	return tui.ToneErr
}

func trendTone(pct float64) tui.Tone {
	switch {
	case pct > 10:
		return tui.ToneOK
	case pct < -10:
		return tui.ToneErr
	}
	return tui.ToneFg
}

// gradeTone colours a health grade; a low data coverage is dimmed.
func gradeTone(grade string, coverage float64) tui.Tone {
	switch {
	case coverage < 0.6:
		return tui.ToneDim
	case grade == "A" || grade == "B":
		return tui.ToneOK
	case grade == "C":
		return tui.ToneWarn
	}
	return tui.ToneErr
}

// releaseStatuses are the values of analytics.ReleaseStats.Status, translated when shown.
var releaseStatuses = []string{N("hot"), N("active"), N("slow"), N("stale")}

func statusTone(s string) tui.Tone {
	switch s {
	case "hot", "active":
		return tui.ToneOK
	case "slow":
		return tui.ToneWarn
	case "stale":
		return tui.ToneErr
	}
	return tui.ToneDim
}

// Rebuild fills the table from the hub, newest release first.
func (v *ProductsView) Rebuild() {
	states := v.app.visible()
	watched := v.app.hub.WatchedProducts()
	ageOf := func(st *model.ProductState) int {
		if lt := st.Latest(); lt != nil {
			return lt.AgeDays()
		}
		return 1 << 30
	}
	sortStable(states, func(x, y *model.ProductState) bool { return ageOf(x) < ageOf(y) })
	rows := make([]Row, 0, len(states))
	for _, st := range states {
		s := analytics.Stats(st)
		lt, pre := st.Latest(), st.LatestPre()
		name := st.Product.Name
		if _, mine := v.app.hub.Inventory[st.Product.ID]; mine {
			name = "◆ " + name
		} else if _, ok := watched[st.Product.ID]; ok {
			name = "◉ " + name
		}
		if st.Loading {
			name += " …"
		}
		if st.Err != nil && len(st.Releases) == 0 {
			name += " !"
		}
		const (
			cLatest = 2 + iota
			cChanges
			cPre
			cAge
			cNew
			cCadence
			cStars
			cCommits
			cCVE
			cDeps
			cHealth
			cStatus
		)
		cells := make([]string, cStatus+1)
		srt := map[int]float64{}
		cells[0], cells[1], cells[cLatest], cells[cStatus] = name, st.Product.Category, "-", T(s.Status)
		tones := map[int]tui.Tone{1: tui.ToneDim, cPre: tui.ToneAccent, cCommits: tui.ToneAccent, cStatus: statusTone(s.Status)}
		if lt != nil {
			cells[cLatest] = lt.Tag
			if lt.Commit {
				cells[cLatest] = "⎇ " + lt.Tag
				tones[cLatest] = tui.ToneDim
			}
			cells[cChanges], tones[cChanges] = badge(lt)
			cells[cAge], srt[cAge] = age(lt.AgeDays()), float64(lt.AgeDays())
			tones[cAge] = ageTone(lt.AgeDays())
		}
		if pre != nil {
			cells[cPre] = pre.Tag
		}
		if n := len(st.Unseen()); n > 0 {
			cells[cNew] = fmt.Sprintf("+%d", n)
			tones[cNew], tones[0] = tui.ToneOK, tui.ToneOK
		}
		if s.Cadence > 0 {
			cells[cCadence] = fmt.Sprintf("%.0f", s.Cadence)
		}
		// stars and their growth over a week; green while they grow
		if r := st.GH.Repo; r != nil {
			cells[cStars], srt[cStars] = num(r.Stars), float64(r.Stars)
			if d, ok := v.app.hub.Delta(st.Product.ID, "stars", 7); ok && d != 0 {
				cells[cStars] += fmt.Sprintf(" %+.0f", d)
				if d > 0 {
					tones[cStars] = tui.ToneOK
				}
			}
		}
		// commits of the last 8 weeks and the trend of 12 weeks against the 12 before, toned by it
		if w := st.GH.Weekly; len(w) >= 8 {
			cells[cCommits] = analytics.Sparkline(w[len(w)-8:])
			if _, _, pct, ok := analytics.CommitTrend(w); ok {
				cells[cCommits] += fmt.Sprintf(" %+.0f%%", pct)
				srt[cCommits] = pct
				tones[cCommits] = trendTone(pct)
			}
		}
		cells[cCVE], tones[cCVE] = v.app.cveYearCell(st)
		cells[cDeps], tones[cDeps] = v.app.upstreamCell(st)
		if n, _, ok := strings.Cut(cells[cDeps], " "); ok {
			srt[cDeps], _ = strconv.ParseFloat(n, 64)
		}
		if hl := v.app.hub.Health(st); hl.Grade != "?" {
			cells[cHealth] = fmt.Sprint(hl.Score)
			tones[cHealth] = gradeTone(hl.Grade, hl.Coverage)
		}
		if st.Err != nil && len(st.Releases) == 0 {
			tones[0] = tui.ToneErr
		}
		rows = append(rows, Row{Cells: cells, CellTones: tones, Markup: map[int]bool{cChanges: true}, Sort: srt, Data: st, Key: st.Product.ID})
	}
	v.SetRows(rows)
	v.SetTitleInfo(v.app.titleInfo())
}

func first[T any](s []T, n int) []T {
	if len(s) > n {
		return s[:n]
	}
	return s
}

func sortStable[T any](s []T, less func(a, b T) bool) {
	for i := 1; i < len(s); i++ {
		for j := i; j > 0 && less(s[j], s[j-1]); j-- {
			s[j], s[j-1] = s[j-1], s[j]
		}
	}
}
