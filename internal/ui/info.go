package ui

import (
	"fmt"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/napadailo/voip-tui/i18n"
	"github.com/napadailo/voip-tui/tui"
	"github.com/napadailo/voip-tui/tui/theme"

	"github.com/napadailo/voip-info/internal/analytics"
	"github.com/napadailo/voip-info/internal/catalog"
	"github.com/napadailo/voip-info/internal/hub"
	"github.com/napadailo/voip-info/internal/ietf"
	"github.com/napadailo/voip-info/internal/market"
	"github.com/napadailo/voip-info/internal/news"
	"github.com/napadailo/voip-info/internal/radar"
	"github.com/napadailo/voip-info/internal/watch"
)

// newsKinds are the kinds of feeds, translated when shown.
var newsKinds = []string{N("project"), N("industry"), N("market"), N("regional"), N("community"), N("lists")}

func kindTone(kind string) tui.Tone {
	switch kind {
	case "project":
		return tui.ToneAccent
	case "market":
		return tui.ToneOK
	case "regional", "community":
		return tui.ToneWarn
	}
	return tui.ToneDim
}

// pctCell is a change in percent and its tone.
func pctCell(p float64, ok bool) (string, tui.Tone) {
	if !ok {
		return "", tui.ToneDim
	}
	return fmt.Sprintf("%+.0f%%", p), trendTone(p)
}

// priceSpark draws prices from their low to their high.
func priceSpark(closes []float64) string {
	if len(closes) == 0 {
		return ""
	}
	lo, hi := slices.Min(closes), slices.Max(closes)
	v := make([]int, len(closes))
	for i, c := range closes {
		v[i] = 1 + int((c-lo)/max(hi-lo, 1e-9)*100)
	}
	return analytics.Sparkline(v)
}

// every takes each n-th value counting from the last one.
func every(v []float64, n int) []float64 {
	var out []float64
	for i := len(v) - 1; i >= 0; i -= n {
		out = append([]float64{v[i]}, out...)
	}
	return out
}

// productNames joins the names of product ids.
func (a *App) productNames(ids []string) string {
	var names []string
	for _, id := range ids {
		if st := a.hub.States[id]; st != nil {
			names = append(names, st.Product.Name)
		}
	}
	return strings.Join(names, ", ")
}

// newsRows are the table rows of news items.
func (a *App) newsRows(items []*news.Item) []Row {
	rows := make([]Row, 0, len(items))
	for _, n := range items {
		score := ""
		switch {
		case n.Score > 0:
			score = fmt.Sprintf("%d/%d", n.Score, n.Comments)
		case n.Comments > 0:
			score = fmt.Sprintf("+%d", n.Comments)
		}
		rows = append(rows, Row{
			Cells: []string{n.Date.Format("2006-01-02"), age(n.AgeDays()), T(n.Kind), tui.Trunc(n.Source, 28), tui.Trunc(n.Title, 110),
				a.productNames(n.Products), score},
			CellTones: map[int]tui.Tone{1: ageTone(n.AgeDays()), 2: kindTone(n.Kind), 3: tui.ToneDim, 5: tui.ToneAccent},
			Sort:      map[int]float64{1: float64(n.AgeDays()), 6: float64(n.Score*1000 + n.Comments)},
			Data:      n, Key: n.URL,
		})
	}
	return rows
}

// kindHints name the key of every kind of feeds, all first, as the categories elsewhere.
func kindHints() []Hint {
	h := []Hint{{Key: "0", Desc: N("all")}}
	for i, k := range news.Kinds {
		h = append(h, Hint{Key: strconv.Itoa(i + 1), Desc: k})
	}
	return h
}

