// Package ui is the terminal interface of voip-info, built on the voip-tui
// framework (github.com/napadailo/voip-tui/tui).
package ui

import (
	"context"
	"fmt"
	"os/exec"
	"runtime"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/napadailo/voip-tui/i18n"
	"github.com/napadailo/voip-tui/tui"
	"github.com/napadailo/voip-tui/tui/theme"

	"github.com/napadailo/voip-info/internal/catalog"
	"github.com/napadailo/voip-info/internal/config"
	"github.com/napadailo/voip-info/internal/hub"
	"github.com/napadailo/voip-info/internal/ietf"
	"github.com/napadailo/voip-info/internal/news"
	"github.com/napadailo/voip-info/internal/sources/nvd"
)

// Version is set at build time.
var Version = "dev"

var logo = []string{
	"__   _____ ___ ___     ___ _  _ ___ ___  ",
	"\\ \\ / / _ \\_ _| _ \\___|_ _| \\| | __/ _ \\ ",
	" \\ V / (_) | ||  _/___|| || .` | _| (_) |",
	"  \\_/ \\___/___|_|     |___|_|\\_|_| \\___/ ",
}

// App is voip-info: the tui frame plus the hub.
type App struct {
	*tui.App
	cfg *config.Config
	hub *hub.Hub

	category  string // product category filter, "" = all
	watchOnly bool   // only my stack (W)
	loading   bool
}

// New builds the interface over h.
func New(cfg *config.Config, h *hub.Hub) *App {
	a := &App{
		App: tui.New(tui.Options{
			Name:        "voip-info",
			Description: N("voip-info - releases, security and news of the VoIP ecosystem"),
			Logo:        logo,
			Timeout:     3 * time.Minute,
		}),
		cfg: cfg,
		hub: h,
	}
	h.Apply = func(f func()) { a.QueueUpdateDraw(f) }
	if v, _ := h.Store.Setting("watch_only"); v == "1" {
		a.watchOnly = true
	}
	h.OnChange = a.onChange
	a.Info = a.infoLines
	a.Help = a.helpSections
	a.OnTheme = func(name string) { _ = h.Store.SetSetting("theme", name) }
	a.OnLanguage = func(lang string) { _ = h.Store.SetSetting("language", lang) }
	a.Completions = a.completions
	a.Unknown = a.unknown
	a.registerCommands()
	return a
}

// Hub is the shared data.
func (a *App) Hub() *hub.Hub { return a.hub }

// Start loads the data in the background and runs the interface.
func (a *App) Start(initial string) error {
	if initial == "" {
		initial = "products"
	}
	go a.load(false)
	return a.App.Start(initial, "products")
}

func (a *App) load(force bool) {
	a.QueueUpdateDraw(func() { a.loading = true; a.UpdateInfo() })
	// NVD without a key takes a request per 6.5 seconds
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()
	a.hub.LoadReleases(ctx, force)
	a.QueueUpdateDraw(func() { a.loading = false; a.UpdateInfo() })
	// the context ends when all of them are done
	var wg sync.WaitGroup
	for _, f := range []func(){
		func() { a.hub.LoadSupportDates(ctx) },
		func() { a.hub.LoadSecurity(ctx, force) },
		func() { a.hub.LoadTableMetrics(ctx, force) },
		func() { a.hub.LoadInfo(ctx, force) },
	} {
		wg.Add(1)
		go func() { defer wg.Done(); f() }()
	}
	wg.Wait()
}

// onChange runs on the UI goroutine when the hub has news.
func (a *App) onChange(string) {
	a.UpdateInfo()
	for _, v := range a.Stack() {
		if r, ok := v.(interface{ Rebuild() }); ok {
			r.Rebuild()
		}
	}
}

