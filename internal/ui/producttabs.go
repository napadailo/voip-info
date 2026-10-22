package ui

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/napadailo/voip-tui/tui"
	"github.com/napadailo/voip-tui/tui/theme"

	"github.com/napadailo/voip-info/internal/analytics"
	"github.com/napadailo/voip-info/internal/deps"
	"github.com/napadailo/voip-info/internal/ghapi"
	"github.com/napadailo/voip-info/internal/health"
	"github.com/napadailo/voip-info/internal/store"
)

// noAPI is what a tab shows while the GitHub API has not answered.
func (v *ProductView) noAPI() string {
	if v.st.GH.Loading {
		return tui.Faint(T("loading the GitHub API…"))
	}
	if !v.app.hub.Fetcher.HasToken() {
		return tui.Faint(T("no GitHub API data: without a token there are 60 requests per hour"))
	}
	return tui.Faint(T("no GitHub API data"))
}

// githubLine is the header line of GitHub metrics.
func (v *ProductView) githubLine() string {
	k := headKey
	r := v.st.GH.Repo
	if r == nil {
		return v.noAPI()
	}
	stars := num(r.Stars)
	if d, ok := v.app.hub.Delta(v.st.Product.ID, "stars", 7); ok && d != 0 {
		stars += " " + tui.Paint(theme.OK, fmt.Sprintf("%+.0f/7d", d))
	}
	commits := "-"
	if cur, _, pct, ok := analytics.CommitTrend(v.st.GH.Weekly); v.st.GH.Weekly != nil {
		commits = fmt.Sprint(cur)
		if ok {
			commits += fmt.Sprintf(" (%+.0f%%)", pct)
		}
	}
	h := v.app.hub.Health(v.st)
	line := k(N("stars")) + stars + "   " + k(N("forks")) + num(r.Forks) + "   " + k(N("open issues+PRs")) + fmt.Sprint(r.OpenIssues) +
		"   " + k(N("commits 12w")) + commits + "   " + k(N("language")) + esc(r.Language) + "   " + k(N("license")) + esc(r.LicenseID()) +
		"   " + k(N("health")) + gradeColor(h.Grade) + h.Label() + "[-]"
	if r.Archived {
		line += "   " + tui.Paint(theme.Err, T("ARCHIVED"))
	}
	return line
}

func (v *ProductView) fillActivity() {
	items := v.st.GH.Activity
	if items == nil {
		v.actSum.SetText(v.noAPI())
		v.act.SetRows(nil)
		return
	}
	a := analytics.ActivityStats(items, 30)
	closeTime := "-"
	if a.MedianClose > 0 {
		closeTime = Tf("%.1f days", a.MedianClose)
	}
	bus := ""
	if a.TopAuthor != "" {
		c := theme.Fg
		if a.TopShare > .6 && a.PRsMerged >= 5 {
			c = theme.Err
		}
		bus = "   " + tui.Faint(T("top author")+": ") + tui.Paint(c, fmt.Sprintf("@%s %.0f%%", a.TopAuthor, a.TopShare*100))
	}
	sum := Tf("30 days: issues +%d / closed %d, median close %s   PRs +%d / merged %d / rejected %d   PR authors %d",
		a.IssuesOpened, a.IssuesClosed, closeTime, a.PRsOpened, a.PRsMerged, a.PRsRejected, a.Authors) + bus
	if a.Truncated {
		sum += "  " + tui.Paint(theme.Warn, T("(the sample is cut at 300)"))
	}
	v.actSum.SetText(sum)
	sorted := append([]ghapi.Issue(nil), items...)
	sort.SliceStable(sorted, func(i, j int) bool { return sorted[i].Updated > sorted[j].Updated })
	rows := make([]Row, 0, len(sorted))
	for _, it := range sorted {
		kind, state, tone := T("issue"), T(it.State), tui.ToneFg
		if it.State == "open" {
			tone = tui.ToneOK
		} else {
			tone = tui.ToneDim
		}
		if it.PR != nil {
			kind = "PR"
			switch {
			case it.PR.Merged != "":
				state, tone = T("merged"), tui.ToneAccent
			case it.State == "closed":
				state, tone = T("closed"), tui.ToneErr
			}
		}
		upd := int(time.Since(ghapi.Time(it.Updated)).Hours() / 24)
		rows = append(rows, Row{Cells: []string{kind, state, fmt.Sprint(it.Number), it.Title, it.User.Login, age(upd), fmt.Sprint(it.Comments)},
			CellTones: map[int]tui.Tone{0: map[bool]tui.Tone{true: tui.ToneAccent, false: tui.ToneFg}[it.PR != nil], 1: tone, 5: ageTone(upd)},
			Data:      it, Key: fmt.Sprint(it.Number)})
	}
	v.act.SetRows(rows)
}

