package ui

import (
	"fmt"
	"strings"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"

	"github.com/napadailo/voip-tui/i18n"
	"github.com/napadailo/voip-tui/tui"
	"github.com/napadailo/voip-tui/tui/theme"

	"github.com/napadailo/voip-info/internal/analytics"
	"github.com/napadailo/voip-info/internal/dockerhub"
	"github.com/napadailo/voip-info/internal/ghapi"
	"github.com/napadailo/voip-info/internal/htmltext"
	"github.com/napadailo/voip-info/internal/model"
	"github.com/napadailo/voip-info/internal/news"
)

// productTabs are the tabs of the product card, in key order; tabIDs name them for callers.
var (
	productTabs = []string{N("Releases"), N("Activity"), N("Roadmap"), N("Stats"), N("Deps"), N("Security"), N("News"), N("Docker")}
	tabIDs      = []string{"releases", "activity", "roadmap", "stats", "deps", "security", "news", "docker"}
	textTabs    = tabIDs[2:5] // the tabs that are a text pane
)

// ProductView is the product card: a header, tabs and their panes.
type ProductView struct {
	*tview.Flex
	app *App
	st  *model.ProductState

	header *tview.TextView
	tabs   *tview.TextView
	pages  *tview.Pages
	tab    int

	rels  *TableView
	notes *tview.TextView

	actSum  *tview.TextView
	act     *TableView
	texts   map[string]*tview.TextView // roadmap, stats, deps, security
	loading bool

	newsT    *TableView
	newsText *tview.TextView

	secHead *tview.TextView
	secT    *TableView
	secFlex *tview.Flex

	imgT       *TableView
	imgText    *tview.TextView
	images     []*dockerhub.Image
	imgLoading bool
	imgLoaded  bool
}

