// Package digest writes what changed over a period as one Markdown page:
// releases, vulnerabilities, my versions, branch support, project signals,
// news, the market, standards and topics. "My stack" collects everything
// about the subscription. The text follows the interface language.
package digest

import (
	"fmt"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/napadailo/voip-tui/i18n"

	"github.com/napadailo/voip-info/internal/analytics"
	"github.com/napadailo/voip-info/internal/deps"
	"github.com/napadailo/voip-info/internal/hub"
	"github.com/napadailo/voip-info/internal/inventory"
	"github.com/napadailo/voip-info/internal/model"
	"github.com/napadailo/voip-info/internal/news"
	"github.com/napadailo/voip-info/internal/relnotes"
)

// T translates; Tf translates a format and fills it in.
func T(s string) string               { return i18n.T(s) }
func Tf(f string, args ...any) string { return i18n.Tf(f, args...) }

var sevIcon = map[string]string{"critical": "🟥", "high": "🟧", "medium": "🟨", "moderate": "🟨", "low": "🟩"}

func icon(sev string) string {
	if s, ok := sevIcon[sev]; ok {
		return s
	}
	return "⬜"
}

var mdEsc = strings.NewReplacer("|", `\|`, "[", `\[`, "]", `\]`)

func esc(s string) string { return mdEsc.Replace(s) }

func trunc(s string, n int) string {
	if r := []rune(s); len(r) > n {
		return string(r[:n-1]) + "…"
	}
	return s
}

// Kind of a release against the older ones of the feed: major, minor (a new branch), patch or pre.
func Kind(r *model.Release, st *model.ProductState) string {
	if r.Prerelease() {
		return "pre"
	}
	v := r.Version()
	var older [][]int
	for _, x := range st.Releases {
		if x.Date.Before(r.Date) && !x.Prerelease() && x.Version() != nil {
			older = append(older, x.Version())
		}
	}
	if v == nil || len(older) == 0 || len(v) > 2 && v[2] != 0 {
		return "patch" // x.y.N with N > 0 patches a branch even when x.y.0 fell out of the feed
	}
	topMajor := 0
	var topMinor []int
	for _, o := range older {
		topMajor = max(topMajor, o[0])
		if len(o) > 1 && (topMinor == nil || model.CompareVersions(o[:2], topMinor) > 0) {
			topMinor = o[:2]
		}
	}
	switch {
	case len(v) > 1 && v[0] > topMajor:
		return "major"
	case len(v) > 1 && topMinor != nil && model.CompareVersions(v[:2], topMinor) > 0:
		return "minor"
	}
	return "patch"
}

// Label is a period of days as the title shows it.
func Label(days int) string {
	if days == 1 {
		return T("24 hours")
	}
	return Tf("%d days", days)
}

type rel struct {
	st *model.ProductState
	r  *model.Release
}

// Options of a digest.
type Options struct {
	Since time.Time
	Label string // the period, as shown: "7 days"
	Mine  bool   // only "my stack"
	Now   time.Time
}

type writer struct {
	h   *hub.Hub
	o   Options
	b   []string
	now time.Time
}

func (w *writer) add(lines ...string) { w.b = append(w.b, lines...) }

func (w *writer) name(id string) string {
	if st := w.h.States[id]; st != nil {
		return st.Product.Name
	}
	return id
}

// Build writes the digest; call it where the hub data may be read (the UI goroutine or a headless load).
func Build(h *hub.Hub, o Options) string {
	if o.Now.IsZero() {
		o.Now = time.Now()
	}
	w := &writer{h: h, o: o, now: o.Now}
	period := Tf("_Period: %s - %s UTC_", o.Since.UTC().Format("2006-01-02 15:04"), o.Now.UTC().Format("2006-01-02 15:04"))
	if o.Mine {
		w.add("# "+Tf("My stack · %s", o.Label), "", period, "")
		if !h.HasWatch() {
			w.add(T("_The subscription is empty: w on a row of any view or :watch <id|topic|vendor|wg>._"))
		} else {
			w.stack()
		}
		return strings.Join(w.b, "\n")
	}
	w.add("# "+Tf("VoIP digest · %s", o.Label), "", period, "")
	w.full()
	return strings.Join(w.b, "\n")
}