// openIssue shows an issue or a pull request on the whole screen: state,
// dates, labels and its description; b opens it, esc returns to the card.
func (v *ProductView) openIssue(it ghapi.Issue) {
	kind, state := T("issue"), T(it.State)
	if it.PR != nil {
		kind = "PR"
		switch {
		case it.PR.Merged != "":
			state = T("merged")
		case it.State == "closed":
			state = T("closed")
		}
	}
	c := tui.NewCard(fmt.Sprintf("%s #%d: %s", kind, it.Number, it.Title))
	c.Sub(v.st.Product.Name, state, "@"+it.User.Login, Tf("comments: %d", it.Comments))
	date := func(s string) string {
		if t := ghapi.Time(s); !t.IsZero() {
			return t.Format("2006-01-02 15:04")
		}
		return "-"
	}
	c.KV(N("created"), date(it.Created))
	c.KV(N("updated"), date(it.Updated))
	if it.Closed != "" {
		c.KV(N("closed"), date(it.Closed))
	}
	if len(it.Labels) > 0 {
		var names []string
		for _, l := range it.Labels {
			names = append(names, l.Name)
		}
		c.KV(N("labels"), tui.Paint(theme.Accent, strings.Join(names, ", ")))
	}
	c.Line(tui.Faint(it.URL))
	body := strings.TrimSpace(it.Body)
	if body == "" {
		body = tui.Faint(T("no description"))
	} else {
		body = mdMarkup(body)
	}
	tv := tui.NewTextViewer(v.app.App, fmt.Sprintf("%s #%d", kind, it.Number), "markup")
	tv.SetWrap(true)
	tv.SetContent(c.String()+"\n"+body, "markup")
	tv.Keys['b'] = func() { v.app.OpenURL(it.URL) }
	tv.Extra = []Hint{{Key: "b", Desc: N("browser")}}
	v.app.Push(tv)
}

func (v *ProductView) renderRoadmap() {
	tv := v.texts["roadmap"]
	tv.SetTitle(" " + T("Roadmap and branch support") + " ")
	var b strings.Builder
	if pre := v.st.LatestPre(); pre != nil {
		b.WriteString(section(N("Next version")))
		b.WriteString(Tf("%s - a pre-release of %s (%s ago); the stable one usually follows within weeks.",
			tui.Strong(pre.Tag), pre.Date.Format("2006-01-02"), age(pre.AgeDays())) + "\n")
	}
	if len(v.st.Releases) > 0 && v.st.Releases[0].Commit {
		b.WriteString("\n" + tui.Faint(T("The project publishes no releases or tags: its commits are followed.")) + "\n")
	}
	if sup := v.app.hub.Support(v.st); len(sup) > 0 {
		src := T("the project's policy")
		if sup[0].Source == "heuristic" {
			src = T("a heuristic by branch activity")
		}
		b.WriteString(sectionMarkup(esc(T("Branch support") + " (" + src + ")")))
		var rows [][]string
		for _, br := range sup {
			latest := "-"
			if br.Latest != nil {
				latest = br.Latest.Tag
			}
			note := ""
			if br.Note != "" {
				note = tui.Faint(tr(br.Note, splitArgs(br.NoteArg)))
			}
			rows = append(rows, []string{esc(br.Branch), esc(br.Kind), esc(latest), day(br.FullUntil), day(br.EOL),
				tui.Paint(supportTone(br.Status).Color(), supportLabel(br.Status)), note})
		}
		b.WriteString(textTable([]string{N("BRANCH"), N("TYPE"), N("LATEST"), N("FULL UNTIL"), N("EOL"), N("STATUS"), N("NOTE")}, rows))
	}
	b.WriteString(section(N("Milestones")))
	switch ms := v.st.GH.Milestones; {
	case ms == nil:
		b.WriteString(v.noAPI() + "\n")
	case len(ms) == 0:
		b.WriteString(tui.Faint(T("No milestones: the roadmap lives outside GitHub.")) + "\n")
	default:
		closed := 0
		for _, m := range ms {
			total := m.Open + m.ClosedN
			if m.State == "open" {
				due := T("no due date")
				if m.Due != "" {
					due = m.Due[:10]
				}
				fmt.Fprintf(&b, "%s  %s %d/%d  %s\n", tui.Strong(m.Title), tui.Paint(theme.Accent, bar(m.ClosedN, total, 20)), m.ClosedN, total, tui.Faint(due))
				if d := strings.TrimSpace(m.Description); d != "" {
					b.WriteString("  " + tui.Faint(tui.Trunc(strings.ReplaceAll(d, "\n", " "), 200)) + "\n")
				}
			} else if closed < 5 {
				closed++
				fmt.Fprintf(&b, "%s %s  %s\n", tui.Faint("✓"), esc(m.Title), tui.Faint(Tf("closed %s", dateOf(m.Closed))))
			}
		}
	}
	tv.SetText(b.String())
}