// newNewsView lists the news; 1-6 pick a kind of feeds, 0 all of them.
// News have no category: W keeps those about my stack.
func newNewsView(a *App, kind string) *listView {
	hints := append(kindHints(), Hint{Key: "enter", Desc: N("details")}, Hint{Key: "b", Desc: N("browser")})
	v := newListView(a, "news", N("News"), []string{N("DATE"), N("AGE"), N("KIND"), N("SOURCE"), N("TITLE"), N("PRODUCTS"), N("SCORE")},
		hints, nil)
	v.info = func() string {
		s := ""
		if kind != "" {
			s = T(kind)
		}
		if !a.hub.NewsLoaded {
			s = strings.TrimSpace(s + " " + T("loading…"))
		}
		return s
	}
	v.fill = func() []Row {
		var watched map[string]hub.Reason
		if a.watchOnly {
			watched = a.hub.WatchedProducts()
		}
		mine := func(id string) bool { _, ok := watched[id]; return ok }
		var items []*news.Item
		for _, n := range a.hub.News {
			if kind != "" && n.Kind != kind {
				continue
			}
			if a.watchOnly && !slices.ContainsFunc(n.Products, mine) && !a.watchedTopic(n.Title) {
				continue
			}
			items = append(items, n)
		}
		return a.newsRows(items)
	}
	setKind := func(k string) {
		kind = k
		v.Rebuild()
		v.SelectIndex(0)
	}
	v.Bind('0', func() { setKind("") })
	for i, k := range news.Kinds {
		v.Bind(rune('1'+i), func() { setKind(k) })
	}
	v.OnEnter(func(r *Row) { a.showNews(r.Data.(*news.Item)) })
	v.Rebuild()
	return v
}

// newsCard is the markup of a news item.
func (a *App) newsCard(n *news.Item) string {
	c := tui.NewCard(n.Title)
	sub := []string{n.Source, T(n.Kind), n.Date.UTC().Format("2006-01-02 15:04") + " UTC"}
	if n.Kind == "community" && n.Score > 0 {
		sub = append(sub, Tf("%d points, %d comments", n.Score, n.Comments))
	} else if n.Comments > 0 {
		sub = append(sub, Tf("replies: %d", n.Comments))
	}
	c.Sub(sub...)
	if p := a.productNames(n.Products); p != "" {
		c.KV(N("products"), tui.Paint(theme.Accent, p))
	}
	c.KV(N("link"), tui.Faint(n.URL))
	if n.Summary == "" {
		c.Line(tui.Faint(T("no description")))
	} else {
		c.Text(n.Summary)
	}
	return c.String()
}

func (a *App) showNews(n *news.Item) { a.showCard(tui.Trunc(n.Title, 40), a.newsCard(n), n.URL) }

func newRadarView(a *App) *listView {
	v := newListView(a, "radar", N("Radar"), []string{N("TOPIC"), "7D", "30D", N("TREND"), N("WEEKLY (12W)"), N("NEWS"),
		N("RELEASES"), "IETF", N("LISTS"), N("TOP PRODUCTS")},
		[]Hint{{Key: "enter", Desc: N("mentions")}}, nil)
	v.fill = func() []Row {
		now := time.Now()
		var stats []*radar.Stat
		for _, s := range a.hub.Radar {
			if !a.watchOnly || a.hub.Watching(watch.Topics, s.Name) {
				stats = append(stats, s)
			}
		}
		sort.SliceStable(stats, func(i, j int) bool { return stats[i].Count(30, now) > stats[j].Count(30, now) })
		rows := make([]Row, 0, len(stats))
		for _, s := range stats {
			src := s.BySource(30, now)
			var top []string
			for _, p := range s.TopProducts(30, 4, now) {
				top = append(top, fmt.Sprintf("%s %d", p.Name, p.Count))
			}
			name := T(s.Name)
			if a.hub.Watching(watch.Topics, s.Name) {
				name = "◉ " + name
			}
			cells := []string{name, fmt.Sprint(s.Count(7, now)), fmt.Sprint(s.Count(30, now)), "-",
				analytics.Sparkline(s.Weekly(12, now)), num0(src[radar.News]), num0(src[radar.Release]), num0(src[radar.IETF]),
				num0(src[radar.Lists]), strings.Join(top, ", ")}
			tones := map[int]tui.Tone{3: tui.ToneDim, 4: tui.ToneAccent, 9: tui.ToneDim}
			if p, ok := s.Trend(now); ok {
				cells[3], tones[3] = pctCell(p, true)
			}
			rows = append(rows, Row{Cells: cells, CellTones: tones, Data: s, Key: s.Name})
		}
		return rows
	}
	v.OnEnter(func(r *Row) { a.showTopic(r.Data.(*radar.Stat)) })
	v.Rebuild()
	return v
}

