package ui

import (
	"fmt"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/napadailo/voip-tui/tui"

	"github.com/napadailo/voip-info/internal/catalog"
	"github.com/napadailo/voip-info/internal/deps"
	"github.com/napadailo/voip-info/internal/eol"
	"github.com/napadailo/voip-info/internal/health"
	"github.com/napadailo/voip-info/internal/hub"
	"github.com/napadailo/voip-info/internal/model"
)

// listView is a table over the hub that rebuilds itself when data changes.
type listView struct {
	*TableView
	app   *App
	name  string
	hints []Hint
	fill  func() []Row
	info  func() string // more title info, after the category
	cats  bool          // 1-9 and 0 filter it by category (withCategories)
}

func newListView(a *App, name, title string, headers []string, hints []Hint, fill func() []Row) *listView {
	v := &listView{TableView: tui.NewTableView(a.App, title, headers), app: a, name: name, hints: hints, fill: fill}
	v.Bind('r', a.Reload)
	a.bindStack(v.TableView)
	a.bindBrowser(v.TableView)
	v.Rebuild()
	return v
}

// withCategories makes 1-9 and 0 filter the view by category; views of
// topics, standards or vendors have none.
func (v *listView) withCategories() *listView {
	v.cats = true
	v.app.bindCategories(v.TableView)
	return v
}

func (v *listView) Name() string      { return v.name }
func (v *listView) Table() *TableView { return v.TableView }
func (v *listView) Close()            {}
func (v *listView) Refresh()          { v.Rebuild() }
func (v *listView) Hints() []Hint {
	var h []Hint
	if v.cats {
		h = categoryHints()
	}
	h = append(h, v.hints...)
	if !slices.ContainsFunc(h, func(x Hint) bool { return x.Key == "b" }) {
		h = append(h, Hint{Key: "b", Desc: N("browser")})
	}
	return append(h, stackHints...)
}

// categoryHints name the key of every category, all first: two columns of
// the hints, as the namespaces of k9s.
func categoryHints() []Hint {
	h := []Hint{{Key: "0", Desc: N("all")}}
	for i, c := range catalog.Categories {
		h = append(h, Hint{Key: strconv.Itoa(i + 1), Desc: c.Name})
	}
	return h
}

// stackHints are the keys of my stack, in every list.
var stackHints = []Hint{{Key: "w/W", Desc: N("watch/my stack")}}

func (v *listView) Rebuild() {
	if v.fill == nil { // the constructor sets it after the table is built
		return
	}
	v.SetRows(v.fill())
	info := v.app.titleInfo()
	if !v.cats && v.app.category != "" {
		info = strings.TrimSpace(strings.TrimPrefix(info, v.app.category))
	}
	if v.info != nil {
		if s := v.info(); s != "" {
			info = strings.TrimSpace(info + " " + s)
		}
	}
	v.SetTitleInfo(info)
}

// visible are the products of the current category, of my stack only when W is on.
func (a *App) visible() []*model.ProductState {
	var watched map[string]hub.Reason
	if a.watchOnly {
		watched = a.hub.WatchedProducts()
	}
	var out []*model.ProductState
	for _, st := range a.hub.Ordered() {
		if a.category != "" && st.Product.Category != a.category {
			continue
		}
		if _, ok := watched[st.Product.ID]; watched != nil && !ok {
			continue
		}
		out = append(out, st)
	}
	return out
}

// shown reports whether a product passes the category and the my-stack filter.
func (a *App) shown(id string) bool {
	st := a.hub.States[id]
	if st == nil || a.category != "" && st.Product.Category != a.category {
		return false
	}
	if a.watchOnly {
		_, ok := a.hub.WatchedProducts()[id]
		return ok
	}
	return true
}

// titleInfo is the category and the my-stack mark of the list titles.
func (a *App) titleInfo() string {
	s := a.category
	if a.watchOnly {
		s = strings.TrimSpace(s + " " + T("my stack"))
	}
	return s
}

// partCell draws a health component as a bar and a number.
func partCell(p health.Part) (string, tui.Tone) {
	if !p.Known {
		return "·", tui.ToneDim
	}
	n := int(p.Score*5 + 0.5)
	return strings.Repeat("█", n) + strings.Repeat("░", 5-n) + fmt.Sprintf(" %3.0f", p.Score*100), gradeTone(health.Grade(int(p.Score*100)), 1)
}

func newHealthView(a *App) *listView {
	headers := []string{N("PRODUCT"), N("CATEGORY"), N("HEALTH"), N("GRADE"), N("+7D")}
	for _, k := range health.Keys {
		headers = append(headers, strings.ToUpper(health.Labels[k]))
	}
	headers = append(headers, N("DATA"))
	v := newListView(a, "health", N("Health"), headers,
		[]Hint{{Key: "enter", Desc: N("card")}, {Key: "o/O", Desc: N("sort")}}, nil)
	v.fill = func() []Row {
		var rows []Row
		for _, st := range a.visible() {
			h := a.hub.Health(st)
			cells := []string{st.Product.Name, st.Product.Category, fmt.Sprint(h.Score), h.Grade, ""}
			tones := map[int]tui.Tone{1: tui.ToneDim, 2: gradeTone(h.Grade, h.Coverage), 3: gradeTone(h.Grade, h.Coverage)}
			if d, ok := a.hub.Delta(st.Product.ID, "health", 7); ok && d != 0 {
				cells[4] = fmt.Sprintf("%+.0f", d)
				tones[4] = trendTone(d * 10)
			}
			for i, p := range h.Parts {
				c, tone := partCell(p)
				cells = append(cells, c)
				tones[5+i] = tone
			}
			cells = append(cells, fmt.Sprintf("%.0f%%", h.Coverage*100))
			if h.Coverage < 0.6 {
				tones[len(cells)-1] = tui.ToneDim
			}
			rows = append(rows, Row{Cells: cells, CellTones: tones, Data: st, Key: st.Product.ID})
		}
		sortStable(rows, func(x, y Row) bool {
			return a.hub.Health(x.Data.(*model.ProductState)).Score > a.hub.Health(y.Data.(*model.ProductState)).Score
		})
		return rows
	}
	v.OnEnter(func(r *Row) { a.Push(newProductView(a, r.Data.(*model.ProductState), "", "stats")) })
	v.withCategories()
	v.Rebuild()
	return v
}

