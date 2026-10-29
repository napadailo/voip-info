package ui

import (
	"fmt"
	"sort"
	"strings"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"

	"github.com/napadailo/voip-tui/tui"
	"github.com/napadailo/voip-tui/tui/theme"

	"github.com/napadailo/voip-info/internal/deps"
	"github.com/napadailo/voip-info/internal/model"
	"github.com/napadailo/voip-info/internal/relnotes"
	"github.com/napadailo/voip-info/internal/sources/threat"
	"github.com/napadailo/voip-info/internal/versions"
)

func severityTone(sev string) tui.Tone {
	switch sev {
	case "critical", "high":
		return tui.ToneErr
	case "medium", "moderate":
		return tui.ToneWarn
	case "low":
		return tui.ToneFg
	}
	return tui.ToneDim
}

func severityColor(sev string) tcell.Color { return severityTone(sev).Color() }

// epssCell is the EPSS score as a percentage and its tone.
func epssCell(e threat.EPSS, ok bool) (string, tui.Tone) {
	if !ok {
		return "-", tui.ToneDim
	}
	p := e.Score * 100
	s := fmt.Sprintf("%.1f%%", p)
	if p < 0.1 {
		s = "<0.1%"
	}
	switch {
	case p >= 1:
		return s, tui.ToneErr
	case p >= 0.3:
		return s, tui.ToneWarn
	}
	return s, tui.ToneDim
}

// badgeTone is the tone of the most important tag of the notes.
func badgeTone(i relnotes.Info) tui.Tone {
	switch {
	case i.Empty:
		return tui.ToneDim
	case i.Has("SEC"):
		return tui.ToneErr
	case i.Has("BRK") || i.Has("DEP"):
		return tui.ToneWarn
	case i.Has("FEAT"):
		return tui.ToneAccent
	}
	return tui.ToneDim
}

// tagMarkup colors the release note tags, each its own color of the theme.
func tagMarkup(tag string) string {
	switch tag {
	case "SEC":
		return theme.Tag(theme.Err) + "[::b]SEC[::-][-]"
	case "BRK":
		return theme.Tag(theme.Warn) + "[::b]BRK[::-][-]"
	case "DEP":
		return tui.Paint(theme.Key, tag)
	case "FEAT":
		return tui.Paint(theme.Accent, tag)
	case "FIX":
		return tui.Paint(theme.OK, tag)
	}
	return tview.Escape(tag)
}

// badge is the CHANGES cell of a release: markup (the row marks the column
// as such), "-" when the release has no notes.
func badge(r *model.Release) (string, tui.Tone) {
	if r == nil || r.Commit {
		return "", tui.ToneDim
	}
	i := relnotes.Classify(r)
	if i.Empty {
		return "-", tui.ToneDim
	}
	parts := make([]string, len(i.Tags))
	for k, t := range i.Tags {
		parts[k] = tagMarkup(t)
	}
	return strings.Join(parts, " "), badgeTone(i)
}

// riskCell is the upgrade risk of a release.
func riskCell(r *model.Release) (string, tui.Tone) {
	if r == nil || r.Commit {
		return "", tui.ToneDim
	}
	switch risk := relnotes.Classify(r).Risk(); risk {
	case "high":
		return T("high"), tui.ToneErr
	case "medium":
		return T("medium"), tui.ToneWarn
	case "low":
		return T("low"), tui.ToneOK
	}
	return "?", tui.ToneDim
}

// riskLabels are the upgrade risk levels, translated when shown.
var riskLabels = []string{N("high"), N("medium"), N("low")}

// inheritedLabel is a fix status of an inherited vulnerability.
func inheritedLabel(status string) string {
	return T(map[string]string{deps.Pending: N("waiting for a release"), deps.Released: N("released, not mentioned"),
		deps.Fixed: N("probably fixed"), deps.Separate: N("update separately")}[status])
}

func inheritedMark(status string) string {
	return map[string]string{deps.Pending: "✗", deps.Released: "?", deps.Fixed: "✓", deps.Separate: "↻"}[status]
}

func inheritedTone(status string) tui.Tone {
	switch status {
	case deps.Pending, deps.Released:
		return tui.ToneErr
	case deps.Separate:
		return tui.ToneWarn
	}
	return tui.ToneOK
}

