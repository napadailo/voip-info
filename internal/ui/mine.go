package ui

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/napadailo/voip-tui/tui"
	"github.com/napadailo/voip-tui/tui/theme"

	"github.com/napadailo/voip-info/internal/deps"
	"github.com/napadailo/voip-info/internal/inventory"
	"github.com/napadailo/voip-info/internal/model"
	"github.com/napadailo/voip-info/internal/relnotes"
	"github.com/napadailo/voip-info/internal/upgrade"
	"github.com/napadailo/voip-info/internal/versions"
)

func urgencyTone(u int) tui.Tone {
	return []tui.Tone{tui.ToneOK, tui.ToneAccent, tui.ToneWarn, tui.ToneErr}[u]
}

// actionText is the recommendation of an assessment; urgent ones name the target.
func actionText(as *inventory.Assessment) string {
	s := tr(as.Action, as.Args)
	if as.Urgency == inventory.Urgent && as.Target() != "" {
		s += " → " + as.Target()
	}
	if m := as.ModuleKEVNote(); m != "" {
		s += " · " + Tf("KEV in the module %s", m)
	}
	return s
}

// moduleStatus is where a vulnerability of a module stands for my version:
// the vulnerable versions of the module on my branch.
func moduleStatus(v *model.Vuln, version string) string {
	if r, ok := moduleRange(v, version); ok {
		return Tf("module %s: vulnerable %s", v.Module, rangeText(r))
	}
	return Tf("module %s: check its version", v.Module)
}

// moduleRange is the vulnerable range of a module on the branch of version;
// an exact CPE version ("16.0") is FreePBX's, not the module's.
func moduleRange(v *model.Vuln, version string) (model.Range, bool) {
	for _, r := range versions.InBranch(v, version) {
		if r.Exact == "" {
			return r, true
		}
	}
	return model.Range{}, false
}

// updateFixes counts the affecting vulnerabilities the newest release of the branch closes.
func updateFixes(as *inventory.Assessment) (string, tui.Tone) {
	if as.Target() == "" {
		return "", tui.ToneDim
	}
	tgt := versions.String(as.Latest.Version())
	n := 0
	for _, v := range as.Affected {
		if hit, _ := versions.Affects(v, tgt); !hit {
			n++
		}
	}
	if n == 0 {
		return "0", tui.ToneDim
	}
	return fmt.Sprintf("-%d", n), tui.ToneOK
}

// pendingInherited are the inherited high+ vulnerabilities of a year without a fix.
func (a *App) pendingInherited(id string) []deps.Inherited {
	var out []deps.Inherited
	for _, i := range a.hub.Inherited(id, 365, 1) {
		if i.Status == deps.Pending || i.Status == deps.Released {
			out = append(out, i)
		}
	}
	return out
}