func newProductView(a *App, st *model.ProductState, focusTag, tab string) *ProductView {
	v := &ProductView{app: a, st: st}
	v.header = tview.NewTextView().SetDynamicColors(true).SetWrap(true)
	v.header.SetBorder(true).SetBorderPadding(0, 0, 1, 1)
	v.tabs = tview.NewTextView().SetDynamicColors(true).SetWrap(false)

	v.rels = tui.NewTableView(a.App, N("Releases"), []string{N("TAG"), N("BRANCH"), N("CHANGES"), N("DATE"), N("AGE")})
	v.rels.OnSelect(func(r *Row) { v.showNotes(r.Data.(*model.Release)) })
	v.rels.OnEnter(func(r *Row) { v.openRelease(r.Data.(*model.Release)) })
	v.rels.Bind('b', func() {
		if r := v.rels.Selected(); r != nil {
			a.OpenURL(r.Data.(*model.Release).URL)
		}
	})
	v.rels.SetInputCapture(v.chain(v.rels.HandleKey))
	v.notes = tview.NewTextView().SetDynamicColors(true).SetWrap(true).SetWordWrap(true)
	v.notes.SetBorder(true).SetBorderPadding(0, 0, 1, 1)

	releases := tview.NewFlex().AddItem(v.rels, 0, 2, true).AddItem(v.notes, 0, 3, false)
	v.pages = tview.NewPages().AddPage("0", releases, true, true)

	v.actSum = tview.NewTextView().SetDynamicColors(true).SetWrap(true)
	v.act = tui.NewTableView(a.App, N("Issues and PRs, 30 days"), []string{N("TYPE"), N("STATE"), "#", N("TITLE"), N("AUTHOR"), N("UPDATED"), N("COMMENTS")})
	v.act.OnEnter(func(r *Row) { v.openIssue(r.Data.(ghapi.Issue)) })
	v.act.Bind('b', func() {
		if r := v.act.Selected(); r != nil {
			a.OpenURL(r.Data.(ghapi.Issue).URL)
		}
	})
	v.act.SetInputCapture(v.chain(v.act.HandleKey))
	v.pages.AddPage("1", tview.NewFlex().SetDirection(tview.FlexRow).AddItem(v.actSum, 2, 0, false).AddItem(v.act, 0, 1, true), true, false)
	v.texts = map[string]*tview.TextView{}
	for i, id := range textTabs {
		tv := tview.NewTextView().SetDynamicColors(true).SetWrap(true).SetWordWrap(true)
		tv.SetBorder(true).SetBorderPadding(0, 0, 1, 1)
		tv.SetInputCapture(v.chain(v.browserKey(func() string { return v.tabURL(id) })))
		v.texts[id] = tv
		v.pages.AddPage(fmt.Sprint(i+2), tv, true, false)
	}
	v.pages.AddPage("5", v.newSecurityPane(), true, false)
	v.newsT, v.newsText = v.listPane(N("News"), []string{N("DATE"), N("SOURCE"), N("TITLE")}, func(r *Row) { v.showNewsItem(r) },
		func(r *Row) { a.showNews(r.Data.(*news.Item)) }, func(r *Row) string { return r.Data.(*news.Item).URL })
	v.pages.AddPage("6", tview.NewFlex().AddItem(v.newsT, 0, 2, true).AddItem(v.newsText, 0, 3, false), true, false)
	v.imgT, v.imgText = v.listPane(N("Docker images"), []string{N("IMAGE"), N("PULLS"), N("STARS"), N("UPDATED"), N("VERSION"), N("LAG"), N("ARCH")},
		func(r *Row) { v.showImage(r.Data.(*dockerhub.Image)) }, func(r *Row) {
			img := r.Data.(*dockerhub.Image)
			a.showCard(img.Name, v.imageCard(img), img.URL())
		}, func(r *Row) string { return r.Data.(*dockerhub.Image).URL() })
	v.pages.AddPage("7", tview.NewFlex().AddItem(v.imgT, 0, 3, true).AddItem(v.imgText, 0, 4, false), true, false)
	v.Flex = tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(v.header, 6, 0, false).
		AddItem(v.tabs, 1, 0, false).
		AddItem(v.pages, 0, 1, true)

	v.Rebuild()
	if focusTag != "" {
		v.rels.SelectKey(focusTag)
	}
	if r := v.rels.Selected(); r != nil {
		v.showNotes(r.Data.(*model.Release))
	}
	for i, id := range tabIDs {
		if id == tab {
			v.setTab(i)
		}
	}
	a.hub.MarkSeen(st)
	if !st.GH.Full {
		go func() {
			ctx, cancel := a.Ctx()
			defer cancel()
			a.hub.LoadAPI(ctx, st, false)
		}()
	}
	return v
}

func (v *ProductView) Name() string { return v.st.Product.ID }
func (v *ProductView) Close()       {}
func (v *ProductView) Refresh()     { v.Rebuild() }

// Table is the list of the active tab while it has the focus (it takes the / filter).
func (v *ProductView) Table() *TableView {
	switch {
	case v.tab == 0:
		return v.rels
	case v.tab == 1:
		return v.act
	case v.tab == 5:
		return v.secT
	case v.tab == 6:
		return v.newsT
	case v.tab == 7:
		return v.imgT
	}
	return nil
}

func (v *ProductView) Hints() []Hint {
	return []Hint{{Key: "enter", Desc: N("open")}, {Key: "b", Desc: N("browser")}, {Key: "1-9/tab", Desc: N("tabs")}}
}

// Focus goes to the main pane of the active tab.
func (v *ProductView) Focus(delegate func(p tview.Primitive)) { delegate(v.pane()) }

func (v *ProductView) pane() tview.Primitive {
	switch v.tab {
	case 0:
		return v.rels
	case 1:
		return v.act
	case 5:
		return v.secT
	case 6:
		return v.newsT
	case 7:
		return v.imgT
	}
	return v.texts[tabIDs[v.tab]]
}