func num0(n int) string {
	if n == 0 {
		return ""
	}
	return fmt.Sprint(n)
}

var sourceLabels = map[string]string{radar.News: N("news"), radar.Release: N("release"), radar.IETF: "IETF", radar.Lists: N("lists")}

// topicSummary is the head of a topic: counts, trend, sources, products.
func (a *App) topicSummary(s *radar.Stat) string {
	now := time.Now()
	c := tui.NewCard(T(s.Name))
	trend := ""
	if p, ok := s.Trend(now); ok {
		trend = Tf("trend %+.0f%%", p)
	}
	c.Sub(Tf("%d in 7 days, %d in 30", s.Count(7, now), s.Count(30, now)), trend)
	c.KV(N("by week, 12 weeks"), tui.Paint(theme.Accent, analytics.Sparkline(s.Weekly(12, now))))
	var src []string
	for _, k := range []string{radar.News, radar.Release, radar.IETF, radar.Lists} {
		if n := s.BySource(30, now)[k]; n > 0 {
			src = append(src, fmt.Sprintf("%s %d", T(sourceLabels[k]), n))
		}
	}
	c.KV(N("sources, 30 days"), esc(strings.Join(src, ", ")))
	var top []string
	for _, p := range s.TopProducts(30, 8, now) {
		top = append(top, fmt.Sprintf("%s (%d)", p.Name, p.Count))
	}
	if len(top) > 0 {
		c.KV(N("next to products"), esc(strings.Join(top, ", ")))
	}
	if a.hub.Watching(watch.Topics, s.Name) {
		c.KV(N("my stack"), tui.Paint(theme.Accent, "◉ "+T("subscription")))
	}
	return c.String()
}

// showTopic opens a topic: the summary and its mentions, newest first.
func (a *App) showTopic(s *radar.Stat) {
	a.Push(newMentionsView(a, T(s.Name), func() string { return a.topicSummary(s) }, func() []mention {
		out := make([]mention, 0, len(s.Mentions))
		for _, m := range s.Mentions {
			out = append(out, mention{m.Date, T(sourceLabels[m.Source]), m.Title, m.Product, m.URL})
		}
		return out
	}))
}

// Pad pads to a width.
func Pad(s string, w int) string { return tui.Pad(s, w) }

func newIETFView(a *App) *listView {
	v := newListView(a, "ietf", N("IETF"), []string{N("DATE"), N("AGE"), N("TYPE"), "WG", N("DOCUMENT"), N("TITLE"), N("PAGES"), N("EXPIRES")},
		[]Hint{{Key: "enter", Desc: N("details")}, {Key: "b", Desc: N("browser")}}, nil)
	v.fill = func() []Row {
		rows := make([]Row, 0, len(a.hub.Docs))
		for _, d := range a.hub.Docs {
			if a.watchOnly && !a.hub.Watching(watch.WGs, d.WG) {
				continue
			}
			typ, tone := "draft", tui.ToneDim
			if d.Kind == "rfc" {
				typ, tone = "RFC", tui.ToneOK
			}
			pages, exp := "", ""
			if d.Pages > 0 {
				pages = fmt.Sprint(d.Pages)
			}
			if d.Kind == "draft" && !d.Expires.IsZero() {
				exp = d.Expires.Format("2006-01-02")
			}
			wg := d.WG
			if a.hub.Watching(watch.WGs, d.WG) {
				wg = "◉ " + wg
			}
			rows = append(rows, Row{Cells: []string{d.Time.Format("2006-01-02"), age(d.AgeDays()), typ, wg, d.Label(), tui.Trunc(d.Title, 80), pages, exp},
				CellTones: map[int]tui.Tone{1: ageTone(d.AgeDays()), 2: tone, 3: tui.ToneWarn, 4: tui.ToneAccent},
				Sort:      map[int]float64{1: float64(d.AgeDays())}, Data: d, Key: d.Name})
		}
		return rows
	}
	v.info = func() string {
		if !a.hub.StdDone {
			return T("loading…")
		}
		return ""
	}
	v.OnEnter(func(r *Row) { a.showDoc(r.Data.(*ietf.Doc)) })
	v.Rebuild()
	return v
}