func newMineView(a *App) *listView {
	v := newListView(a, "mine", N("My versions"), []string{N("PRODUCT"), N("MY VERSION"), N("NOTE"), N("BRANCH"), N("SUPPORT"),
		N("LATEST IN BRANCH"), N("BEHIND"), "CVE", "HIGH+", "KEV", N("UPDATE FIXES"), N("NO DATA 1Y"), N("CVE DEPS"), N("ACTION")},
		[]Hint{{Key: "enter", Desc: N("details")}, {Key: "u", Desc: N("upgrade path")}, {Key: "c", Desc: N("card")}}, nil)
	v.fill = func() []Row {
		var rows []Row
		for _, as := range a.hub.Assessments() {
			st := a.hub.States[as.ID]
			if c := a.category; c != "" && st.Product.Category != c {
				continue
			}
			cells := []string{st.Product.Name, as.Version, tui.Trunc(as.Note, 16), as.Branch, "?", "-", "0",
				fmt.Sprint(len(as.Affected)), "0", "", "", fmt.Sprint(len(as.Unknown)), "", actionText(as)}
			tones := map[int]tui.Tone{1: tui.ToneAccent, 2: tui.ToneDim, 4: tui.ToneDim, 5: tui.ToneAccent, 6: tui.ToneOK,
				7: tui.ToneOK, 8: tui.ToneOK, 11: tui.ToneDim, 13: urgencyTone(as.Urgency)}
			if as.Support != nil {
				cells[4], tones[4] = supportLabel(as.Support.Status), supportTone(as.Support.Status)
			}
			if as.Latest != nil {
				cells[5] = as.Latest.Tag
			}
			if n := len(as.Behind); n > 0 {
				cells[6], tones[6] = fmt.Sprintf("+%d", n), tui.ToneWarn
			}
			if len(as.Affected) > 0 {
				tones[7] = tui.ToneErr
			}
			if n := len(as.Serious()); n > 0 {
				cells[8], tones[8] = fmt.Sprint(n), tui.ToneErr
			}
			if n := len(as.KEV) + len(as.ModuleKEV); n > 0 {
				cells[9], tones[9] = fmt.Sprint(n), tui.ToneErr
			}
			cells[10], tones[10] = updateFixes(as)
			if n := len(a.pendingInherited(as.ID)); n > 0 {
				cells[12], tones[12] = fmt.Sprint(n), tui.ToneErr
			}
			rows = append(rows, Row{Cells: cells, CellTones: tones, Data: as, Key: as.ID})
		}
		return rows
	}
	v.info = func() string {
		if len(a.hub.Inventory) == 0 {
			return T("empty: :have asterisk 20.18.0 prod")
		}
		return ""
	}
	v.OnEnter(func(r *Row) { a.showAssessment(r.Data.(*inventory.Assessment)) })
	v.Bind('u', func() {
		if r := v.Selected(); r != nil {
			a.openUpgrade(r.Data.(*inventory.Assessment).ID, "", "")
		}
	})
	v.Bind('c', func() {
		if r := v.Selected(); r != nil {
			a.Push(newProductView(a, a.hub.States[r.Data.(*inventory.Assessment).ID], "", "security"))
		}
	})
	v.withCategories()
	v.Rebuild()
	return v
}

// showAssessment opens what follows from my version of a product: the
// summary over the vulnerabilities that affect it, those without version
// data and those of the dependencies without a fix, a row each.
func (a *App) showAssessment(as *inventory.Assessment) {
	st := a.hub.States[as.ID]
	summary := func() string {
		c := tui.NewCard(st.Product.Name + " " + as.Version)
		c.Sub(as.Note)
		c.KV(N("recommendation"), tui.Paint(urgencyTone(as.Urgency).Color(), actionText(as)))
		if len(as.Behind) > 0 {
			c.KV(N("upgrade path"), tui.Faint(":upgrade "+as.ID))
		}
		if b := as.Support; b != nil {
			c.KV(N("branch"), esc(as.Branch+" ("+b.Kind+")")+"  "+tui.Paint(supportTone(b.Status).Color(), supportLabel(b.Status))+
				"  "+tui.Faint(Tf("full support until %s, EOL %s", day(b.FullUntil), day(b.EOL))))
		}
		if len(as.Behind) > 0 {
			var tags, secs []string
			for _, r := range as.Behind {
				tags = append(tags, r.Tag+" ("+r.Date.Format("2006-01-02")+")")
				if relnotes.Classify(r).Has("SEC") {
					secs = append(secs, r.Tag)
				}
			}
			c.KV(N("newer in the branch"), esc(strings.Join(tags, ", ")))
			if len(secs) > 0 {
				c.KV(N("with security fixes"), tui.Paint(theme.Err, strings.Join(secs, ", ")))
			}
		}
		c.KV(N("vulnerabilities"), Tf("affected %d, without version data %d, of dependencies without a fix %d",
			len(as.Affected), len(as.Unknown), len(a.pendingInherited(as.ID))))
		if n := len(as.Modules); n > 0 {
			c.KV(N("in modules"), Tf("%d: a module has its own version, check it on the system", n))
		}
		if m := as.ModuleKEVNote(); m != "" {
			c.KV(N("exploited (KEV)"), tui.Paint(theme.Err, Tf("in the module %s", m)))
		}
		return c.String()
	}
	rows := func() []vulnRow {
		var out []vulnRow
		// exploited vulnerabilities of my modules first, then my version's, then the other modules'
		modules := append([]*model.Vuln(nil), as.Modules...)
		sort.SliceStable(modules, func(i, j int) bool { return a.hub.RiskLess(modules[i], modules[j]) })
		for _, v := range modules {
			if a.hub.KEVOf(v) != nil {
				out = append(out, vulnRow{v, moduleStatus(v, as.Version), tui.ToneErr})
			}
		}
		affected := append([]*model.Vuln(nil), as.Affected...)
		sort.SliceStable(affected, func(i, j int) bool { return a.hub.RiskLess(affected[i], affected[j]) })
		for _, v := range affected {
			fixed := strings.Join(versions.FixedIn(v, as.Version), ", ")
			if fixed == "" {
				fixed = as.Target()
			}
			status := T("affects my version")
			if fixed != "" {
				status = Tf("fixed in %s", tui.Trunc(fixed, 30))
			}
			out = append(out, vulnRow{v, status, tui.ToneErr})
		}
		for _, v := range as.Unknown {
			out = append(out, vulnRow{v, T("no version data: check by hand"), tui.ToneWarn})
		}
		for _, v := range modules {
			if a.hub.KEVOf(v) == nil {
				out = append(out, vulnRow{v, moduleStatus(v, as.Version), tui.ToneWarn})
			}
		}
		for _, i := range a.hub.Inherited(as.ID, 365, 1) {
			status := a.hub.States[i.Upstream].Product.Name + " " + inheritedMark(i.Status) + " " + inheritedLabel(i.Status)
			out = append(out, vulnRow{i.Vuln, status, inheritedTone(i.Status)})
		}
		return out
	}
	a.Push(newVulnListView(a, st.Product.Name+" "+as.Version, summary, rows))
}

