package hub

import (
	"context"
	"sync"
	"time"

	"github.com/napadailo/voip-info/internal/adoption"
	"github.com/napadailo/voip-info/internal/ietf"
	"github.com/napadailo/voip-info/internal/market"
	"github.com/napadailo/voip-info/internal/model"
	"github.com/napadailo/voip-info/internal/news"
	"github.com/napadailo/voip-info/internal/radar"
)

// info is what the hub knows beyond the products: news, the market,
// standards and events, the use of the packages, the topic radar.
type info struct {
	News       []*news.Item // newest first, tagged with products
	NewsErrs   []string
	NewsLoaded bool

	Quotes     map[string]*market.Quote
	VendorNews map[string][]*news.Item // newest first
	MarketDone bool

	Docs      []*ietf.Doc
	Meetings  []*ietf.Meeting
	EventNews map[string][]*news.Item
	StdDone   bool

	Adoption     map[string][]*adoption.Stat
	AdoptionDone bool

	Radar []*radar.Stat
}

// LoadInfo loads news, the market, standards and adoption side by side.
func (h *Hub) LoadInfo(ctx context.Context, force bool) {
	var wg sync.WaitGroup
	for _, f := range []func(context.Context, bool){h.LoadNews, h.LoadMarket, h.LoadStandards, h.LoadAdoption} {
		wg.Add(1)
		go func() { defer wg.Done(); f(ctx, force) }()
	}
	wg.Wait()
}

// LoadNews reads every feed, keeps one item per link and tags the products.
func (h *Hub) LoadNews(ctx context.Context, force bool) {
	type result struct {
		items []*news.Item
		err   error
	}
	res := make([]result, len(h.Feeds))
	var wg sync.WaitGroup
	for i, fd := range h.Feeds {
		wg.Add(1)
		go func() {
			defer wg.Done()
			items, err := news.Fetch(ctx, h.Fetcher, fd, force)
			res[i] = result{items, err}
		}()
	}
	wg.Wait()
	seen := map[string]bool{}
	var items []*news.Item
	var errs []string
	for _, r := range res {
		if r.err != nil {
			errs = append(errs, r.err.Error())
		}
		for _, n := range r.items {
			if n.URL != "" && !seen[n.URL] {
				seen[n.URL] = true
				items = append(items, n)
			}
		}
	}
	news.Sort(items)
	news.Tag(items, news.Matchers(h.Products))
	h.Apply(func() { h.News, h.NewsErrs, h.NewsLoaded = items, errs, true })
	h.RebuildRadar()
	h.changed("news")
}

// LoadMarket reads the quotes and the news of every vendor.
func (h *Hub) LoadMarket(ctx context.Context, force bool) {
	quotes := map[string]*market.Quote{}
	vnews := map[string][]*news.Item{}
	var mu sync.Mutex
	var wg sync.WaitGroup
	for _, v := range h.Vendors {
		wg.Add(1)
		go func() {
			defer wg.Done()
			var q *market.Quote
			if v.Ticker != "" {
				q = market.FetchQuote(ctx, h.Fetcher, v.Ticker, force)
			}
			items, _ := news.Fetch(ctx, h.Fetcher, market.VendorFeed(v), force)
			items = news.TitleHas(items, news.FirstWord(v.Name))
			news.Sort(items)
			mu.Lock()
			if q != nil {
				quotes[v.ID] = q
			}
			vnews[v.ID] = items
			mu.Unlock()
		}()
	}
	wg.Wait()
	h.Apply(func() { h.Quotes, h.VendorNews, h.MarketDone = quotes, vnews, true })
	h.RebuildRadar()
	h.changed("market")
}

