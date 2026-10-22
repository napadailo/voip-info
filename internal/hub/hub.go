// Package hub holds the data and analytics of voip-info without any UI: the
// terminal interface and the headless commands share it.
//
// Loaders work in background goroutines and hand their results to Apply,
// which the UI sets to run on its goroutine (QueueUpdateDraw); the state is
// changed only there, so views read it without locks.
package hub

import (
	"context"
	"sync"
	"time"

	"github.com/napadailo/voip-info/internal/catalog"
	"github.com/napadailo/voip-info/internal/deps"
	"github.com/napadailo/voip-info/internal/fetch"
	"github.com/napadailo/voip-info/internal/inventory"
	"github.com/napadailo/voip-info/internal/model"
	"github.com/napadailo/voip-info/internal/sources/github"
	"github.com/napadailo/voip-info/internal/sources/threat"
	"github.com/napadailo/voip-info/internal/store"
	"github.com/napadailo/voip-info/internal/watch"
)

// Hub is the shared state.
type Hub struct {
	Store   *store.Store
	Fetcher *fetch.Fetcher

	Products  []*catalog.Product
	States    map[string]*model.ProductState
	Graph     *deps.Graph
	X0        map[int]time.Time // when the Asterisk majors came out
	KEV       map[string]*threat.KEV
	EPSS      map[string]threat.EPSS
	ThreatErr error

	NVDDone, NVDTotal int // progress of the NVD load
	DataDir           string
	Inventory         inventory.Inventory
	Watch             *watch.Watch
	vulns             map[string][]*model.Vuln

	Feeds   []catalog.Feed
	Vendors []catalog.Vendor
	Events  []catalog.Event
	info    // news, market, standards, adoption

	Loaded      int // products whose releases arrived in the current load
	metricsDone int // products whose API metrics arrived
	LastUpdate  time.Time

	// Apply runs a state change; the UI runs it on its goroutine.
	Apply func(func())
	// OnChange is told what changed ("releases", ...), after Apply ran it.
	OnChange func(topic string)
}

// New loads the catalog.
func New(s *store.Store) (*Hub, error) {
	products, err := catalog.Products()
	if err != nil {
		return nil, err
	}
	h := &Hub{Store: s, Fetcher: fetch.New(s), Products: products, States: map[string]*model.ProductState{}, Graph: deps.New(products)}
	for _, p := range products {
		h.States[p.ID] = &model.ProductState{Product: p}
	}
	if h.Feeds, err = catalog.Feeds(); err != nil {
		return nil, err
	}
	if h.Vendors, err = catalog.Vendors(); err != nil {
		return nil, err
	}
	if h.Events, err = catalog.Events(); err != nil {
		return nil, err
	}
	var mu sync.Mutex
	h.Apply = func(f func()) { mu.Lock(); defer mu.Unlock(); f() }
	h.OnChange = func(string) {}
	return h, nil
}

// Ordered returns the states in catalog order.
func (h *Hub) Ordered() []*model.ProductState {
	out := make([]*model.ProductState, len(h.Products))
	for i, p := range h.Products {
		out[i] = h.States[p.ID]
	}
	return out
}

func (h *Hub) changed(topic string) {
	h.Apply(func() { h.OnChange(topic) })
}

// LoadReleases fetches the releases of every product, 8 at a time, and
// applies each as it arrives.
func (h *Hub) LoadReleases(ctx context.Context, force bool) {
	h.Apply(func() {
		h.Loaded = 0
		for _, st := range h.States {
			st.Loading = true
		}
	})
	var wg sync.WaitGroup
	sem := make(chan struct{}, 8)
	for _, p := range h.Products {
		wg.Add(1)
		go func(p *catalog.Product) {
			defer wg.Done()
			sem <- struct{}{}
			rels, err := github.FetchReleases(ctx, h.Fetcher, p, force)
			<-sem
			h.Apply(func() {
				st := h.States[p.ID]
				if rels != nil || err == nil {
					st.Releases = rels
				}
				st.Err, st.Loading = err, false
				h.initSeen(st)
				h.Loaded++
				if h.Loaded%6 == 0 {
					h.OnChange("releases")
				}
			})
		}(p)
	}
	wg.Wait()
	h.Apply(func() { h.LastUpdate = time.Now() })
	h.changed("releases")
}

// initSeen sets the seen marker; on the first run everything older than
// 14 days counts as seen, so the fresh releases show as new right away.
func (h *Hub) initSeen(st *model.ProductState) {
	tag, ok := h.Store.Seen(st.Product.ID)
	if !ok && len(st.Releases) > 0 {
		cutoff := time.Now().Add(-14 * 24 * time.Hour)
		base := st.Releases[len(st.Releases)-1]
		for _, r := range st.Releases {
			if r.Date.Before(cutoff) {
				base = r
				break
			}
		}
		tag, ok = base.Tag, true
		_ = h.Store.MarkSeen(st.Product.ID, tag)
	}
	st.SeenTag, st.HasSeen = tag, ok
}

// MarkSeen marks the newest release of st as seen.
func (h *Hub) MarkSeen(st *model.ProductState) {
	if len(st.Releases) > 0 {
		st.SeenTag, st.HasSeen = st.Releases[0].Tag, true
		_ = h.Store.MarkSeen(st.Product.ID, st.SeenTag)
	}
}

// NewCount is the number of unseen releases over all products.
func (h *Hub) NewCount() int {
	n := 0
	for _, st := range h.States {
		n += len(st.Unseen())
	}
	return n
}