func supportTone(status string) tui.Tone {
	switch status {
	case "full", "active?":
		return tui.ToneOK
	case "security", "stale?":
		return tui.ToneWarn
	case "eol", "eol?":
		return tui.ToneErr
	case "upcoming":
		return tui.ToneAccent
	}
	return tui.ToneDim
}

// supportLabel is the status of a branch in words.
func supportLabel(status string) string {
	if status == "" || status == "?" {
		return "?"
	}
	return T(map[string]string{"full": N("full"), "security": N("security-only"), "eol": "EOL", "upcoming": N("upcoming"),
		"active?": N("active?"), "stale?": N("rare fixes?"), "eol?": N("probably EOL")}[status])
}

func day(t time.Time) string {
	if t.IsZero() {
		return "-"
	}
	return t.Format("2006-01-02")
}

type branchRef struct {
	st *model.ProductState
	b  eol.Branch
}

func newEOLView(a *App) *listView {
	v := newListView(a, "eol", N("Support"), []string{N("PRODUCT"), N("BRANCH"), N("TYPE"), N("LATEST"), N("X.0 RELEASED"),
		N("FULL UNTIL"), N("EOL"), N("DAYS LEFT"), N("STATUS"), N("NEXT CHANGE"), N("SOURCE")},
		[]Hint{{Key: "enter", Desc: N("card")}}, nil)
	v.fill = func() []Row {
		now := time.Now()
		var rows []Row
		for _, st := range a.visible() {
			for _, b := range a.hub.Support(st) {
				latest := "-"
				if b.Latest != nil {
					latest = b.Latest.Tag
				}
				cells := []string{st.Product.Name, b.Branch, b.Kind, latest, day(b.Released), day(b.FullUntil), day(b.EOL), "",
					supportLabel(b.Status), "", T(b.Source)}
				tones := map[int]tui.Tone{8: supportTone(b.Status), 10: tui.ToneDim}
				if b.Kind == "LTS" {
					tones[2] = tui.ToneAccent
				}
				if left, ok := b.DaysLeft(now); ok {
					cells[7] = fmt.Sprint(left)
					if left <= 90 {
						tones[7] = tui.ToneErr
					}
				}
				if what, when, ok := b.Next(now); ok {
					d := int(when.Sub(now).Hours() / 24)
					cells[9] = Tf("%s in %d days", T(what), d)
					switch {
					case d <= 90:
						tones[9] = tui.ToneErr
					case d <= 365:
						tones[9] = tui.ToneWarn
					}
				}
				rows = append(rows, Row{Cells: cells, CellTones: tones, Data: branchRef{st, b}, Key: st.Product.ID + "\t" + b.Branch})
			}
		}
		// policies first, then by product
		sortStable(rows, func(x, y Row) bool {
			bx, by := x.Data.(branchRef), y.Data.(branchRef)
			return bx.b.Source == "policy" && by.b.Source != "policy"
		})
		return rows
	}
	v.OnEnter(func(r *Row) { a.Push(newProductView(a, r.Data.(branchRef).st, "", "roadmap")) })
	v.withCategories()
	v.Rebuild()
	return v
}

func newDepsView(a *App) *listView {
	kindLabel := func(k string) string {
		return T(map[string]string{deps.Bundled: N("bundled"), deps.Linked: N("library"), deps.Optional: N("optional"),
			deps.Runtime: N("service alongside"), deps.Platform: N("platform")}[k])
	}
	v := newListView(a, "deps", N("Dependencies"), []string{N("PRODUCT"), "→", N("DEPENDS ON"), N("KIND"),
		N("UPSTREAM LATEST"), N("AGE"), N("IMPACT")},
		[]Hint{{Key: "enter", Desc: N("card")}}, nil)
	v.fill = func() []Row {
		var rows []Row
		for _, e := range a.hub.Graph.Edges {
			src, dst := a.hub.States[e.Src], a.hub.States[e.Dst]
			if c := a.category; c != "" && src.Product.Category != c && dst.Product.Category != c {
				continue
			}
			if a.watchOnly && !a.shown(e.Src) {
				continue
			}
			cells := []string{src.Product.Name, "→", dst.Product.Name, kindLabel(e.Kind), "-", "", fmt.Sprintf("+%d", len(a.hub.Graph.Impact(e.Dst)))}
			tones := map[int]tui.Tone{1: tui.ToneDim, 2: tui.ToneAccent}
			if deps.Strong(e.Kind) {
				tones[3] = tui.ToneWarn
			} else {
				tones[3] = tui.ToneDim
			}
			if lt := dst.Latest(); lt != nil {
				cells[4], cells[5] = lt.Tag, age(lt.AgeDays())
				tones[5] = ageTone(lt.AgeDays())
			}
			rows = append(rows, Row{Cells: cells, CellTones: tones, Data: src, Key: e.Src + "\t" + e.Dst})
		}
		return rows
	}
	v.OnEnter(func(r *Row) { a.Push(newProductView(a, r.Data.(*model.ProductState), "", "deps")) })
	v.withCategories()
	v.Rebuild()
	return v
}