func (w *writer) releases(only map[string]hub.Reason) []rel {
	var out []rel
	for _, st := range w.h.Ordered() {
		if only != nil {
			if _, ok := only[st.Product.ID]; !ok {
				continue
			}
		}
		for _, r := range st.Releases {
			if !r.Commit && !r.Date.Before(w.o.Since) {
				out = append(out, rel{st, r})
			}
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].r.Date.After(out[j].r.Date) })
	return out
}

func (w *writer) newVulns() []*model.Vuln {
	var out []*model.Vuln
	for _, v := range w.h.AllVulns() {
		if !v.Published.Before(w.o.Since) {
			out = append(out, v)
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return w.h.RiskLess(out[i], out[j]) })
	return out
}

func (w *writer) epss(v *model.Vuln) string {
	if e, ok := w.h.EPSSOf(v); ok {
		return fmt.Sprintf("%.1f%%", e.Score*100)
	}
	return "-"
}

func (w *writer) newsLine(n *news.Item, similar int) string {
	src := n.Source
	if i := strings.LastIndex(src, " · "); i >= 0 {
		src = src[i+len(" · "):]
	}
	s := fmt.Sprintf("- [%s](%s) · _%s_ · %s", esc(n.Title), n.URL, src, n.Date.Format("01-02"))
	if n.Score > 0 {
		s += fmt.Sprintf(" · ▲%d", n.Score)
	}
	if similar > 0 {
		s += " · _" + Tf("+%d similar", similar) + "_"
	}
	return s
}