func (a *App) infoLines() [][2]string {
	f := a.hub.Fetcher
	// both tokens on one line: the header holds six
	tok := "GitHub " + theme.Tag(theme.OK) + T("yes") + "[-]"
	if !f.HasToken() {
		tok = "GitHub " + theme.Tag(theme.Warn) + T("none") + "[-] " + T("(60 req/h)")
	}
	if nvd.HasKey() {
		tok += " · NVD " + theme.Tag(theme.OK) + T("yes") + "[-]"
	} else {
		tok += " · " + tui.Faint("NVD "+T("none"))
	}
	api := "-"
	if r := f.Rate(); r.Known {
		c := theme.InfoValue
		if r.Remaining < 10 {
			c = theme.Err
		}
		api = fmt.Sprintf("%s%d/%d[-] %s%s[-]", theme.Tag(c), r.Remaining, r.Limit, theme.Tag(theme.Dim), Tf("reset %s", r.Reset.Format("15:04")))
	}
	upd := Tf("loading %d/%d", a.hub.Loaded, len(a.hub.Products))
	if !a.loading && !a.hub.LastUpdate.IsZero() {
		upd = a.hub.LastUpdate.Format("15:04:05")
	}
	return [][2]string{
		{N("Tokens"), tok},
		{N("API"), api},
		{N("Category"), a.categoryInfo()},
		{N("New releases"), theme.Tag(theme.OK) + fmt.Sprint(a.hub.NewCount()) + "[-]" + a.newsInfo()},
		{N("CVE/30d"), a.securityInfo()},
		{N("Updated"), upd + " · " + tui.Faint(i18n.Lang()+" · "+Version)},
	}
}

// categoryInfo is the category filter and its keys.
func (a *App) categoryInfo() string {
	cat, n := T("all"), len(a.hub.Products)
	if a.category != "" {
		cat, n = a.category, 0
		for _, p := range a.hub.Products {
			if p.Category == a.category {
				n++
			}
		}
	}
	return theme.Tag(theme.InfoValue) + esc(cat) + "[-] · " + Tf("%d products", n)
}

// newsInfo is the number of news items of the last day.
func (a *App) newsInfo() string {
	if !a.hub.NewsLoaded {
		return ""
	}
	return " · " + Tf("news 24h: %d", a.hub.NewsSince(time.Now().Add(-24*time.Hour)))
}

// securityInfo is the info line of vulnerabilities: new in 30 days, those of
// the catalog exploited in attacks (KEV), the NVD progress.
func (a *App) securityInfo() string {
	h := a.hub
	n, serious, exploited := 0, 0, 0
	for _, v := range h.AllVulns() {
		if v.AgeDays() <= 30 {
			n++
			if v.Serious() {
				serious++
			}
		}
		if h.KEVOf(v) != nil {
			exploited++
		}
	}
	c := theme.InfoValue
	if serious > 0 {
		c = theme.Err
	}
	s := theme.Tag(c) + fmt.Sprint(n) + "[-]"
	if serious > 0 {
		s += " " + Tf("(high+ %d)", serious)
	}
	if exploited > 0 {
		s += " · " + theme.Tag(theme.Err) + "KEV " + fmt.Sprint(exploited) + "[-]"
	}
	if h.NVDTotal > 0 && h.NVDDone < h.NVDTotal {
		s += " " + theme.Tag(theme.Dim) + Tf("NVD %d/%d", h.NVDDone, h.NVDTotal) + "[-]"
	}
	return s
}

func (a *App) helpSections(h *tui.Help) {
	h.Section(T("Products and releases"))
	h.Key("enter", T("product card: releases and notes"))
	h.Key("b", T("open in the browser"))
	h.Key("r", T("reload everything from the network"))
	h.Key("n", T("mark all releases seen"))
	h.Key("1-9 / 0", T("category / all"))
	h.Section(T("Security"))
	h.Key(":security", T("vulnerabilities of the products; R sorts by risk (KEV, EPSS, CVSS)"))
	h.Key(":kev", T("VoIP vulnerabilities exploited in attacks (CISA KEV)"))
	h.Key(":have id ver", T("record the version in use; :have id - removes it"))
	h.Key(":mine", T("my versions: vulnerabilities, newer releases, support"))
	h.Key(":upgrade id", T("upgrade path from my version: CVEs closed, breaking changes"))
	h.Section(T("News, market, standards"))
	h.Key(":news [kind]", T("news of the projects, industry and market; 1-6 pick the kind, 0 all"))
	h.Key(":radar", T("topics of the news, releases, lists and IETF: 7 and 30 days, trend"))
	h.Key(":ietf", T("drafts and RFCs of the VoIP and RTC working groups"))
	h.Key(":events", T("conferences and IETF meetings"))
	h.Key(":market", T("vendors: quotes and news"))
	h.Section(T("My stack and the digest"))
	h.Key("w", T("watch the product, topic, vendor or WG of the row (again: unwatch)"))
	h.Key("W", T("only my stack in the lists: watched, my versions and their dependencies"))
	h.Key(":watch [q]", T("my stack; :watch <id|topic|vendor|wg> subscribes, :unwatch drops"))
	h.Key(":digest [days] [mine]", T("what changed over a period; e saves .md, :export too"))
	h.Section(T("Product card"))
	h.Key("1-9, tab", T("tabs"))
	h.Key("enter", T("release notes; esc returns to the list"))
	h.Section(T("Categories"))
	var cats []string
	for i, c := range catalog.Categories {
		cats = append(cats, fmt.Sprintf("%d %s (:%s)", i+1, c.Name, c.Alias))
	}
	h.Line("  " + strings.Join(cats, ", "))
}