func dateOf(s string) string {
	if len(s) >= 10 {
		return s[:10]
	}
	return s
}

func splitArgs(s string) []any {
	if s == "" {
		return nil
	}
	var out []any
	for _, p := range strings.Split(s, "\x00") {
		out = append(out, p)
	}
	return out
}

func (v *ProductView) renderStats() {
	tv := v.texts["stats"]
	tv.SetTitle(" " + T("Health and statistics") + " ")
	h := v.app.hub.Health(v.st)
	var b strings.Builder
	b.WriteString(sectionMarkup(esc(T("Health index")) + ": " + gradeColor(h.Grade) + h.Label() + "[-]"))
	b.WriteString(tui.Faint(Tf("Data coverage %.0f%% of the weight; from the known components %d, corrected for coverage %d.",
		h.Coverage*100, h.Raw, h.Score)) + "\n\n")
	var rows [][]string
	for _, p := range h.Parts {
		score := tui.Faint("-")
		if p.Known {
			c, tone := partCell(p)
			score = tui.Paint(tone.Color(), c)
		}
		rows = append(rows, []string{esc(T(health.Labels[p.Key])), fmt.Sprintf("%.0f", health.Weights[p.Key]), score, esc(tr(p.Note, p.Args))})
	}
	b.WriteString(textTable([]string{N("COMPONENT"), N("WEIGHT"), N("SCORE"), N("BASIS")}, rows))
	if hist := v.app.hub.Store.History(v.st.Product.ID, "health"); len(hist) >= 2 {
		b.WriteString("\n" + tui.Faint(T("Index by day")+": ") + sparkPoints(hist) + "\n")
	}
	if u := v.app.usageLine(v.st.Product.ID); u != "" {
		b.WriteString("\n" + tui.Faint(T("Usage")+": ") + esc(u) + "\n")
	}

	r := v.st.GH.Repo
	b.WriteString(section(N("Repository")))
	if r == nil {
		b.WriteString(v.noAPI() + "\n")
	} else {
		pushed := "-"
		if t := ghapi.Time(r.Pushed); !t.IsZero() {
			pushed = t.Format("2006-01-02") + " (" + age(int(time.Since(t).Hours()/24)) + ")"
		}
		issues := T("yes")
		if !r.HasIssues {
			issues = T("no (the tracker is outside GitHub)")
		}
		kv := [][]string{
			{T("stars"), num(r.Stars)}, {T("forks"), num(r.Forks)}, {T("watchers"), num(r.Watchers)},
			{T("open issues+PRs"), fmt.Sprint(r.OpenIssues)}, {T("language"), esc(r.Language)}, {T("license"), esc(r.LicenseID())},
			{T("created"), dateOf(r.Created)}, {T("last push"), pushed}, {T("default branch"), esc(r.DefaultBranch)},
			{T("issues on GitHub"), issues}, {T("topics"), esc(strings.Join(r.Topics, ", "))},
		}
		b.WriteString(textTable([]string{N("METRIC"), N("VALUE")}, kv))
	}
	if w := v.st.GH.Weekly; w != nil {
		cur, prev, pct, ok := analytics.CommitTrend(w)
		total := 0
		for _, n := range w {
			total += n
		}
		b.WriteString(section(N("Commits in 52 weeks")))
		b.WriteString(tui.Paint(theme.Accent, analytics.Sparkline(w)) + "\n")
		trend := ""
		if ok {
			trend = Tf(", trend %+.0f%%", pct)
		}
		b.WriteString(Tf("total %d · last 12 weeks %d · the 12 before %d", total, cur, prev) + trend + "\n")
	}
	if hist := v.app.hub.Store.History(v.st.Product.ID, "stars"); len(hist) >= 2 {
		b.WriteString(section(N("Stars by day")))
		b.WriteString(sparkPoints(hist) + "  " + tui.Faint(hist[0].Day+" → "+hist[len(hist)-1].Day) + "\n")
	} else {
		b.WriteString("\n" + tui.Faint(T("The history of stars builds up: a snapshot is stored once a day.")) + "\n")
	}
	tv.SetText(b.String())
}