// stack is the section "My stack": everything about the subscription.
func (w *writer) stack() {
	h := w.h
	watched := h.WatchedProducts()
	w.add("## ⭐ "+Tf("My stack (%d products with their dependencies)", len(watched)), "")
	empty := true

	if rels := w.releases(watched); len(rels) > 0 {
		empty = false
		w.add("**"+T("Releases:")+"**", "")
		var order []string
		by := map[string][]string{}
		for _, x := range rels {
			name := x.st.Product.Name
			if by[name] == nil {
				order = append(order, name)
			}
			info := relnotes.Classify(x.r)
			s := "`" + x.r.Tag + "`"
			switch Kind(x.r, x.st) {
			case "major", "minor":
				s += " 🆕"
			case "pre":
				s += " ⏳"
			}
			if len(info.Tags) > 0 {
				s += " `" + info.Badge() + "`"
			}
			by[name] = append(by[name], s)
		}
		for _, n := range order {
			w.add("- **" + n + "**: " + strings.Join(first(by[n], 5), ", "))
		}
		w.add("")
	}

	var vulns []*model.Vuln
	for _, v := range w.newVulns() {
		if _, ok := watched[v.Product]; ok {
			vulns = append(vulns, v)
		}
	}
	if len(vulns) > 0 {
		empty = false
		w.add("**"+Tf("Vulnerabilities (%d):", len(vulns))+"**", "")
		for _, v := range first(vulns, 12) {
			fire := ""
			if h.KEVOf(v) != nil {
				fire = "🔥"
			}
			line := fmt.Sprintf("- %s%s **%s** [%s](%s) %s", icon(v.Severity), fire, w.name(v.Product), v.ID, v.URL, esc(trunc(v.Summary, 90)))
			if e := w.epss(v); e != "-" {
				line += " · EPSS " + e
			}
			w.add(line)
		}
		w.add("")
	}

	var soon []string
	for _, st := range h.Ordered() {
		if _, ok := watched[st.Product.ID]; !ok {
			continue
		}
		for _, b := range h.Support(st) {
			if what, when, ok := b.Next(w.now); ok && b.Source == "policy" && when.Sub(w.now) <= 180*24*time.Hour {
				soon = append(soon, "- "+Tf("**%s %s**: %s %s (in %d days)", st.Product.Name, b.Branch, T(what), when.Format("2006-01-02"),
					int(when.Sub(w.now).Hours()/24)))
			}
		}
	}
	if len(soon) > 0 {
		empty = false
		w.add("**"+T("Branch support:")+"**", "")
		w.add(soon...)
		w.add("")
	}

	var items []*news.Item
	for _, n := range h.News {
		if n.Date.Before(w.o.Since) {
			continue
		}
		if slices.ContainsFunc(n.Products, func(id string) bool { _, ok := watched[id]; return ok }) {
			items = append(items, n)
		}
	}
	if len(items) > 0 {
		empty = false
		sort.SliceStable(items, func(i, j int) bool {
			if items[i].Comments != items[j].Comments {
				return items[i].Comments > items[j].Comments
			}
			return items[i].Date.After(items[j].Date)
		})
		w.add("**"+Tf("News and discussions (%d):", len(items))+"**", "")
		for _, g := range first(news.Dedupe(items), 10) {
			w.add(w.newsLine(g.Item, g.Similar))
		}
		w.add("")
	}

	if h.Watch != nil && len(h.Watch.Topics) > 0 {
		var lines []string
		for _, s := range h.Radar {
			if !slices.Contains(h.Watch.Topics, s.Name) {
				continue
			}
			line := "- **" + T(s.Name) + "**: " + Tf("%d in 7 days, %d in 30", s.Count(7, w.now), s.Count(30, w.now))
			if p, ok := s.Trend(w.now); ok {
				line += fmt.Sprintf(" (%+.0f%%)", p)
			}
			lines = append(lines, line)
			n := 0
			for _, m := range s.Mentions {
				if !m.Date.Before(w.o.Since) && n < 3 {
					lines = append(lines, fmt.Sprintf("  - [%s](%s)", esc(trunc(m.Title, 100)), m.URL))
					n++
				}
			}
		}
		if len(lines) > 0 {
			empty = false
			w.add("**"+T("Topics:")+"**", "")
			w.add(lines...)
			w.add("")
		}
	}

	if h.Watch != nil {
		var lines []string
		for _, v := range h.Vendors {
			if !slices.Contains(h.Watch.Vendors, v.ID) {
				continue
			}
			var recent []*news.Item
			for _, n := range h.VendorNews[v.ID] {
				if !n.Date.Before(w.o.Since) {
					recent = append(recent, n)
				}
			}
			ch, chOK := h.Quotes[v.ID].Change(21)
			if len(recent) == 0 && !chOK {
				continue
			}
			line := "- **" + v.Name + "**"
			if chOK {
				line += " (" + Tf("%s %+.0f%% in a month", v.Ticker, ch) + ")"
			}
			if len(recent) > 0 {
				g := news.Dedupe(recent)[0]
				line += fmt.Sprintf(": [%s](%s)", esc(g.Item.Title), g.Item.URL)
				if g.Similar > 0 {
					line += " · _" + Tf("+%d similar", g.Similar) + "_"
				}
			}
			lines = append(lines, line)
		}
		if len(lines) > 0 {
			empty = false
			w.add("**"+T("Vendors:")+"**", "")
			w.add(lines...)
			w.add("")
		}
		var docs []string
		for _, d := range h.Docs {
			if slices.Contains(h.Watch.WGs, d.WG) && !d.Time.Before(w.o.Since) && len(docs) < 10 {
				docs = append(docs, fmt.Sprintf("- [%s](%s) (%s): %s", d.Label(), d.URL(), d.WG, esc(d.Title)))
			}
		}
		if len(docs) > 0 {
			empty = false
			w.add("**IETF:**", "")
			w.add(docs...)
			w.add("")
		}
	}
	if empty {
		w.add(T("_Nothing new about your stack over the period._"), "")
	}
}

func first[T any](s []T, n int) []T {
	if len(s) > n {
		return s[:n]
	}
	return s
}