// watchCompletions are the products, vendors and working groups.
func (a *App) watchCompletions() []string {
	out := a.productIDs()
	for _, v := range a.hub.Vendors {
		out = append(out, v.ID)
	}
	for _, w := range ietf.WGs {
		out = append(out, w.Name)
	}
	return out
}

func (a *App) productIDs() []string {
	var out []string
	for _, p := range a.hub.Products {
		out = append(out, p.ID)
	}
	return out
}

func (a *App) completions() []string {
	var out []string
	for _, p := range a.hub.Products {
		out = append(out, p.ID)
	}
	for _, c := range catalog.Categories {
		out = append(out, c.Alias)
	}
	return out
}

// unknown opens a product card by id or filters by a category alias.
func (a *App) unknown(name, _ string) (bool, error) {
	if st, ok := a.hub.States[name]; ok {
		a.Push(newProductView(a, st, "", ""))
		return true, nil
	}
	if name == "all" {
		a.SetCategory("")
		return true, nil
	}
	if cat, ok := catalog.CategoryByAlias(name); ok {
		a.SetCategory(cat)
		return true, nil
	}
	return false, nil
}

// SetCategory filters the product lists.
func (a *App) SetCategory(cat string) {
	a.category = cat
	a.UpdateInfo()
	a.onChange("category")
}

// Category is the product category filter.
func (a *App) Category() string { return a.category }