// LoadStandards reads the IETF documents and meetings and the news of the events.
func (h *Hub) LoadStandards(ctx context.Context, force bool) {
	var (
		docs     []*ietf.Doc
		meetings []*ietf.Meeting
		evnews   = map[string][]*news.Item{}
		mu       sync.Mutex
		wg       sync.WaitGroup
	)
	wg.Add(2)
	go func() { defer wg.Done(); docs, _ = ietf.FetchDocs(ctx, h.Fetcher, force) }()
	go func() { defer wg.Done(); meetings, _ = ietf.FetchMeetings(ctx, h.Fetcher, force) }()
	for _, e := range h.Events {
		wg.Add(1)
		go func() {
			defer wg.Done()
			items, _ := news.Fetch(ctx, h.Fetcher, market.EventFeed(e), force)
			items = news.TitleHas(items, news.FirstWord(e.Name))
			news.Sort(items)
			mu.Lock()
			evnews[e.ID] = items
			mu.Unlock()
		}()
	}
	wg.Wait()
	h.Apply(func() { h.Docs, h.Meetings, h.EventNews, h.StdDone = docs, meetings, evnews, true })
	h.RebuildRadar()
	h.changed("standards")
}

// LoadAdoption reads the downloads and pulls of the packages and keeps a daily snapshot.
func (h *Hub) LoadAdoption(ctx context.Context, force bool) {
	out := map[string][]*adoption.Stat{}
	var mu sync.Mutex
	var wg sync.WaitGroup
	for _, p := range h.Products {
		if len(p.NPM)+len(p.PyPI)+len(p.Docker) == 0 {
			continue
		}
		wg.Add(1)
		go func() {
			defer wg.Done()
			stats := adoption.Product(ctx, h.Fetcher, p, force)
			snap := map[string]float64{}
			for _, s := range stats {
				switch {
				case s.Ecosystem == "docker" && s.Total > 0:
					snap["pulls:"+s.Name] = float64(s.Total)
				case s.Ecosystem != "docker" && s.Month > 0:
					snap["dl30:"+s.Name] = float64(s.Month)
				}
			}
			if len(snap) > 0 {
				_ = h.Store.Snapshot(p.ID, today(), snap)
			}
			mu.Lock()
			out[p.ID] = stats
			mu.Unlock()
		}()
	}
	wg.Wait()
	h.Apply(func() { h.Adoption, h.AdoptionDone = out, true })
	h.changed("adoption")
}

// RebuildRadar recounts the topics over what is loaded, off the UI goroutine.
func (h *Hub) RebuildRadar() {
	var in radar.Input
	h.Apply(func() {
		in.News = append([]*news.Item(nil), h.News...)
		for _, items := range h.VendorNews {
			in.News = append(in.News, items...)
		}
		in.Docs = h.Docs
		in.Names = map[string]string{}
		for _, st := range h.Ordered() {
			in.States = append(in.States, &model.ProductState{Product: st.Product, Releases: st.Releases})
			in.Names[st.Product.ID] = st.Product.Name
		}
	})
	stats := radar.Build(in)
	h.Apply(func() { h.Radar = stats })
}

// ProductNews are the news items mentioning a product.
func (h *Hub) ProductNews(id string) []*news.Item {
	var out []*news.Item
	for _, n := range h.News {
		if n.Has(id) {
			out = append(out, n)
		}
	}
	return out
}

// PullsDelta is the growth of the pulls of a Docker image over about days.
func (h *Hub) PullsDelta(id, image string, days int) (float64, bool) {
	return h.Delta(id, "pulls:"+image, days)
}

// NewsSince counts the news items newer than t.
func (h *Hub) NewsSince(t time.Time) int {
	n := 0
	for _, it := range h.News {
		if it.Date.After(t) {
			n++
		}
	}
	return n
}

// LoadAll loads everything without the interface (the CLI, a schedule).
func (h *Hub) LoadAll(ctx context.Context, force bool) {
	h.LoadReleases(ctx, force)
	var wg sync.WaitGroup
	for _, f := range []func(){
		func() { h.LoadSupportDates(ctx) },
		func() { h.LoadSecurity(ctx, force) },
		func() { h.LoadTableMetrics(ctx, force) },
		func() { h.LoadInfo(ctx, force) },
	} {
		wg.Add(1)
		go func() { defer wg.Done(); f() }()
	}
	wg.Wait()
	h.RebuildRadar()
}