func (w *writer) full() {
	h := w.h
	rels := w.releases(nil)
	kinds := map[string][]rel{}
	products := map[string]bool{}
	for _, x := range rels {
		k := Kind(x.r, x.st)
		kinds[k] = append(kinds[k], x)
		products[x.st.Product.ID] = true
	}
	vulns := w.newVulns()
	serious := 0
	for _, v := range vulns {
		if v.Serious() {
			serious++
		}
	}
	var win []*news.Item
	for _, n := range h.News {
		if !n.Date.Before(w.o.Since) {
			win = append(win, n)
		}
	}
	byKind := func(kind string) []*news.Item {
		var out []*news.Item
		for _, n := range win {
			if n.Kind == kind {
				out = append(out, n)
			}
		}
		return out
	}

	// TL;DR
	var kindCounts []string
	for _, k := range news.Kinds {
		kindCounts = append(kindCounts, fmt.Sprintf("%s %d", T(k), len(byKind(k))))
	}
	w.add("## TL;DR", "",
		"- "+Tf("**%d** releases in %d products, significant (major or a new branch): **%d**, pre-releases: **%d**",
			len(rels), len(products), len(kinds["major"])+len(kinds["minor"]), len(kinds["pre"])),
		"- "+Tf("**%d** new vulnerabilities, high/critical: **%d**", len(vulns), serious),
		"- "+Tf("**%d** news items: %s", len(win), strings.Join(kindCounts, ", ")), "")
	if h.HasWatch() {
		w.stack()
	}
	w.mine()
	w.kev()
	w.security(vulns, win)
	w.inherited(vulns, rels)
	w.releaseSections(kinds)
	w.support()
	w.signals()
	w.market()
	w.standards()
	section := func(title string, items []*news.Item, limit int) {
		if len(items) == 0 {
			return
		}
		sort.SliceStable(items, func(i, j int) bool {
			if items[i].Score != items[j].Score {
				return items[i].Score > items[j].Score
			}
			return items[i].Date.After(items[j].Date)
		})
		w.add("## "+title, "")
		for _, g := range first(news.Dedupe(items), limit) {
			line := w.newsLine(g.Item, g.Similar)
			if len(g.Item.Products) > 0 {
				var names []string
				for _, id := range g.Item.Products {
					names = append(names, w.name(id))
				}
				line += " · `" + strings.Join(names, ", ") + "`"
			}
			w.add(line)
		}
		w.add("")
	}
	var market []*news.Item
	for _, n := range byKind("market") {
		if !strings.Contains(strings.ToLower(n.Source), "security") {
			market = append(market, n)
		}
	}
	section("📰 "+T("Project blogs"), byKind("project"), 12)
	section("🌐 "+T("Market"), market, 15)
	section("🧭 "+T("Industry press"), byKind("industry"), 10)
	section("💬 Hacker News", byKind("community"), 10)
	section("🗺 "+T("Region: Kazakhstan and CIS"), byKind("regional"), 12)
	if lists := byKind("lists"); len(lists) > 0 {
		sort.SliceStable(lists, func(i, j int) bool { return lists[i].Comments > lists[j].Comments })
		var srcs []string
		count := map[string]int{}
		for _, n := range lists {
			if count[n.Source] == 0 {
				srcs = append(srcs, n.Source)
			}
			count[n.Source]++
		}
		var parts []string
		for _, s := range srcs {
			parts = append(parts, fmt.Sprintf("%s %d", s, count[s]))
		}
		w.add("## 🗨 "+T("Lists and forums"), "", Tf("Threads over the period: %s", strings.Join(parts, ", ")), "")
		for _, n := range first(lists, 10) {
			w.add(fmt.Sprintf("- [%s](%s) · _%s_ · 💬%d · %s", esc(n.Title), n.URL, n.Source, n.Comments, n.Date.Format("01-02")))
		}
		w.add("")
	}
}

func (w *writer) mine() {
	h := w.h
	if len(h.Inventory) == 0 {
		return
	}
	all := h.Assessments()
	w.add("## 🧰 "+T("My versions"), "")
	n := 0
	for _, a := range all {
		if a.Urgency < inventory.Update {
			continue
		}
		mark := "🟡"
		if a.Urgency == inventory.Urgent {
			mark = "🔴"
		}
		action := Tf(a.Action, a.Args...)
		if len(a.Args) == 0 {
			action = T(a.Action)
		}
		if a.Urgency == inventory.Urgent && a.Target() != "" {
			action += " → " + a.Target()
		}
		if m := a.ModuleKEVNote(); m != "" {
			action += " · " + Tf("KEV in the module %s", m)
		}
		w.add(fmt.Sprintf("- %s **%s %s**: %s", mark, w.name(a.ID), a.Version, action))
		n++
	}
	if n == 0 {
		w.add(Tf("_All %d products are up to date._", len(all)))
	}
	w.add("")
}