func (a *App) showDoc(d *ietf.Doc) {
	c := tui.NewCard(d.Label())
	c.Sub(d.Title)
	if d.Kind == "rfc" {
		c.KV(N("published"), "RFC, "+d.Time.Format("2006-01-02"))
	} else {
		c.KV(N("draft"), Tf("revision %s, updated %s", d.Rev, d.Time.Format("2006-01-02")))
		if !d.Expires.IsZero() {
			c.KV(N("expires"), d.Expires.Format("2006-01-02"))
		}
	}
	c.KV(N("working group"), esc(d.WG+" ("+T(ietf.WGDesc(d.WG))+")"))
	if d.Pages > 0 {
		c.KV(N("pages"), fmt.Sprint(d.Pages))
	}
	c.Line(tui.Faint(d.URL()))
	if d.Abstract == "" {
		c.Line(tui.Faint(T("no abstract")))
	} else {
		c.Text(d.Abstract)
	}
	a.showCard(d.Label(), c.String(), d.URL())
}

// eventRef is a row of the events view: an IETF meeting or a catalog event.
type eventRef struct {
	meeting *ietf.Meeting
	event   *catalog.Event
	when    time.Time
}

func newEventsView(a *App) *listView {
	v := newListView(a, "events", N("Events"), []string{N("WHEN"), N("IN DAYS"), N("EVENT"), N("WHERE"), N("SOURCE"), N("LATEST MENTION")},
		[]Hint{{Key: "enter", Desc: N("details")}, {Key: "b", Desc: N("browser")}}, nil)
	v.fill = func() []Row {
		now := time.Now()
		var refs []eventRef
		for _, m := range a.hub.Meetings {
			refs = append(refs, eventRef{meeting: m, when: m.Date})
		}
		for i := range a.hub.Events {
			e := &a.hub.Events[i]
			refs = append(refs, eventRef{event: e, when: market.NextMonth(*e, now)})
		}
		sort.SliceStable(refs, func(i, j int) bool {
			if refs[i].when.IsZero() != refs[j].when.IsZero() {
				return !refs[i].when.IsZero()
			}
			return refs[i].when.Before(refs[j].when)
		})
		var rows []Row
		for _, r := range refs {
			cells := make([]string, 6)
			tones := map[int]tui.Tone{3: tui.ToneDim, 4: tui.ToneDim}
			key := ""
			if m := r.meeting; m != nil {
				cells[0], cells[2], cells[3], cells[4] = m.Date.Format("2006-01-02"), "IETF "+m.Number, m.City+", "+m.Country, "datatracker"
				tones[0], key = tui.ToneAccent, "ietf\t"+m.Number
			} else {
				e := r.event
				cells[0], cells[2], cells[3], cells[4] = "?", e.Name, e.Where.In(i18n.Lang()), T("varies")
				if !r.when.IsZero() {
					cells[0], cells[4] = r.when.Format("2006-01"), T("usually")
				}
				if ns := a.hub.EventNews[e.ID]; len(ns) > 0 {
					cells[5] = tui.Trunc(ns[0].Title, 80)
				}
				key = "event\t" + e.ID
			}
			if !r.when.IsZero() {
				d := int(r.when.Sub(now).Hours() / 24)
				cells[1] = fmt.Sprint(d)
				if d <= 45 {
					tones[1] = tui.ToneWarn
				}
			}
			rows = append(rows, Row{Cells: cells, CellTones: tones, Data: r, Key: key})
		}
		return rows
	}
	url := func(r eventRef) string {
		if r.meeting != nil {
			return r.meeting.URL()
		}
		return r.event.URL
	}
	v.OnEnter(func(r *Row) { a.showEvent(r.Data.(eventRef), url(r.Data.(eventRef))) })
	v.Rebuild()
	return v
}

