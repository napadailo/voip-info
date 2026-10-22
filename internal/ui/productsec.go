package ui

import (
	"fmt"
	"strings"

	"github.com/rivo/tview"

	"github.com/napadailo/voip-tui/tui"
	"github.com/napadailo/voip-tui/tui/theme"
)

// newSecurityPane is the Security tab: a summary over the list of the
// vulnerabilities; enter opens one, b the advisory.
func (v *ProductView) newSecurityPane() tview.Primitive {
	a := v.app
	v.secHead = tview.NewTextView().SetDynamicColors(true).SetWrap(true).SetWordWrap(true)
	v.secHead.SetBorder(true).SetBorderPadding(0, 0, 1, 1)
	v.secT = tui.NewTableView(a.App, N("Vulnerabilities"), []string{N("PUBLISHED"), N("SEVERITY"), "CVSS", N("EPSS"), "KEV", "ID", N("FROM"), N("SUMMARY")})
	v.secT.OnEnter(func(r *Row) { a.showVuln(r.Data.(vulnRow).vu) })
	v.secT.Bind('b', func() {
		if r := v.secT.Selected(); r != nil {
			a.OpenURL(r.Data.(vulnRow).vu.URL)
		}
	})
	v.secT.SetInputCapture(v.chain(v.secT.HandleKey))
	v.secFlex = tview.NewFlex().SetDirection(tview.FlexRow).AddItem(v.secHead, 4, 0, false).AddItem(v.secT, 0, 1, true)
	return v.secFlex
}

// renderSecurity fills the Security tab.
func (v *ProductView) renderSecurity() {
	v.secHead.SetTitle(" " + T("Security") + " ")
	text := strings.TrimRight(v.securitySummary(), "\n")
	v.secHead.SetText(text)
	v.secFlex.ResizeItem(v.secHead, min(strings.Count(text, "\n")+3, 12), 0)

	h, id := v.app.hub, v.st.Product.ID
	var list []vulnRow
	for _, vu := range h.Vulns(id) {
		list = append(list, vulnRow{vu, T("own"), tui.ToneDim})
	}
	for _, i := range h.Inherited(id, 180, 2) {
		list = append(list, vulnRow{i.Vuln, h.States[i.Upstream].Product.Name + " " + inheritedMark(i.Status) + " " + inheritedLabel(i.Status),
			inheritedTone(i.Status)})
	}
	v.secT.SetRows(v.app.vulnRows(list))
}

// securitySummary is the head of the Security tab: counts, sources, my
// version and whom a vulnerability here reaches.
func (v *ProductView) securitySummary() string {
	h, st := v.app.hub, v.st
	id := st.Product.ID
	var b strings.Builder
	vulns := h.Vulns(id)
	year, serious, kev := 0, 0, 0
	for _, vu := range vulns {
		if vu.AgeDays() <= 365 {
			year++
			if vu.Serious() {
				serious++
			}
		}
		if h.KEVOf(vu) != nil {
			kev++
		}
	}
	var src []string
	switch {
	case st.Product.NVD == "":
	case st.NVDErr != nil:
		src = append(src, tui.Paint(theme.Err, "NVD: "+st.NVDErr.Error()))
	case st.NVD == nil:
		src = append(src, tui.Faint(Tf("NVD: loading %d/%d…", h.NVDDone, h.NVDTotal)))
	default:
		src = append(src, "NVD: "+fmt.Sprint(len(st.NVD)))
	}
	if st.GH.Advisories != nil {
		src = append(src, "GitHub advisories: "+fmt.Sprint(len(st.GH.Advisories)))
	}
	if len(src) == 0 {
		src = append(src, tui.Faint(T("no source of vulnerabilities for this product")))
	}
	b.WriteString(Tf("Vulnerabilities: %d, in the last year %d (high+ %d), exploited (KEV) %d", len(vulns), year, serious, kev))
	b.WriteString("   " + tui.Faint(T("sources")+": ") + strings.Join(src, " · ") + "\n")
	if as := h.Assess(id); as != nil {
		b.WriteString(tui.Faint(Tf("My version %s", as.Version)+": ") + tui.Paint(urgencyTone(as.Urgency).Color(), actionText(as)) +
			"  " + tui.Faint(Tf("affected %d, high+ %d, KEV %d · details: :mine, path: :upgrade %s",
			len(as.Affected), len(as.Serious()), len(as.KEV), id)) + "\n")
	}
	if imp := h.Graph.Impact(id); len(imp) > 0 {
		var names []string
		for _, x := range imp {
			names = append(names, h.States[x].Product.Name)
		}
		b.WriteString(tui.Faint(Tf("A vulnerability here flows into %d products", len(imp))+": ") + esc(strings.Join(names, ", ")) + "\n")
	}
	if len(st.Product.Deps) > 0 {
		b.WriteString(tui.Faint(T("FROM: a bundled or linked dependency is fixed by a release of this product; a platform or a service alongside is updated on its own.")) + "\n")
	}
	return b.String()
}