func (a *App) registerCommands() {
	a.AddCommands(
		&tui.Command{Names: []string{"products", "p", "prod"}, Help: N("products: latest release, branches, activity"),
			Run: func(string) error { a.Replace(newProductsView(a)); return nil }},
		&tui.Command{Names: []string{"releases", "rel"}, Help: N("all releases, newest first"),
			Run: func(string) error { a.Replace(newReleasesView(a)); return nil }},
		&tui.Command{Names: []string{"health", "hl"}, Help: N("health index of the products, by component"),
			Run: func(string) error { a.Replace(newHealthView(a)); return nil }},
		&tui.Command{Names: []string{"eol", "lts", "support"}, Help: N("support of the release branches: LTS, security-only, EOL"),
			Run: func(string) error { a.Replace(newEOLView(a)); return nil }},
		&tui.Command{Names: []string{"deps", "graph"}, Help: N("dependencies between the products"),
			Run: func(string) error { a.Replace(newDepsView(a)); return nil }},
		&tui.Command{Names: []string{"security", "sec", "cve", "vuln"}, Help: N("vulnerabilities: NVD and GitHub advisories, KEV, EPSS"),
			Run: func(string) error { a.Replace(newSecurityView(a)); return nil }},
		&tui.Command{Names: []string{"kev", "exploited"}, Help: N("VoIP vulnerabilities from the CISA KEV catalog"),
			Run: func(string) error { a.Replace(newKEVView(a)); return nil }},
		&tui.Command{Names: []string{"mine", "my", "inventory", "inv"}, Help: N("my versions and what follows from them"),
			Run: func(string) error { a.Replace(newMineView(a)); return nil }},
		&tui.Command{Names: []string{"have"}, Args: "<product-id> <version> [note]", Help: N("record the version of a product in use"),
			Complete: a.productIDs(), Run: a.have},
		&tui.Command{Names: []string{"upgrade", "up"}, Args: "<product-id> [from] [target]", Help: N("upgrade path within a branch"),
			Complete: a.productIDs(), Run: a.upgradeCmd},
		&tui.Command{Names: []string{"news", "ns"}, Args: "[" + strings.Join(news.Kinds, "|") + "]", Help: N("news of the projects, industry, market and lists"),
			Complete: news.Kinds, Run: func(args string) error {
				kind := strings.TrimSpace(args)
				if kind != "" && !slices.Contains(news.Kinds, kind) {
					return fmt.Errorf("%s", Tf("unknown kind %s: %s", kind, strings.Join(news.Kinds, ", ")))
				}
				a.Replace(newNewsView(a, kind))
				return nil
			}},
		&tui.Command{Names: []string{"radar", "topics", "trends"}, Help: N("topics: what the news, releases, lists and IETF talk about"),
			Run: func(string) error { a.Replace(newRadarView(a)); return nil }},
		&tui.Command{Names: []string{"ietf", "rfc", "standards"}, Help: N("IETF drafts and RFCs of the VoIP and RTC working groups"),
			Run: func(string) error { a.Replace(newIETFView(a)); return nil }},
		&tui.Command{Names: []string{"events", "conf", "calendar"}, Help: N("conferences and IETF meetings"),
			Run: func(string) error { a.Replace(newEventsView(a)); return nil }},
		&tui.Command{Names: []string{"market", "vendors", "biz"}, Help: N("commercial vendors: quotes and news"),
			Run: func(string) error { a.Replace(newMarketView(a)); return nil }},
		&tui.Command{Names: []string{"watch"}, Args: "[id|topic|vendor|wg]", Help: N("my stack; with an argument subscribes to a product, topic, vendor or IETF WG"),
			Complete: a.watchCompletions(), Run: a.watchCmd(true)},
		&tui.Command{Names: []string{"unwatch"}, Args: "<id|topic|vendor|wg>", Help: N("drop a subscription"),
			Complete: a.watchCompletions(), Run: a.watchCmd(false)},
		&tui.Command{Names: []string{"digest"}, Args: "[days] [mine|all]", Help: N("what changed over a period, as one page; e saves it"),
			Complete: []string{"1", "7", "30", "mine", "all"}, Run: a.digestCmd},
		&tui.Command{Names: []string{"export"}, Args: "[days] [mine|all]", Help: N("save the digest as Markdown in the data directory"),
			Complete: []string{"1", "7", "30", "mine", "all"}, Run: a.exportCmd},
		&tui.Command{Names: []string{"reload"}, Help: N("reload everything from the network"),
			Run: func(string) error { a.Reload(); return nil }},
		&tui.Command{Names: []string{"seen"}, Help: N("mark all releases seen"),
			Run: func(string) error { a.MarkAllSeen(); return nil }},
		// voip-info remembers the language itself: the built-in message asks to edit the config
		&tui.Command{Names: []string{"lang", "language"}, Args: "[" + strings.Join(i18n.Languages(), "|") + "]",
			Help: N("switch the interface language, no argument shows the current one"), Complete: i18n.Languages(),
			Run: func(args string) error {
				if args = strings.TrimSpace(args); args == "" {
					a.Flash().Infof("language: %s (available: %s)", i18n.Lang(), strings.Join(i18n.Languages(), ", "))
					return nil
				}
				if err := a.SetLanguage(args); err != nil {
					return err
				}
				a.Flash().Infof("language %s", i18n.Lang())
				return nil
			}},
	)
}

// Reload fetches everything again, bypassing the cache TTL.
func (a *App) Reload() {
	a.Flash().Info(T("reloading…"))
	go a.load(true)
}

// MarkAllSeen marks every release seen.
func (a *App) MarkAllSeen() {
	for _, st := range a.hub.States {
		a.hub.MarkSeen(st)
	}
	a.onChange("seen")
}

// OpenURL opens a link in the browser.
func (a *App) OpenURL(url string) {
	if url == "" {
		return
	}
	cmd := "xdg-open"
	if runtime.GOOS == "darwin" {
		cmd = "open"
	}
	go func() {
		if err := exec.Command(cmd, url).Run(); err != nil {
			a.QueueUpdateDraw(func() { a.Flash().Errf("cannot open %s: %v", url, err) })
		}
	}()
}

// bindCategories binds 1-9 and 0 in a product list.
func (a *App) bindCategories(t *TableView) {
	for i, c := range catalog.Categories {
		name := c.Name
		t.Bind(rune('1'+i), func() { a.SetCategory(name) })
	}
	t.Bind('0', func() { a.SetCategory("") })
}

// age formats days as today, 5d, 3mo, 2y in the interface language.
func age(days int) string {
	switch {
	case days < 1:
		return T("today")
	case days < 60:
		return Tf("%dd", days)
	case days < 730:
		return Tf("%dmo", days/30)
	}
	return Tf("%dy", days/365)
}