func (w *writer) kev() {
	h := w.h
	if len(h.KEV) == 0 {
		return
	}
	w.add("## 🔥 "+T("Exploited in attacks (CISA KEV)"), "")
	var fresh []string
	total := 0
	seen := map[string]bool{}
	var added []*struct {
		line string
		at   time.Time
	}
	for _, k := range h.KEV {
		if !k.VoIP() {
			continue
		}
		total++
		if !k.Added.Before(w.o.Since) {
			seen[k.CVE] = true
			line := fmt.Sprintf("- **%s %s** [%s](https://nvd.nist.gov/vuln/detail/%s) - %s · %s", k.Vendor, k.Product, k.CVE, k.CVE,
				esc(k.Name), Tf("added %s", k.Added.Format("01-02")))
			if k.Ransomware {
				line += " · ransomware"
			}
			added = append(added, &struct {
				line string
				at   time.Time
			}{line, k.Added})
		}
	}
	sort.Slice(added, func(i, j int) bool { return added[i].at.After(added[j].at) })
	for _, a := range first(added, 10) {
		fresh = append(fresh, a.line)
	}
	for _, v := range h.AllVulns() {
		if k := h.KEVOf(v); k != nil && !k.Added.Before(w.o.Since) && !seen[k.CVE] {
			seen[k.CVE] = true
			fresh = append(fresh, fmt.Sprintf("- **%s** [%s](%s) (%s)", w.name(v.Product), v.ID, v.URL, T("a tracked product")))
		}
	}
	if len(fresh) == 0 {
		w.add(T("_No new VoIP entries in KEV over the period._"))
	} else {
		w.add(fresh...)
	}
	w.add("", Tf("_VoIP and UC vulnerabilities in KEV in all: %d, see :kev_", total), "")
}

func (w *writer) security(vulns []*model.Vuln, win []*news.Item) {
	h := w.h
	w.add("## 🔴 "+T("Security"), "")
	if len(vulns) == 0 {
		w.add(T("_No new CVEs or advisories of the tracked products._"), "")
	} else {
		w.add("| | "+T("Product")+" | ID | CVSS | EPSS | "+T("Summary")+" |", "|---|---|---|---|---|---|")
		for _, v := range first(vulns, 25) {
			fire := ""
			if h.KEVOf(v) != nil {
				fire = "🔥"
			}
			score := "-"
			if v.Score > 0 {
				score = fmt.Sprintf("%.1f", v.Score)
			}
			w.add(fmt.Sprintf("| %s%s | %s | [%s](%s) | %s | %s | %s |", icon(v.Severity), fire, w.name(v.Product), v.ID, v.URL, score,
				w.epss(v), esc(trunc(v.Summary, 100))))
		}
		if len(vulns) > 25 {
			w.add("", Tf("_…and %d more in :security_", len(vulns)-25))
		}
		count := map[string]int{}
		var ids []string
		for _, v := range vulns {
			if count[v.Product] == 0 {
				ids = append(ids, v.Product)
			}
			count[v.Product]++
		}
		sort.SliceStable(ids, func(i, j int) bool { return count[ids[i]] > count[ids[j]] })
		var top []string
		for _, id := range first(ids, 5) {
			top = append(top, fmt.Sprintf("**%s** (%d)", w.name(id), count[id]))
		}
		w.add("", T("The most:")+" "+strings.Join(top, ", "), "")
	}
	var press []*news.Item
	for _, n := range win {
		if n.Kind == "market" && strings.Contains(strings.ToLower(n.Source), "security") {
			press = append(press, n)
		}
	}
	if len(press) > 0 {
		w.add("**"+T("In the press:")+"**", "")
		for _, g := range first(news.Dedupe(press), 8) {
			w.add(w.newsLine(g.Item, g.Similar))
		}
		w.add("")
	}
}

var statusMark = map[string]string{deps.Pending: "⏳", deps.Released: "❔", deps.Fixed: "✅", deps.Separate: "🔧"}