// inheritedDetail explains a status: who decides it and by which release.
func (a *App) inheritedDetail(i deps.Inherited) string {
	name := a.hub.States[i.By].Product.Name
	switch i.Status {
	case deps.Fixed:
		return Tf("%s: mentioned in %s", name, i.Tag)
	case deps.Released:
		return Tf("%s: released %s without a mention", name, i.Tag)
	case deps.Pending:
		return Tf("%s: no release since the publication", name)
	}
	return Tf("update %s on its own", name)
}

// upstreamCell sums up the inherited high+ vulnerabilities without a fix.
func (a *App) upstreamCell(st *model.ProductState) (string, tui.Tone) {
	if len(st.Product.Deps) == 0 {
		return "", tui.ToneDim
	}
	n, pending := 0, false
	for _, i := range a.hub.Inherited(st.Product.ID, 180, 1) {
		if i.Status == deps.Fixed {
			continue
		}
		n++
		pending = pending || i.Status != deps.Separate
	}
	switch {
	case n == 0:
		return "ok", tui.ToneOK
	case pending:
		return fmt.Sprintf("%d ✗", n), tui.ToneErr
	}
	return fmt.Sprintf("%d ↻", n), tui.ToneWarn
}

// cveYearCell counts the vulnerabilities of the last year, toned by the worst.
func (a *App) cveYearCell(st *model.ProductState) (string, tui.Tone) {
	n, worst := 0, "?"
	for _, v := range a.hub.Vulns(st.Product.ID) {
		if v.AgeDays() <= 365 {
			n++
			if model.SeverityRank(v.Severity) < model.SeverityRank(worst) {
				worst = v.Severity
			}
		}
	}
	switch {
	case n > 0:
		return fmt.Sprint(n), severityTone(worst)
	case a.hub.SecurityTracked(st):
		return "0", tui.ToneDim
	}
	return "", tui.ToneDim
}

func newSecurityView(a *App) *listView {
	risk := false
	v := newListView(a, "security", N("Security"), []string{N("PUBLISHED"), N("AGE"), N("SEVERITY"), N("CVSS"), N("EPSS"),
		"KEV", N("PRODUCT"), N("IMPACT"), "ID", N("SUMMARY"), N("SRC")},
		[]Hint{{Key: "enter", Desc: N("details")}, {Key: "b", Desc: N("browser")}, {Key: "R", Desc: N("by risk")}}, nil)
	v.info = func() string {
		if risk {
			return T("by risk")
		}
		return ""
	}
	v.fill = func() []Row {
		var vulns []*model.Vuln
		for _, st := range a.visible() {
			vulns = append(vulns, a.hub.Vulns(st.Product.ID)...)
		}
		if risk {
			sort.SliceStable(vulns, func(i, j int) bool { return a.hub.RiskLess(vulns[i], vulns[j]) })
		} else {
			sort.SliceStable(vulns, func(i, j int) bool { return vulns[i].Published.After(vulns[j].Published) })
		}
		rows := make([]Row, 0, len(vulns))
		for _, vu := range vulns {
			cells := []string{vu.Published.Format("2006-01-02"), age(vu.AgeDays()), vu.Severity, "-", "", "",
				a.hub.States[vu.Product].Product.Name, "", vu.ID, tui.Trunc(vu.Summary, 100), vu.Source}
			tones := map[int]tui.Tone{1: ageTone(vu.AgeDays()), 2: severityTone(vu.Severity), 8: tui.ToneAccent, 10: tui.ToneDim}
			if vu.Score > 0 {
				cells[3] = fmt.Sprintf("%.1f", vu.Score)
			}
			cells[4], tones[4] = epssCell(a.hub.EPSSOf(vu))
			if a.hub.KEVOf(vu) != nil {
				cells[5], tones[5] = T("yes"), tui.ToneErr
			}
			if imp := a.hub.Graph.Impact(vu.Product); len(imp) > 0 {
				cells[7], tones[7] = fmt.Sprintf("+%d", len(imp)), tui.ToneWarn
			}
			srt := map[int]float64{1: float64(vu.AgeDays())}
			if e, ok := a.hub.EPSSOf(vu); ok {
				srt[4] = e.Score
			}
			rows = append(rows, Row{Cells: cells, CellTones: tones, Sort: srt, Data: vu, Key: vu.Product + "\t" + vu.ID})
		}
		return rows
	}
	v.Bind('R', func() { risk = !risk; v.Rebuild(); v.SelectIndex(0) })
	v.OnEnter(func(r *Row) { a.showVuln(r.Data.(*model.Vuln)) })
	v.withCategories()
	v.Rebuild()
	return v
}

