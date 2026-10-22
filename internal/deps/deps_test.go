package deps

import (
	"strings"
	"testing"
	"time"

	"github.com/napadailo/voip-info/internal/catalog"
	"github.com/napadailo/voip-info/internal/model"
)

func products() []*catalog.Product {
	return []*catalog.Product{
		{ID: "pjsip", Name: "PJSIP"},
		{ID: "asterisk", Name: "Asterisk", Deps: []string{"pjsip:bundled"}},
		{ID: "freepbx", Name: "FreePBX", Deps: []string{"asterisk:platform"}},
		{ID: "fonoster", Name: "Fonoster", Deps: []string{"asterisk:runtime", "outside:linked"}},
	}
}

func TestImpactFollowsStrongEdgesOnly(t *testing.T) {
	g := New(products())
	if got := strings.Join(g.Impact("pjsip"), ","); got != "asterisk,freepbx" {
		t.Errorf("impact %s: runtime consumers do not inherit", got)
	}
	up := g.Upstream("freepbx")
	if len(up) != 2 || up[0].Edge.Dst != "asterisk" || up[1].Edge.Dst != "pjsip" || up[1].Depth != 2 {
		t.Errorf("upstream %+v", up)
	}
	if len(g.Edges) != 3 {
		t.Errorf("edges outside the catalog are dropped: %+v", g.Edges)
	}
}

func TestTree(t *testing.T) {
	lines := New(products()).Tree("pjsip", false)
	want := []string{"PJSIP", "└─ Asterisk (bundled)", "   ├─ FreePBX (platform)", "   └─ Fonoster (runtime)"}
	if strings.Join(lines, "\n") != strings.Join(want, "\n") {
		t.Errorf("got\n%s", strings.Join(lines, "\n"))
	}
}

type fakeSource struct {
	rel   map[string][]*model.Release
	vulns map[string][]*model.Vuln
}

func (f fakeSource) Releases(id string) []*model.Release { return f.rel[id] }
func (f fakeSource) Vulns(id string) []*model.Vuln       { return f.vulns[id] }

func TestInheritedStatuses(t *testing.T) {
	g := New(products())
	now := time.Now()
	day := func(d int) time.Time { return now.AddDate(0, 0, -d) }
	src := fakeSource{
		rel: map[string][]*model.Release{
			"asterisk": {
				{Tag: "20.2.0", Date: day(2), NotesHTML: "<p>Bump bundled pjproject to 2.15</p>"},
				{Tag: "20.1.0", Date: day(8), NotesHTML: "<p>misc</p>"},
			},
		},
		vulns: map[string][]*model.Vuln{
			"pjsip":    {{ID: "CVE-2026-1", Severity: "high", Published: day(10)}, {ID: "CVE-2026-2", Severity: "low", Published: day(10)}},
			"asterisk": {{ID: "CVE-2026-3", Severity: "critical", Published: day(1)}},
		},
	}
	inh := g.InheritedVulns(src, "asterisk", 180, 2)
	if len(inh) != 1 || inh[0].Status != Fixed || inh[0].Tag != "20.2.0" {
		t.Fatalf("asterisk %+v", inh)
	}
	// FreePBX is built on Asterisk: the status of the PJSIP fix is Asterisk's,
	// and the Asterisk CVE itself is a separate update
	inh = g.InheritedVulns(src, "freepbx", 180, 2)
	if len(inh) != 2 || inh[0].Status != Separate || inh[1].Status != Fixed || inh[1].By != "asterisk" {
		t.Fatalf("freepbx %+v", inh)
	}
	if s := Summary(inh); s[Fixed] != 1 || s[Separate] != 1 {
		t.Errorf("summary %v", s)
	}
	// Fonoster runs next to Asterisk: PJSIP is two weak hops away and does not count
	inh = g.InheritedVulns(src, "fonoster", 180, 2)
	if len(inh) != 1 || inh[0].Vuln.ID != "CVE-2026-3" {
		t.Errorf("fonoster %+v", inh)
	}
}
