package ui

import (
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/napadailo/voip-tui/tui"

	"github.com/napadailo/voip-info/internal/catalog"
	"github.com/napadailo/voip-info/internal/hub"
	"github.com/napadailo/voip-info/internal/ietf"
	"github.com/napadailo/voip-info/internal/inventory"
	"github.com/napadailo/voip-info/internal/model"
	"github.com/napadailo/voip-info/internal/news"
	"github.com/napadailo/voip-info/internal/radar"
	"github.com/napadailo/voip-info/internal/sources/threat"
	"github.com/napadailo/voip-info/internal/watch"
)

// watchTarget is what w subscribes to on a row.
func (a *App) watchTarget(data any) (kind, id string, ok bool) {
	product := func(id string) (string, string, bool) { return watch.Products, id, id != "" }
	switch d := data.(type) {
	case *model.ProductState:
		return product(d.Product.ID)
	case releaseRef:
		return product(d.st.Product.ID)
	case *model.Release:
		return product(d.Product)
	case branchRef:
		return product(d.st.Product.ID)
	case *model.Vuln:
		return product(d.Product)
	case *inventory.Assessment:
		return product(d.ID)
	case *news.Item:
		if len(d.Products) > 0 {
			return product(d.Products[0])
		}
	case *threat.KEV:
		return product(a.trackedCVEs()[d.CVE])
	case *ietf.Doc:
		return watch.WGs, d.WG, true
	case *catalog.Vendor:
		return watch.Vendors, d.ID, true
	case *radar.Stat:
		return watch.Topics, d.Name, true
	case watchRef:
		return d.kind, d.id, true
	}
	return "", "", false
}

// watchLabel names a subscription as shown.
func (a *App) watchLabel(kind, id string) string {
	switch kind {
	case watch.Products:
		if st := a.hub.States[id]; st != nil {
			return st.Product.Name
		}
	case watch.Topics:
		return T(id)
	case watch.Vendors:
		for _, v := range a.hub.Vendors {
			if v.ID == id {
				return v.Name
			}
		}
	case watch.WGs:
		return id + " (" + T(ietf.WGDesc(id)) + ")"
	}
	return id
}

// setWatch subscribes or unsubscribes and tells about it.
func (a *App) setWatch(kind, id string, on bool) {
	if err := a.hub.SetWatch(kind, id, on); err != nil {
		a.Flash().Err(err)
		return
	}
	if on {
		a.Flash().Info(Tf("watching %s", a.watchLabel(kind, id)))
	} else {
		a.Flash().Info(Tf("not watching %s", a.watchLabel(kind, id)))
	}
	a.onChange("watch")
}

// bindStack binds w (watch the row) and W (only my stack) in a table.
func (a *App) bindStack(t *TableView) {
	t.Bind('w', func() {
		r := t.Selected()
		if r == nil {
			return
		}
		kind, id, ok := a.watchTarget(r.Data)
		if !ok {
			a.Flash().Warn(T("nothing to watch on this row"))
			return
		}
		a.setWatch(kind, id, !a.hub.Watching(kind, id))
	})
	t.Bind('W', a.toggleWatchOnly)
}

// toggleWatchOnly shows only my stack in the lists, or everything.
func (a *App) toggleWatchOnly() {
	a.watchOnly = !a.watchOnly
	v := "0"
	if a.watchOnly {
		v = "1"
		if !a.hub.HasWatch() {
			a.Flash().Warn(T("my stack is empty: w on a row or :watch <id|topic|vendor|wg>"))
		}
	}
	_ = a.hub.Store.SetSetting("watch_only", v)
	a.onChange("watch")
}

// watchedTopic reports whether a news title matches a watched topic.
func (a *App) watchedTopic(title string) bool {
	if a.hub.Watch == nil {
		return false
	}
	for _, t := range radar.Topics {
		if slices.Contains(a.hub.Watch.Topics, t.Name) && t.Re.MatchString(title) {
			return true
		}
	}
	return false
}

// watchRef is a row of the watch view.
type watchRef struct {
	kind, id string
	reason   hub.Reason
}

var whyLabels = map[string]string{hub.WhyWatch: N("subscription"), hub.WhyMine: N("my versions"), hub.WhyDep: N("dependency of %s")}