func (w *writer) inherited(vulns []*model.Vuln, rels []rel) {
	h := w.h
	var rows []string
	for _, v := range vulns {
		if !v.Serious() {
			continue
		}
		imp := h.Graph.Downstream(v.Product, true)
		if len(imp) == 0 {
			continue
		}
		var parts []string
		for _, hop := range imp {
			mark := ""
			for _, i := range h.Inherited(hop.Edge.Src, 3650, 1) {
				if i.Vuln.ID == v.ID {
					mark = " " + statusMark[i.Status]
					break
				}
			}
			parts = append(parts, w.name(hop.Edge.Src)+mark)
		}
		rows = append(rows, fmt.Sprintf("- %s **%s** [%s](%s) → %s", icon(v.Severity), w.name(v.Product), v.ID, v.URL, strings.Join(parts, ", ")))
	}
	w.add("## 🔗 "+T("Through dependencies"), "")
	if len(rows) == 0 {
		w.add(T("_No new high/critical vulnerabilities flowing through dependencies._"))
	} else {
		w.add(T("Vulnerabilities of libraries and platforms other products inherit (⏳ waiting for a release · ❔ released, not mentioned · ✅ probably fixed · 🔧 update separately):"), "")
		w.add(first(rows, 15)...)
	}
	var branches []string
	for _, x := range rels {
		k := Kind(x.r, x.st)
		down := h.Graph.Downstream(x.st.Product.ID, false)
		if x.r.Prerelease() || len(down) == 0 || k != "major" && k != "minor" {
			continue
		}
		var users []string
		for _, hop := range down {
			users = append(users, w.name(hop.Edge.Src))
		}
		branches = append(branches, fmt.Sprintf("- **%s** `%s` → %s", x.st.Product.Name, x.r.Tag, strings.Join(users, ", ")))
	}
	if len(branches) > 0 {
		w.add("", "**"+T("New branches of dependencies")+"** "+T("(check the compatibility of their users):"), "")
		w.add(branches...)
	}
	w.add("")
}

func (w *writer) releaseSections(kinds map[string][]rel) {
	w.add("## 🚀 "+T("Significant releases"), "")
	sig := append(append([]rel(nil), kinds["major"]...), kinds["minor"]...)
	sort.SliceStable(sig, func(i, j int) bool { return sig[i].r.Date.After(sig[j].r.Date) })
	if len(sig) == 0 {
		w.add(T("_No major releases or new branches._"))
	}
	for _, x := range sig {
		kind := T("new branch")
		if Kind(x.r, x.st) == "major" {
			kind = "**MAJOR**"
		}
		info := relnotes.Classify(x.r)
		line := fmt.Sprintf("- **%s** `%s` - %s, %s", x.st.Product.Name, x.r.Tag, kind, x.r.Date.Format("2006-01-02"))
		if len(info.Tags) > 0 {
			line += " · `" + info.Badge() + "`"
		}
		if info.Risk() == "high" {
			line += " · " + T("upgrade risk **high**")
		}
		w.add(line + fmt.Sprintf(" · [notes](%s)", x.r.URL))
	}
	w.add("")
	if pre := kinds["pre"]; len(pre) > 0 {
		w.add("### ⏳ "+T("Coming soon (pre-releases)"), "")
		seen := map[string]bool{}
		for _, x := range pre {
			key := x.st.Product.ID + "\t" + x.r.Branch()
			if !seen[key] {
				seen[key] = true
				w.add(fmt.Sprintf("- **%s** `%s` (%s)", x.st.Product.Name, x.r.Tag, x.r.Date.Format("2006-01-02")))
			}
		}
		w.add("")
	}
	if patch := kinds["patch"]; len(patch) > 0 {
		w.add("### 🔧 "+T("Patch releases"), "")
		var names []string
		by := map[string][]string{}
		for _, x := range patch {
			n := x.st.Product.Name
			if by[n] == nil {
				names = append(names, n)
			}
			info := relnotes.Classify(x.r)
			s := "`" + x.r.Tag + "`"
			if info.Has("SEC") {
				s += " 🛡"
			}
			if info.Has("BRK") {
				s += " ⚠"
			}
			by[n] = append(by[n], s)
		}
		sort.Strings(names)
		for _, n := range names {
			more := ""
			if len(by[n]) > 6 {
				more = " …"
			}
			w.add("- **" + n + "**: " + strings.Join(first(by[n], 6), ", ") + more)
		}
		w.add("", T("_🛡 security fixes in the notes, ⚠ breaking changes or upgrade notes._"), "")
	}
}

func (w *writer) support() {
	type item struct {
		at         time.Time
		name, line string
	}
	var items []item
	for _, st := range w.h.Ordered() {
		for _, b := range w.h.Support(st) {
			if b.Source != "policy" {
				continue
			}
			if what, when, ok := b.Next(w.now); ok && when.Sub(w.now) <= 180*24*time.Hour {
				items = append(items, item{when, st.Product.Name, Tf("**%s %s** (%s): %s %s (in %d days)", st.Product.Name, b.Branch, b.Kind,
					T(what), when.Format("2006-01-02"), int(when.Sub(w.now).Hours()/24))})
			}
			if !b.EOL.IsZero() && !b.EOL.Before(w.o.Since) && !b.EOL.After(w.now) {
				items = append(items, item{b.EOL, st.Product.Name, Tf("**%s %s** (%s): EOL came %s (%d days ago)", st.Product.Name, b.Branch, b.Kind,
					b.EOL.Format("2006-01-02"), int(w.now.Sub(b.EOL).Hours()/24))})
			}
		}
	}
	if len(items) == 0 {
		return
	}
	sort.SliceStable(items, func(i, j int) bool { return items[i].at.Before(items[j].at) })
	w.add("## 📅 "+T("Branch support"), "")
	for _, it := range items {
		w.add("- " + it.line)
	}
	w.add("")
}

