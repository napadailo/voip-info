package hub

import (
	"context"
	"sort"
	"time"

	"github.com/napadailo/voip-info/internal/deps"
	"github.com/napadailo/voip-info/internal/health"
	"github.com/napadailo/voip-info/internal/model"
	"github.com/napadailo/voip-info/internal/sources/github"
	"github.com/napadailo/voip-info/internal/sources/nvd"
	"github.com/napadailo/voip-info/internal/sources/threat"
)

// LoadSecurity reads the CVEs of every product with an NVD keyword, one at a
// time (NVD allows 5 requests in 30 seconds without a key), then KEV and EPSS.
func (h *Hub) LoadSecurity(ctx context.Context, force bool) {
	var todo []*model.ProductState
	for _, p := range h.Products {
		if p.NVD != "" {
			todo = append(todo, h.States[p.ID])
		}
	}
	h.Apply(func() { h.NVDDone, h.NVDTotal = 0, len(todo) })
	c := nvd.NewClient(h.Fetcher)
	for _, st := range todo {
		if ctx.Err() != nil {
			return
		}
		vs, err := c.Fetch(ctx, st.Product, force)
		h.Apply(func() {
			if err == nil {
				st.NVD = vs
				if st.NVD == nil {
					st.NVD = []*model.Vuln{}
				}
				delete(h.vulns, st.Product.ID)
			}
			st.NVDErr = err
			h.NVDDone++
			if h.NVDDone%4 == 0 || h.NVDDone == h.NVDTotal {
				h.OnChange("security")
			}
		})
	}
	h.LoadThreat(ctx, force)
}

// LoadThreat reads CISA KEV and the EPSS scores of the known CVEs.
func (h *Hub) LoadThreat(ctx context.Context, force bool) {
	kev, err := threat.FetchKEV(ctx, h.Fetcher, force)
	var ids []string
	h.Apply(func() {
		if kev != nil {
			h.KEV = kev
		}
		h.ThreatErr = err
		for _, v := range h.AllVulns() {
			ids = append(ids, v.IDs()...)
		}
	})
	epss, err2 := threat.FetchEPSS(ctx, h.Fetcher, ids, force)
	h.Apply(func() {
		if len(epss) > 0 {
			h.EPSS = epss
		}
		if err2 != nil && h.ThreatErr == nil {
			h.ThreatErr = err2
		}
		h.SnapshotHealth()
	})
	h.changed("security")
}

// Releases of a product, newest first (deps.Source).
func (h *Hub) Releases(id string) []*model.Release {
	if st := h.States[id]; st != nil {
		return st.Releases
	}
	return nil
}

// Vulns of a product: NVD and GitHub advisories merged, one entry per id,
// newest first. GitHub wins over NVD: its ranges name the patched versions.
func (h *Hub) Vulns(id string) []*model.Vuln {
	if v, ok := h.vulns[id]; ok {
		return v
	}
	st := h.States[id]
	if st == nil {
		return nil
	}
	byID := map[string]*model.Vuln{}
	var order []string
	add := func(v *model.Vuln) {
		cur, ok := byID[v.ID]
		if !ok {
			order = append(order, v.ID)
			byID[v.ID] = v
			return
		}
		if cur.Source == "nvd" && v.Source == "github" {
			if len(v.Ranges) == 0 {
				cp := *v
				cp.Ranges = cur.Ranges
				v = &cp
			}
			byID[v.ID] = v
		}
	}
	for _, v := range st.NVD {
		add(v)
	}
	for _, v := range github.AdvisoryVulns(id, st.GH.Advisories) {
		add(v)
	}
	out := make([]*model.Vuln, 0, len(order))
	for _, k := range order {
		out = append(out, byID[k])
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Published.After(out[j].Published) })
	if h.vulns == nil {
		h.vulns = map[string][]*model.Vuln{}
	}
	h.vulns[id] = out
	return out
}

// AllVulns of the catalog.
func (h *Hub) AllVulns() []*model.Vuln {
	var out []*model.Vuln
	for _, p := range h.Products {
		out = append(out, h.Vulns(p.ID)...)
	}
	return out
}

// SecurityTracked: some source of vulnerabilities covers the product.
func (h *Hub) SecurityTracked(st *model.ProductState) bool {
	return st.NVD != nil || st.GH.Advisories != nil
}

// KEVOf is the CISA KEV entry of a vulnerability or of one of its aliases.
func (h *Hub) KEVOf(v *model.Vuln) *threat.KEV {
	for _, id := range v.IDs() {
		if k := h.KEV[id]; k != nil {
			return k
		}
	}
	return nil
}

// EPSSOf is the EPSS score of a vulnerability or of one of its aliases.
func (h *Hub) EPSSOf(v *model.Vuln) (threat.EPSS, bool) {
	for _, id := range v.IDs() {
		if e, ok := h.EPSS[id]; ok {
			return e, true
		}
	}
	return threat.EPSS{}, false
}

// RiskLess orders by the real risk: exploited (KEV) first, then EPSS, then CVSS.
func (h *Hub) RiskLess(a, b *model.Vuln) bool {
	ka, kb := h.KEVOf(a) != nil, h.KEVOf(b) != nil
	if ka != kb {
		return ka
	}
	ea, _ := h.EPSSOf(a)
	eb, _ := h.EPSSOf(b)
	if ea.Score != eb.Score {
		return ea.Score > eb.Score
	}
	if a.Score != b.Score {
		return a.Score > b.Score
	}
	return a.Published.After(b.Published)
}

// Inherited are the vulnerabilities of the dependencies of a product.
func (h *Hub) Inherited(id string, days, maxRank int) []deps.Inherited {
	return h.Graph.InheritedVulns(h, id, days, maxRank)
}

// security sums up the vulnerabilities of a product for its health index.
func (h *Hub) security(st *model.ProductState) health.Security {
	s := health.Security{Tracked: h.SecurityTracked(st)}
	for _, v := range h.Vulns(st.Product.ID) {
		age := time.Since(v.Published)
		if age <= 365*24*time.Hour && v.Serious() {
			s.SeriousYear++
			if v.Severity == "critical" {
				s.CriticalYear++
			}
		}
		if age <= 730*24*time.Hour && h.KEVOf(v) != nil {
			s.Exploited++
		}
	}
	for _, i := range h.Inherited(st.Product.ID, 365, 1) {
		if i.Status == deps.Pending || i.Status == deps.Released {
			s.PendingInherited++
		}
	}
	return s
}