func sparkPoints(hist []store.Point) string {
	vals := make([]int, len(hist))
	lo := hist[0].Value
	for _, p := range hist {
		lo = min(lo, p.Value)
	}
	for i, p := range hist {
		vals[i] = int(p.Value - lo + 1)
	}
	return tui.Paint(theme.Accent, analytics.Sparkline(vals))
}

func (v *ProductView) renderDeps() {
	tv := v.texts["deps"]
	tv.SetTitle(" " + T("Dependencies") + " ")
	g, id := v.app.hub.Graph, v.st.Product.ID
	up, down := g.Upstream(id), g.Downstream(id, false)
	var b strings.Builder
	if len(up) == 0 && len(down) == 0 {
		tv.SetText(tui.Faint(T("No links to other catalog products: add deps to catalog.yaml.")))
		return
	}
	kind := func(k string) string {
		return T(map[string]string{deps.Bundled: N("bundled"), deps.Linked: N("library"), deps.Optional: N("optional"),
			deps.Runtime: N("service alongside"), deps.Platform: N("platform")}[k])
	}
	if len(up) > 0 {
		b.WriteString(section(N("Depends on")))
		b.WriteString(tui.Paint(theme.Accent, strings.Join(g.Tree(id, true), "\n")) + "\n\n")
		var rows [][]string
		for _, hop := range up {
			d := v.app.hub.States[hop.Edge.Dst]
			latest, ago := "-", ""
			if lt := d.Latest(); lt != nil {
				latest, ago = lt.Tag, tui.Paint(ageTone(lt.AgeDays()).Color(), age(lt.AgeDays()))
			}
			rows = append(rows, []string{strings.Repeat("  ", hop.Depth-1) + esc(d.Product.Name), kind(hop.Edge.Kind), esc(latest), ago})
		}
		b.WriteString(textTable([]string{N("DEPENDENCY"), N("KIND"), N("LATEST"), N("AGE")}, rows))
		var fresh []string
		for _, hop := range up {
			for _, r := range v.app.hub.States[hop.Edge.Dst].Stable() {
				if r.AgeDays() <= 30 {
					fresh = append(fresh, "• "+tui.Strong(v.app.hub.States[hop.Edge.Dst].Product.Name)+" "+esc(r.Tag)+" "+tui.Faint(r.Date.Format("2006-01-02")))
				}
			}
		}
		if len(fresh) > 0 {
			b.WriteString(section(N("Fresh releases of the dependencies (30 days)")))
			b.WriteString(strings.Join(fresh, "\n") + "\n")
		}
	}
	if len(down) > 0 {
		b.WriteString(sectionMarkup(esc(Tf("Depend on it (impact radius: %d over strong links)", len(g.Impact(id))))))
		b.WriteString(tui.Paint(theme.Accent, strings.Join(g.Tree(id, false), "\n")) + "\n")
		b.WriteString(tui.Faint(T("A vulnerability here flows into the products linked as bundled, library or platform.")) + "\n")
	}
	tv.SetText(b.String())
}