// have sets my version of a product: ":have asterisk 20.18.0 prod", ":have asterisk -" removes it.
func (a *App) have(args string) error {
	f := strings.Fields(args)
	if len(f) == 0 {
		a.Replace(newMineView(a))
		return nil
	}
	st, ok := a.hub.States[f[0]]
	if !ok || len(f) < 2 {
		return fmt.Errorf("%s", T("usage: :have <product-id> <version> [note] · :have <id> - removes"))
	}
	ver, note := f[1], strings.Join(f[2:], " ")
	if ver != "-" && versions.Parse(ver) == nil {
		return fmt.Errorf("%s", Tf("not a version: %s", ver))
	}
	if err := a.hub.SetVersion(st.Product.ID, ver, note); err != nil {
		return err
	}
	if ver == "-" {
		a.Flash().Info(Tf("%s removed from my versions", st.Product.Name))
	} else {
		a.Flash().Info(Tf("%s %s added to my versions", st.Product.Name, ver))
	}
	a.onChange("inventory")
	return nil
}

// upgradeCmd: ":upgrade asterisk [target]" from my version, ":upgrade asterisk 20.1.0 20.5.0" from any.
func (a *App) upgradeCmd(args string) error {
	f := strings.Fields(args)
	if len(f) == 0 {
		return fmt.Errorf("%s", T("usage: :upgrade <product-id> [from] [target]"))
	}
	st, ok := a.hub.States[f[0]]
	if !ok {
		return fmt.Errorf("%s", Tf("unknown product %s", f[0]))
	}
	from, target := "", ""
	switch len(f) {
	case 2:
		target = f[1]
	case 3:
		from, target = f[1], f[2]
	}
	if from == "" {
		e, ok := a.hub.Inventory[st.Product.ID]
		if !ok {
			return fmt.Errorf("%s", Tf("set the version first: :have %s <version>", st.Product.ID))
		}
		from = e.Version
	}
	a.openUpgrade(st.Product.ID, from, target)
	return nil
}

// openUpgrade shows the upgrade path; from "" = my version.
func (a *App) openUpgrade(id, from, target string) {
	st := a.hub.States[id]
	if from == "" {
		from = a.hub.Inventory[id].Version
	}
	tv := tui.NewTextViewer(a.App, Tf("upgrade %s", id), "markup")
	tv.SetWrap(true)
	tv.SetLoading()
	load := func() {
		snap, vulns := upgrade.Snapshot(st), a.hub.Vulns(id)
		a.Async(func(ctx context.Context) func() {
			plan := upgrade.Build(ctx, a.hub.Fetcher, snap, from, target, vulns)
			return func() { tv.SetContent(a.planMarkup(plan), "markup") }
		})
	}
	tv.Loader = func() { tv.SetLoading(); load() }
	tv.Keys['b'] = func() { a.OpenURL(st.Product.URL()) }
	tv.Extra = []Hint{{Key: "b", Desc: N("browser")}}
	a.Push(tv)
	load()
}