func (w *writer) signals() {
	h := w.h
	w.add("## 📈 "+T("Project signals"), "")
	type trend struct {
		pct  float64
		name string
	}
	var trends []trend
	for _, st := range h.Ordered() {
		if _, _, pct, ok := analytics.CommitTrend(st.GH.Weekly); ok {
			trends = append(trends, trend{pct, st.Product.Name})
		}
	}
	if len(trends) == 0 {
		w.add(T("_Commit trends come with the GitHub metrics (a token is needed)._"))
	} else {
		sort.Slice(trends, func(i, j int) bool { return trends[i].pct > trends[j].pct })
		var up, down []string
		for _, t := range trends {
			if t.pct >= 40 && len(up) < 5 {
				up = append(up, fmt.Sprintf("%s %+.0f%%", t.name, t.pct))
			}
		}
		for i := len(trends) - 1; i >= 0; i-- {
			if t := trends[i]; t.pct <= -40 && len(down) < 5 {
				down = append(down, fmt.Sprintf("%s %+.0f%%", t.name, t.pct))
			}
		}
		if len(up) > 0 {
			w.add("**"+T("Sped up")+"** "+T("(commits of 12 weeks against the 12 before):")+" "+strings.Join(up, ", "), "")
		}
		if len(down) > 0 {
			w.add("**"+T("Slowed down:")+"** "+strings.Join(down, ", "), "")
		}
		if len(up) == 0 && len(down) == 0 {
			w.add(Tf("No sharp changes of the commit pace (%d projects with data).", len(trends)), "")
		}
	}
	type scored struct {
		score int
		name  string
		label string
		weak  string
	}
	var solid, risky []scored
	var moves []string
	for _, st := range h.Ordered() {
		hl := h.Health(st)
		s := scored{hl.Score, st.Product.Name, hl.Label(), ""}
		if hl.Coverage >= 0.6 {
			solid = append(solid, s)
		}
		if hl.Grade == "D" || hl.Grade == "F" {
			weakest := -1.0
			for _, p := range hl.Parts {
				if p.Known && (weakest < 0 || p.Score < weakest) {
					weakest, s.weak = p.Score, Tf(p.Note, p.Args...)
				}
			}
			risky = append(risky, s)
		}
		if d, ok := h.Delta(st.Product.ID, "health", 7); ok && (d >= 5 || d <= -5) {
			moves = append(moves, fmt.Sprintf("%s %+.0f", st.Product.Name, d))
		}
	}
	sort.SliceStable(solid, func(i, j int) bool { return solid[i].score > solid[j].score })
	if len(solid) > 0 {
		var top []string
		for _, s := range first(solid, 5) {
			top = append(top, fmt.Sprintf("%s %d", s.name, s.score))
		}
		w.add("**"+T("Health index leaders")+"** "+T("(full data):")+" "+strings.Join(top, ", "), "")
	}
	if len(risky) > 0 {
		var parts []string
		for _, s := range first(risky, 6) {
			parts = append(parts, s.name+" "+s.label+" - "+s.weak)
		}
		w.add("**"+T("At risk (D/F):")+"** "+strings.Join(parts, ", "), "")
	}
	if len(moves) > 0 {
		w.add("**"+T("Index change over a week:")+"** "+strings.Join(moves, ", "), "")
	}
	var movers []string
	for _, st := range h.Ordered() {
		for _, s := range h.Adoption[st.Product.ID] {
			if s.HasTrend && s.Month >= 2000 && (s.Trend >= 15 || s.Trend <= -15) && len(movers) < 8 {
				movers = append(movers, fmt.Sprintf("%s `%s` %+.0f%% (%d/%s)", st.Product.Name, s.Name, s.Trend, s.Month, T("month")))
			}
		}
	}
	if len(movers) > 0 {
		w.add("**"+T("Use, week over week:")+"** "+strings.Join(movers, ", "), "")
	}
	var stale, archived []string
	for _, st := range h.Ordered() {
		if analytics.Stats(st).Status == "stale" {
			stale = append(stale, st.Product.Name)
		}
		if st.GH.Repo != nil && st.GH.Repo.Archived {
			archived = append(archived, st.Product.Name)
		}
	}
	if len(stale) > 0 {
		w.add("**"+T("No releases for over a year:")+"** "+strings.Join(stale, ", "), "")
	}
	if len(archived) > 0 {
		w.add("**⚠ "+T("Archived:")+"** "+strings.Join(archived, ", "), "")
	}
}