func newWatchView(a *App) *listView {
	v := newListView(a, "watch", N("My stack"), []string{N("TYPE"), N("ITEM"), N("WHY"), N("LATEST SIGNAL")},
		[]Hint{{Key: "enter", Desc: N("open")}, {Key: "w", Desc: N("unwatch")}}, nil)
	v.fill = func() []Row {
		now := time.Now()
		var rows []Row
		watched := a.hub.WatchedProducts()
		for _, st := range a.hub.Ordered() {
			r, ok := watched[st.Product.ID]
			if !ok {
				continue
			}
			why := T(whyLabels[r.Why])
			if r.Why == hub.WhyDep {
				why = Tf(whyLabels[r.Why], a.hub.States[r.Of].Product.Name)
			}
			sig := ""
			if lt := st.Latest(); lt != nil {
				sig = lt.Tag + " · " + age(lt.AgeDays())
			}
			tone := tui.ToneDim
			if r.Why != hub.WhyDep {
				tone = tui.ToneAccent
			}
			rows = append(rows, Row{Cells: []string{T("product"), st.Product.Name, why, sig},
				CellTones: map[int]tui.Tone{0: tui.ToneWarn, 2: tone, 3: tui.ToneDim},
				Data:      watchRef{watch.Products, st.Product.ID, r}, Key: "products\t" + st.Product.ID})
		}
		if w := a.hub.Watch; w != nil {
			add := func(kind, typ, id, sig string) {
				rows = append(rows, Row{Cells: []string{T(typ), a.watchLabel(kind, id), T("subscription"), sig},
					CellTones: map[int]tui.Tone{0: tui.ToneWarn, 2: tui.ToneAccent, 3: tui.ToneDim},
					Data:      watchRef{kind, id, hub.Reason{Why: hub.WhyWatch}}, Key: kind + "\t" + id})
			}
			for _, name := range w.Topics {
				sig := ""
				for _, s := range a.hub.Radar {
					if s.Name == name {
						sig = Tf("%d in 7 days, %d in 30", s.Count(7, now), s.Count(30, now))
					}
				}
				add(watch.Topics, N("topic"), name, sig)
			}
			for _, id := range w.Vendors {
				sig := ""
				if ns := a.hub.VendorNews[id]; len(ns) > 0 {
					sig = tui.Trunc(ns[0].Title, 70)
				}
				add(watch.Vendors, N("vendor"), id, sig)
			}
			for _, wg := range w.WGs {
				sig := ""
				for _, d := range a.hub.Docs {
					if d.WG == wg {
						sig = d.Label() + " · " + age(d.AgeDays())
						break
					}
				}
				add(watch.WGs, N("IETF WG"), wg, sig)
			}
		}
		return rows
	}
	v.info = func() string {
		if !a.hub.HasWatch() {
			return T("empty: w on a row or :watch <id|topic|vendor|wg>")
		}
		return ""
	}
	v.OnEnter(func(r *Row) {
		ref := r.Data.(watchRef)
		switch ref.kind {
		case watch.Products:
			a.Push(newProductView(a, a.hub.States[ref.id], "", ""))
		case watch.Topics:
			for _, s := range a.hub.Radar {
				if s.Name == ref.id {
					a.showTopic(s)
				}
			}
		case watch.Vendors:
			for i := range a.hub.Vendors {
				if a.hub.Vendors[i].ID == ref.id {
					a.showVendor(&a.hub.Vendors[i])
				}
			}
		case watch.WGs:
			a.Replace(newIETFView(a))
			a.Flash().Info(ref.id)
		}
	})
	v.Rebuild()
	return v
}

// watchCmd: ":watch" opens my stack, ":watch <query>" subscribes, ":unwatch <query>" drops.
func (a *App) watchCmd(on bool) func(string) error {
	return func(args string) error {
		q := strings.TrimSpace(args)
		if q == "" {
			if !on {
				return fmt.Errorf("%s", T("usage: :unwatch <id|topic|vendor|wg>"))
			}
			a.Replace(newWatchView(a))
			return nil
		}
		kind, id, ok := a.hub.ResolveWatch(q, T)
		if !ok {
			return fmt.Errorf("%s", Tf("nothing to watch by %q: a product or vendor id, an IETF WG or a part of a topic name", q))
		}
		a.setWatch(kind, id, on)
		return nil
	}
}
