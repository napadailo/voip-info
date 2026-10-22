package ui

import (
	"github.com/gdamore/tcell/v2"

	"github.com/napadailo/voip-info/internal/catalog"
	"github.com/napadailo/voip-info/internal/ietf"
	"github.com/napadailo/voip-info/internal/inventory"
	"github.com/napadailo/voip-info/internal/model"
	"github.com/napadailo/voip-info/internal/news"
	"github.com/napadailo/voip-info/internal/radar"
	"github.com/napadailo/voip-info/internal/sources/threat"
	"github.com/napadailo/voip-info/internal/watch"
)

// urlOf is the page b opens for a row: the release, the vulnerability, the
// news item, or the product's repository.
func (a *App) urlOf(data any) string {
	product := func(id string) string {
		if st := a.hub.States[id]; st != nil {
			return st.Product.URL()
		}
		return ""
	}
	switch d := data.(type) {
	case *model.ProductState:
		return d.Product.URL()
	case releaseRef:
		return d.r.URL
	case *model.Release:
		return d.URL
	case branchRef:
		return d.st.Product.URL()
	case *model.Vuln:
		return d.URL
	case *inventory.Assessment:
		return product(d.ID)
	case *news.Item:
		return d.URL
	case *threat.KEV:
		return nvdURL(d.CVE)
	case *ietf.Doc:
		return d.URL()
	case *catalog.Vendor:
		return vendorURL(d)
	case *radar.Stat:
		if len(d.Mentions) > 0 {
			return d.Mentions[0].URL
		}
	case mention:
		return d.url
	case vulnRow:
		return d.vu.URL
	case eventRef:
		if d.meeting != nil {
			return d.meeting.URL()
		}
		return d.event.URL
	case watchRef:
		switch d.kind {
		case watch.Products:
			return product(d.id)
		case watch.WGs:
			return "https://datatracker.ietf.org/wg/" + d.id + "/about/"
		case watch.Vendors:
			for i := range a.hub.Vendors {
				if a.hub.Vendors[i].ID == d.id {
					return vendorURL(&a.hub.Vendors[i])
				}
			}
		case watch.Topics:
			for _, s := range a.hub.Radar {
				if s.Name == d.id && len(s.Mentions) > 0 {
					return s.Mentions[0].URL
				}
			}
		}
	}
	return ""
}

// bindBrowser binds b to open the page of the selected row.
func (a *App) bindBrowser(t *TableView) {
	t.Bind('b', func() {
		r := t.Selected()
		if r == nil {
			return
		}
		if u := a.urlOf(r.Data); u != "" {
			a.OpenURL(u)
		} else {
			a.Flash().Warn(T("no page to open for this row"))
		}
	})
}

// browserKey opens url on b in a text pane of the card.
func (v *ProductView) browserKey(url func() string) func(*tcell.EventKey) *tcell.EventKey {
	return func(ev *tcell.EventKey) *tcell.EventKey {
		if ev.Key() == tcell.KeyRune && ev.Rune() == 'b' {
			v.app.OpenURL(url())
			return nil
		}
		return ev
	}
}

// tabURL is the page of a text tab of the card.
func (v *ProductView) tabURL(id string) string {
	base := v.st.Product.URL()
	switch id {
	case "roadmap":
		return base + "/milestones"
	}
	return base
}