func (w *writer) market() {
	h := w.h
	if len(h.Vendors) == 0 || !h.MarketDone {
		return
	}
	w.add("## 💼 "+T("Commercial segment"), "")
	var moves []string
	for _, v := range h.Vendors {
		if p, ok := h.Quotes[v.ID].Change(21); ok && (p >= 10 || p <= -10) {
			moves = append(moves, fmt.Sprintf("%s (%s) %+.0f%%", v.Name, v.Ticker, p))
		}
	}
	if len(moves) > 0 {
		w.add("**"+T("Strong stock moves over a month:")+"** "+strings.Join(moves, ", "), "")
	}
	for _, v := range h.Vendors {
		var recent []*news.Item
		for _, n := range h.VendorNews[v.ID] {
			if !n.Date.Before(w.o.Since) {
				recent = append(recent, n)
			}
		}
		if len(recent) == 0 {
			continue
		}
		gs := news.Dedupe(recent)
		line := fmt.Sprintf("- **%s** (%d): [%s](%s)", v.Name, len(gs), esc(gs[0].Item.Title), gs[0].Item.URL)
		if gs[0].Similar > 0 {
			line += " · _" + Tf("+%d similar", gs[0].Similar) + "_"
		}
		w.add(line)
	}
	w.add("")
}

func (w *writer) standards() {
	h := w.h
	var rfcs, drafts []string
	for _, d := range h.Docs {
		if d.Time.Before(w.o.Since) {
			continue
		}
		if d.Kind == "rfc" {
			rfcs = append(rfcs, fmt.Sprintf("- **%s** (%s): [%s](%s) · %s", d.Label(), d.WG, esc(d.Title), d.URL(), Tf("published %s", d.Time.Format("01-02"))))
		} else if len(drafts) < 12 {
			drafts = append(drafts, fmt.Sprintf("[%s](%s)", d.Label(), d.URL()))
		}
	}
	if len(rfcs)+len(drafts) > 0 {
		w.add("## 📜 "+T("Standards (IETF)"), "")
		w.add(rfcs...)
		if len(drafts) > 0 {
			w.add("", Tf("New revisions of drafts (%d): %s", len(drafts), strings.Join(drafts, ", ")))
		}
		w.add("")
	}
	if len(h.Radar) > 0 {
		stats := append(h.Radar[:0:0], h.Radar...)
		sort.SliceStable(stats, func(i, j int) bool { return stats[i].Count(30, w.now) > stats[j].Count(30, w.now) })
		var top, rising []string
		for _, s := range first(stats, 6) {
			top = append(top, fmt.Sprintf("**%s** %d", T(s.Name), s.Count(30, w.now)))
		}
		for _, s := range stats {
			if p, ok := s.Trend(w.now); ok && p >= 40 && s.Count(7, w.now) >= 3 {
				rising = append(rising, fmt.Sprintf("%s %+.0f%%", T(s.Name), p))
			}
		}
		w.add("## 📡 "+T("Topic radar (30 days)"), "", strings.Join(top, " · "))
		if len(rising) > 0 {
			w.add("", "**"+T("Rising this week:")+"** "+strings.Join(rising, ", "))
		}
		w.add("")
	}
	var soon []string
	for _, m := range h.Meetings {
		if d := int(m.Date.Sub(w.now).Hours() / 24); d >= 0 && d <= 60 {
			soon = append(soon, "- "+Tf("%s: IETF %s (%s), in %d days", m.Date.Format("2006-01-02"), m.Number, m.City, d))
		}
	}
	if len(soon) > 0 {
		w.add("## 📆 "+T("Coming events"), "")
		w.add(soon...)
		w.add("")
	}
}