// Draw applies the current theme.
func (v *ProductView) Draw(screen tcell.Screen) {
	for _, tv := range []*tview.TextView{v.header, v.tabs, v.notes, v.actSum, v.newsText, v.imgText, v.secHead} {
		tv.SetBackgroundColor(theme.Bg)
		tv.SetTextColor(theme.Fg)
	}
	for _, tv := range v.texts {
		tv.SetBackgroundColor(theme.Bg)
		tv.SetTextColor(theme.Fg)
	}
	v.header.SetBorderColor(theme.InfoKey)
	for _, tv := range []*tview.TextView{v.notes, v.newsText, v.imgText, v.secHead} {
		tv.SetBorderColor(theme.Border).SetTitleColor(theme.Title)
	}
	for _, tv := range v.texts {
		tv.SetBorderColor(theme.Border).SetTitleColor(theme.Title)
	}
	v.Flex.SetBackgroundColor(theme.Bg)
	v.pages.SetBackgroundColor(theme.Bg)
	v.Flex.Draw(screen)
}

// chain handles the card's keys (tabs) before the pane's own.
func (v *ProductView) chain(next func(*tcell.EventKey) *tcell.EventKey) func(*tcell.EventKey) *tcell.EventKey {
	return func(ev *tcell.EventKey) *tcell.EventKey {
		switch ev.Key() {
		case tcell.KeyTab:
			v.setTab((v.tab + 1) % len(productTabs))
			return nil
		case tcell.KeyBacktab:
			v.setTab((v.tab + len(productTabs) - 1) % len(productTabs))
			return nil
		case tcell.KeyRune:
			if r := ev.Rune(); r >= '1' && r <= '9' && int(r-'1') < len(productTabs) {
				v.setTab(int(r - '1'))
				return nil
			}
		}
		if next != nil {
			return next(ev)
		}
		return ev
	}
}

func (v *ProductView) setTab(t int) {
	v.tab = t
	if tabIDs[t] == "docker" {
		v.loadImages()
	}
	v.pages.SwitchToPage(fmt.Sprint(t))
	v.renderTabs()
	v.app.SetFocus(v.pane())
	v.app.RefreshHints()
}

func (v *ProductView) renderTabs() {
	var b strings.Builder
	for i, name := range productTabs {
		label := fmt.Sprintf(" %d %s ", i+1, T(name))
		if i == v.tab {
			fmt.Fprintf(&b, "[%s:%s:b]%s[-:-:-] ", theme.Name(theme.CrumbFg), theme.Name(theme.CrumbLast), tview.Escape(label))
		} else {
			fmt.Fprintf(&b, "%s%s[-] ", theme.Tag(theme.Dim), tview.Escape(label))
		}
	}
	v.tabs.SetText(b.String())
}

// Rerender rebuilds the markup after a theme or language switch.
func (v *ProductView) Rerender() {
	v.Rebuild()
	if r := v.rels.Selected(); r != nil {
		v.showNotes(r.Data.(*model.Release))
	}
}

// Rebuild refreshes the card from the hub.
func (v *ProductView) Rebuild() {
	v.renderHeader()
	v.renderTabs()
	v.fillReleases()
	v.fillActivity()
	v.renderRoadmap()
	v.renderStats()
	v.renderDeps()
	v.renderSecurity()
	v.fillNews()
	v.fillImages()
}

// headIcons mark the fields of the card header. Emoji of two cells and no
// variation selector: they keep their width in every terminal.
var headIcons = map[string]string{
	"latest": "🚀", "pre-release": "🧪", "cadence": "📅", "releases/90d": "📦", "live branches": "🌿", "status": "📊",
	"stars": "⭐", "forks": "🍴", "open issues+PRs": "🐛", "commits 12w": "📝", "language": "💻", "license": "📜", "health": "💚",
}

// headKey is a field label of the card header: its icon and the label, dimmed.
func headKey(label string) string {
	icon := ""
	if i, ok := headIcons[label]; ok {
		icon = i + " "
	}
	return icon + theme.Tag(theme.Dim) + tview.Escape(T(label)) + ":[-] "
}