// trackedCVEs maps the ids of the known vulnerabilities to their product.
func (a *App) trackedCVEs() map[string]string {
	out := map[string]string{}
	for _, vu := range a.hub.AllVulns() {
		for _, id := range vu.IDs() {
			out[id] = vu.Product
		}
	}
	return out
}

func newKEVView(a *App) *listView {
	v := newListView(a, "kev", N("Exploited (KEV)"), []string{N("ADDED"), N("AGE"), "CVE", N("VENDOR"), N("PRODUCT"),
		N("NAME"), N("RANSOM"), N("EPSS"), N("TRACKED")},
		[]Hint{{Key: "enter", Desc: N("details")}, {Key: "b", Desc: N("browser")}}, nil)
	v.fill = func() []Row {
		tracked := a.trackedCVEs()
		var ks []*threat.KEV
		for _, k := range a.hub.KEV {
			if a.watchOnly && !a.shown(tracked[k.CVE]) {
				continue
			}
			if k.VoIP() || tracked[k.CVE] != "" {
				ks = append(ks, k)
			}
		}
		sort.Slice(ks, func(i, j int) bool {
			if !ks[i].Added.Equal(ks[j].Added) {
				return ks[i].Added.After(ks[j].Added)
			}
			return ks[i].CVE > ks[j].CVE
		})
		rows := make([]Row, 0, len(ks))
		for _, k := range ks {
			cells := []string{k.Added.Format("2006-01-02"), age(k.AgeDays()), k.CVE, tui.Trunc(k.Vendor, 20), tui.Trunc(k.Product, 24),
				tui.Trunc(k.Name, 60), "", "", ""}
			tones := map[int]tui.Tone{1: ageTone(k.AgeDays()), 2: tui.ToneAccent, 5: tui.ToneDim, 8: tui.ToneOK}
			if k.Ransomware {
				cells[6], tones[6] = T("yes"), tui.ToneErr
			}
			e, ok := a.hub.EPSS[k.CVE]
			cells[7], tones[7] = epssCell(e, ok)
			if id := tracked[k.CVE]; id != "" {
				cells[8] = a.hub.States[id].Product.Name
			}
			srt := map[int]float64{1: float64(k.AgeDays())}
			if ok {
				srt[7] = e.Score
			}
			rows = append(rows, Row{Cells: cells, CellTones: tones, Sort: srt, Data: k, Key: k.CVE})
		}
		return rows
	}
	v.OnEnter(func(r *Row) { a.showKEV(r.Data.(*threat.KEV)) })
	v.Rebuild()
	return v
}

func nvdURL(cve string) string { return "https://nvd.nist.gov/vuln/detail/" + cve }

// rangeText is a range of vulnerable versions: ">= 20.0.0, < 20.5.1".
func rangeText(r model.Range) string {
	if r.Exact != "" {
		return "= " + r.Exact
	}
	var parts []string
	for _, b := range []struct{ op, v string }{{">=", r.StartIncl}, {">", r.StartExcl}, {"<=", r.EndIncl}, {"<", r.EndExcl}} {
		if b.v != "" {
			parts = append(parts, b.op+" "+b.v)
		}
	}
	if len(parts) == 0 {
		return "?"
	}
	return strings.Join(parts, ", ")
}

// threatLines are the KEV and EPSS lines of a vulnerability card.
func (a *App) threatLines(c *tui.Card, vu *model.Vuln) {
	if k := a.hub.KEVOf(vu); k != nil {
		s := Tf("in CISA KEV since %s: exploited in attacks", k.Added.Format("2006-01-02"))
		if k.Ransomware {
			s += ", " + T("used by ransomware")
		}
		c.KV(N("KEV"), tui.Paint(theme.Err, "🔥 "+s))
	}
	if e, ok := a.hub.EPSSOf(vu); ok {
		c.KV(N("EPSS"), Tf("%.2f%% chance of exploitation in 30 days (percentile %.0f)", e.Score*100, e.Percentile*100))
	}
}