// planMarkup renders an upgrade plan.
func (a *App) planMarkup(p *upgrade.Plan) string {
	name := a.hub.States[p.ID].Product.Name
	if p.Target == nil {
		c := tui.NewCard(Tf("%s: upgrade path", name))
		c.Text(Tf("Could not find the target release for %s.", p.Current))
		return c.String()
	}
	c := tui.NewCard(name + ": " + p.Current + " → " + p.Target.Tag)
	if len(p.Path) == 0 {
		c.Text(T("You are on the target version or newer: nothing to update in this branch."))
		return c.String()
	}
	risk, riskTone := "?", tui.ToneDim
	switch p.Risk() {
	case "high":
		risk, riskTone = T("high"), tui.ToneErr
	case "medium":
		risk, riskTone = T("medium"), tui.ToneWarn
	case "low":
		risk, riskTone = T("low"), tui.ToneOK
	}
	c.Sub(Tf("%d releases over %d days", len(p.Path), p.Days()))
	c.KV(N("upgrade risk"), tui.Paint(riskTone.Color(), risk))
	c.KV(N("security fixes in"), Tf("%d releases", p.With("SEC")))
	c.KV(N("breaking changes in"), Tf("%d releases", p.With("BRK")))
	c.KV(N("mentions"), Tf("features %d, fixes %d", p.Count("FEAT"), p.Count("FIX")))
	if !p.Complete {
		c.Line(tui.Paint(theme.Warn, T("⚠ the branch history is incomplete: some releases in between may be missing")))
	}
	hi := 0
	for _, v := range p.Closes {
		if v.Serious() {
			hi++
		}
	}
	c.Section(N("Security"), Tf("closes %d CVEs (high/critical: %d), remain %d", len(p.Closes), hi, len(p.Remains)))
	if len(p.Closes) > 0 {
		closes := append([]*model.Vuln(nil), p.Closes...)
		sort.SliceStable(closes, func(i, j int) bool { return a.hub.RiskLess(closes[i], closes[j]) })
		var rows [][]string
		for _, v := range first(closes, 25) {
			e, _ := epssCell(a.hub.EPSSOf(v))
			kev := ""
			if a.hub.KEVOf(v) != nil {
				kev = "🔥"
			}
			rows = append(rows, []string{tui.Paint(theme.Accent, v.ID), tui.Paint(severityColor(v.Severity), v.Severity), e, kev, esc(tui.Trunc(v.Summary, 80))})
		}
		c.Line(strings.ReplaceAll(strings.TrimRight(textTable([]string{"ID", N("SEVERITY"), N("EPSS"), "KEV", N("SUMMARY")}, rows), "\n"), "\n", "\n    "))
	}
	if len(p.Remains) > 0 {
		var ids []string
		for _, v := range first(p.Remains, 10) {
			ids = append(ids, v.ID)
		}
		c.KV(N("not closed"), esc(strings.Join(ids, ", "))+"  "+tui.Faint(T("another branch is needed or there is no fix yet")))
	}
	c.Section(N("Pay attention (breaking / UpgradeNote)"))
	n := 0
	for _, r := range p.Path {
		for _, ln := range p.Notes[r.Tag].Upgrade {
			if n++; n <= 25 {
				c.Line(tui.Paint(theme.Accent, r.Tag) + "  " + esc(tui.Trunc(ln, 220)))
			}
		}
	}
	if n == 0 {
		c.Line(tui.Faint(T("no breaking changes or upgrade notes found in the notes")))
	}
	c.Section(N("Releases on the path"))
	for _, r := range p.Path {
		info := p.Notes[r.Tag]
		line := tui.Paint(theme.Accent, r.Tag) + "  " + tui.Faint(r.Date.Format("2006-01-02"))
		if len(info.Tags) > 0 {
			line += "  " + tui.Paint(badgeTone(info).Color(), info.Badge())
		}
		if len(info.CVEs) > 0 {
			line += "  " + esc("CVE: "+strings.Join(first(info.CVEs, 5), ", "))
		}
		c.Line(line)
	}
	return c.String()
}