func (v *ProductView) renderHeader() {
	p := v.st.Product
	s := analytics.Stats(v.st)
	k := headKey
	latest := "-"
	if lt := v.st.Latest(); lt != nil {
		latest = tui.Strong(lt.Tag) + " " + tui.Faint("("+age(lt.AgeDays())+")")
	}
	if pre := v.st.LatestPre(); pre != nil {
		latest += "  " + k(N("pre-release")) + tui.Paint(theme.Accent, pre.Tag)
	}
	cad := "-"
	if s.Cadence > 0 {
		cad = Tf("%.0f days", s.Cadence)
	}
	branches := strings.Join(s.Branches, ", ")
	if branches == "" {
		branches = "-"
	}
	lines := []string{
		fmt.Sprintf("%s[::b]%s[::-][-]  %s  📁 %s  🌐 %s", theme.Tag(theme.Title), tview.Escape(p.Name), tui.Faint(p.Category),
			tui.Paint(theme.Key, p.Repo), tui.Faint(p.Homepage)),
		tview.Escape(p.Desc.In(i18n.Lang())),
		k(N("latest")) + latest + "   " + k(N("cadence")) + cad + "   " + k(N("releases/90d")) + fmt.Sprint(s.Rel90d) +
			"   " + k(N("live branches")) + tview.Escape(branches) + "   " + k(N("status")) + T(s.Status),
		v.githubLine(),
	}
	v.header.SetText(strings.Join(lines, "\n"))
}

func (v *ProductView) fillReleases() {
	commits := len(v.st.Releases) > 0 && v.st.Releases[0].Commit
	if commits {
		v.rels.SetHeaders([]string{N("COMMIT"), N("MESSAGE"), "", N("DATE"), N("AGE")})
	} else {
		v.rels.SetHeaders([]string{N("TAG"), N("BRANCH"), N("CHANGES"), N("DATE"), N("AGE")})
	}
	rows := make([]Row, 0, len(v.st.Releases))
	for _, r := range v.st.Releases {
		second := r.Branch()
		if r.Commit {
			second = tui.Trunc(r.Title, 40)
		}
		tag := r.Tag
		tones := map[int]tui.Tone{4: ageTone(r.AgeDays())}
		changes, ct := badge(r)
		tones[2] = ct
		switch {
		case v.st.IsUnseen(r):
			tag = "● " + tag
			tones[0] = tui.ToneOK
		case r.Prerelease():
			tones[0] = tui.ToneDim
		}
		rows = append(rows, Row{Cells: []string{tag, second, changes, r.Date.Format("2006-01-02"), age(r.AgeDays())},
			CellTones: tones, Markup: map[int]bool{2: true}, Sort: map[int]float64{4: float64(r.AgeDays())}, Data: r, Key: r.Tag})
	}
	v.rels.SetRows(rows)
}

// openRelease shows the notes of a release on the whole screen: / searches,
// b opens the release, esc returns to the card.
func (v *ProductView) openRelease(r *model.Release) {
	tv := tui.NewTextViewer(v.app.App, v.st.Product.Name+" "+r.Tag, "markup")
	tv.SetWrap(true)
	tv.SetContent(v.notesMarkup(r), "markup")
	tv.Keys['b'] = func() { v.app.OpenURL(r.URL) }
	tv.Extra = []Hint{{Key: "b", Desc: N("browser")}}
	v.app.Push(tv)
}

// notesMarkup is a release: the tag, the date, the link and the notes.
func (v *ProductView) notesMarkup(r *model.Release) string {
	head := fmt.Sprintf("%s · %s UTC · %s\n\n", tui.Strong(r.Tag), r.Date.UTC().Format("2006-01-02 15:04"), tui.Faint(r.URL))
	body := htmltext.Render(r.NotesHTML)
	if strings.TrimSpace(body) == "" {
		body = tui.Faint(T("No release notes, only a tag."))
	}
	return head + body
}

// showNotes previews the notes of the selected release beside the list.
func (v *ProductView) showNotes(r *model.Release) {
	v.notes.SetTitle(" " + tview.Escape(r.Title) + " ")
	v.notes.SetText(v.notesMarkup(r))
	v.notes.ScrollToBeginning()
}