// showVuln opens the card of a vulnerability.
func (a *App) showVuln(vu *model.Vuln) {
	st := a.hub.States[vu.Product]
	title := vu.ID + " · " + st.Product.Name
	if vu.Module != "" {
		title += " · " + Tf("module %s", vu.Module)
	}
	c := tui.NewCard(title)
	score := "-"
	if vu.Score > 0 {
		score = fmt.Sprintf("%.1f", vu.Score)
	}
	c.Sub(vu.Severity, "CVSS "+score, vu.Published.Format("2006-01-02"), vu.Source)
	c.Section(N("Threat"))
	c.KV(N("severity"), tui.Paint(severityColor(vu.Severity), vu.Severity))
	a.threatLines(c, vu)
	if len(vu.Aliases) > 0 {
		c.KV(N("aliases"), esc(strings.Join(vu.Aliases, ", ")))
	}
	if len(vu.Ranges) > 0 {
		// an exact CPE version ("16.0") is FreePBX's, not the module's
		exact := true
		for _, r := range vu.Ranges {
			exact = exact && r.Exact != ""
		}
		if vu.Module != "" && !exact {
			c.Section(Tf("Affected versions of the %s module", vu.Module))
		} else {
			c.Section(N("Affected versions"))
		}
		for _, r := range vu.Ranges {
			fixed := ""
			if r.Fixed != "" {
				fixed = "  → " + tui.Paint(theme.OK, T("fixed")+": "+r.Fixed)
			}
			c.Line(esc(rangeText(r)) + fixed)
		}
	}
	if imp := a.hub.Graph.Downstream(vu.Product, true); len(imp) > 0 {
		var names []string
		for _, h := range imp {
			names = append(names, fmt.Sprintf("%s (%s)", a.hub.States[h.Edge.Src].Product.Name, h.Edge.Kind))
		}
		c.Section(N("Inherited through dependencies"))
		c.Line(esc(strings.Join(names, ", ")))
	}
	if mine := a.hub.Assess(vu.Product); mine != nil {
		c.Section(N("My version"))
		if vu.Module != "" {
			r, ok := moduleRange(vu, mine.Version)
			switch {
			case ok:
				c.KVRaw(mine.Version, tui.Paint(theme.Warn, Tf("the %s module of this branch is vulnerable %s: check its version on the system", vu.Module, rangeText(r))))
			case len(vu.Ranges) > 0 && len(versions.InBranch(vu, mine.Version)) == 0:
				c.KVRaw(mine.Version, tui.Paint(theme.OK, Tf("the %s module of this branch is not affected", vu.Module)))
			default:
				c.KVRaw(mine.Version, tui.Paint(theme.Warn, Tf("not comparable: the versions are of the %s module, check it on the system", vu.Module)))
			}
		}
		for _, x := range mine.Affected {
			if x == vu {
				c.KVRaw(mine.Version, tui.Paint(theme.Err, T("affected")))
			}
		}
	}
	desc := vu.Description
	if desc == "" {
		desc = vu.Summary
	}
	c.Text(desc)
	c.Line(tui.Faint(vu.URL))
	a.showCard(vu.ID, c.String(), vu.URL)
}

// showKEV opens the card of a CISA KEV entry.
func (a *App) showKEV(k *threat.KEV) {
	c := tui.NewCard("🔥 " + k.CVE + " · " + k.Vendor + " " + k.Product)
	c.Sub(k.Name)
	c.KV(N("added to KEV"), k.Added.Format("2006-01-02"))
	c.KV(N("due date (US agencies)"), esc(k.Due))
	if k.Ransomware {
		c.KV(N("ransomware"), tui.Paint(theme.Err, T("used by ransomware")))
	}
	if e, ok := a.hub.EPSS[k.CVE]; ok {
		c.KV(N("EPSS"), fmt.Sprintf("%.1f%% (p%.0f)", e.Score*100, e.Percentile*100))
	}
	if id := a.trackedCVEs()[k.CVE]; id != "" {
		c.KV(N("tracked"), tui.Paint(theme.OK, a.hub.States[id].Product.Name))
	}
	c.Text(k.Description)
	c.Section(N("Required action"))
	c.Text(k.Action)
	c.Line(tui.Faint(nvdURL(k.CVE)))
	a.showCard(k.CVE, c.String(), nvdURL(k.CVE))
}

// showCard pushes a text view of markup; b opens url.
func (a *App) showCard(name, markup, url string) {
	tv := tui.NewTextViewer(a.App, name, "markup")
	tv.SetWrap(true)
	tv.SetContent(markup, "markup")
	if url != "" {
		tv.Keys['b'] = func() { a.OpenURL(url) }
		tv.Extra = []Hint{{Key: "b", Desc: N("browser")}}
	}
	a.Push(tv)
}