func (a *App) showEvent(r eventRef, url string) {
	if m := r.meeting; m != nil {
		c := tui.NewCard("IETF " + m.Number)
		c.Sub(m.City+", "+m.Country, m.Date.Format("2006-01-02")+" - "+m.Date.AddDate(0, 0, m.Days-1).Format("2006-01-02"))
		c.Section(N("Working groups on the topic"))
		for _, w := range ietf.WGs {
			c.KVRaw(w.Name, esc(T(w.Desc)))
		}
		c.Line(tui.Faint(url))
		a.showCard("IETF "+m.Number, c.String(), url)
		return
	}
	e := r.event
	summary := func() string {
		c := tui.NewCard(e.Name)
		c.Sub(e.Where.In(i18n.Lang()), url)
		if e.Month > 0 {
			c.KV(N("usually"), Tf("month %d of the year (a guide)", e.Month))
		} else {
			c.KV(N("usually"), T("the month varies: see the latest news"))
		}
		return c.String()
	}
	a.Push(newMentionsView(a, e.Name, summary, func() []mention { return newsMentions(news.Dedupe(a.hub.EventNews[e.ID])) }))
}

// newsMentions are folded headlines as mentions; the similar ones are counted in the title.
func newsMentions(groups []news.Group) []mention {
	out := make([]mention, 0, len(groups))
	for _, g := range groups {
		src := g.Item.Source
		if i := strings.LastIndex(src, " · "); i >= 0 {
			src = src[i+len(" · "):]
		}
		title := g.Item.Title
		if g.Similar > 0 {
			title += "  " + Tf("+%d similar", g.Similar)
		}
		out = append(out, mention{g.Item.Date, src, title, "", g.Item.URL})
	}
	return out
}

func newMarketView(a *App) *listView {
	v := newListView(a, "market", N("Market"), []string{N("VENDOR"), N("SEGMENT"), N("TICKER"), N("PRICE"), "1M", "3M", "90D",
		N("NEWS 30D"), N("LATEST HEADLINE")},
		[]Hint{{Key: "enter", Desc: N("details")}, {Key: "b", Desc: N("browser")}}, nil)
	v.fill = func() []Row {
		var vendors []*catalog.Vendor
		for i := range a.hub.Vendors {
			if !a.watchOnly || a.hub.Watching(watch.Vendors, a.hub.Vendors[i].ID) {
				vendors = append(vendors, &a.hub.Vendors[i])
			}
		}
		move := func(v *catalog.Vendor) float64 {
			p, _ := a.hub.Quotes[v.ID].Change(21)
			return max(p, -p)
		}
		sort.SliceStable(vendors, func(i, j int) bool {
			if (vendors[i].Ticker == "") != (vendors[j].Ticker == "") {
				return vendors[i].Ticker != ""
			}
			return move(vendors[i]) > move(vendors[j])
		})
		var rows []Row
		for _, vd := range vendors {
			q := a.hub.Quotes[vd.ID]
			groups := news.Dedupe(a.hub.VendorNews[vd.ID])
			name := vd.Name
			if a.hub.Watching(watch.Vendors, vd.ID) {
				name = "◉ " + name
			}
			cells := []string{name, vd.Segment, vd.Ticker, "-", "", "", "", fmt.Sprint(len(groups)), ""}
			tones := map[int]tui.Tone{1: tui.ToneDim, 2: tui.ToneAccent, 6: tui.ToneAccent}
			if vd.Ticker == "" {
				cells[2], tones[2] = T("private"), tui.ToneDim
			}
			srt := map[int]float64{}
			if q != nil && q.Price > 0 {
				cells[3], srt[3] = fmt.Sprintf("$%.2f", q.Price), q.Price
				cells[4], tones[4] = pctCell(q.Change(21))
				cells[5], tones[5] = pctCell(q.Change(63))
				cells[6] = priceSpark(every(q.Closes[max(0, len(q.Closes)-63):], 3))
			} else if q != nil && q.Err != nil {
				cells[6], tones[6] = tui.Trunc(q.Err.Error(), 30), tui.ToneDim
			}
			if len(groups) > 0 {
				cells[8] = tui.Trunc(groups[0].Item.Title, 70)
			}
			rows = append(rows, Row{Cells: cells, CellTones: tones, Sort: srt, Data: vd, Key: vd.ID})
		}
		return rows
	}
	v.info = func() string {
		if !a.hub.MarketDone {
			return T("loading…")
		}
		return ""
	}
	v.OnEnter(func(r *Row) { a.showVendor(r.Data.(*catalog.Vendor)) })
	v.Rebuild()
	return v
}

func vendorURL(v *catalog.Vendor) string {
	if v.Ticker == "" {
		return ""
	}
	return "https://www.nasdaq.com/market-activity/stocks/" + strings.ToLower(v.Ticker)
}

// vendorSummary is the head of a vendor: what it does and its quotes.
func (a *App) vendorSummary(v *catalog.Vendor) string {
	c := tui.NewCard(v.Name + " · " + v.Segment)
	c.Text(v.Desc.In(i18n.Lang()))
	q := a.hub.Quotes[v.ID]
	switch {
	case q != nil && q.Price > 0:
		ch := func(d int) string {
			if p, ok := q.Change(d); ok {
				return fmt.Sprintf("%+.1f%%", p)
			}
			return "-"
		}
		c.KV(v.Ticker, fmt.Sprintf("$%.2f", q.Price)+"  "+tui.Faint(q.AsOf))
		c.KV(N("week / month / 3 months"), ch(5)+" / "+ch(21)+" / "+ch(63))
		c.KV("90D", tui.Paint(theme.Accent, priceSpark(q.Closes[max(0, len(q.Closes)-90):])))
	case v.Ticker != "":
		reason := T("no data")
		if q != nil && q.Err != nil {
			reason = q.Err.Error()
		}
		c.KV(v.Ticker, tui.Faint(Tf("quotes unavailable: %s", reason)))
	default:
		c.Line(tui.Faint(T("a private company: no quotes")))
	}
	if a.hub.Watching(watch.Vendors, v.ID) {
		c.KV(N("my stack"), tui.Paint(theme.Accent, "◉ "+T("subscription")))
	}
	return c.String()
}

// showVendor opens a vendor: the quotes and its news of 30 days.
func (a *App) showVendor(v *catalog.Vendor) {
	a.Push(newMentionsView(a, v.Name, func() string { return a.vendorSummary(v) },
		func() []mention { return newsMentions(news.Dedupe(a.hub.VendorNews[v.ID])) }))
}

// usageLine sums up the packages of a product on one line: npm and PyPI
// downloads of a month and their weekly trend, Docker Hub pulls and their
// growth over a week; "" when the product has none.
func (a *App) usageLine(id string) string {
	var parts []string
	for _, s := range a.hub.Adoption[id] {
		label := s.Ecosystem + " " + s.Name
		switch {
		case s.Ecosystem == "docker" && s.Total > 0:
			p := Tf("%s: %s pulls", label, num(s.Total))
			if d, ok := a.hub.PullsDelta(id, s.Name, 7); ok && d > 0 {
				p += ", " + Tf("+%s in 7 days", num(int(d)))
			}
			parts = append(parts, p)
		case s.Month > 0:
			p := Tf("%s: %s/month", label, num(s.Month))
			if s.HasTrend {
				p += ", " + Tf("week %+.0f%%", s.Trend)
			}
			parts = append(parts, p)
		}
	}
	return strings.Join(parts, " · ")
}
