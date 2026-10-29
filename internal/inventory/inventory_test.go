package inventory

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/napadailo/voip-info/internal/catalog"
	"github.com/napadailo/voip-info/internal/eol"
	"github.com/napadailo/voip-info/internal/model"
)

func TestLoadImportsLegacyAndSaves(t *testing.T) {
	old := t.TempDir()
	t.Setenv("VOIPINFO_HOME", old)
	_ = os.WriteFile(filepath.Join(old, "inventory.toml"), []byte("# x\n\n[asterisk]\nversion = \"20.18.0\"\nnote = \"lab\"\n\n[empty]\nnote = \"x\"\n"), 0o644)
	dir := t.TempDir()
	inv, err := Load(dir)
	if err != nil || len(inv) != 1 || inv["asterisk"] != (Entry{"20.18.0", "lab"}) {
		t.Fatalf("%v %v", inv, err)
	}
	// imported once: the yaml now wins
	inv["kamailio"] = Entry{Version: "6.0.1"}
	if err := Save(dir, inv); err != nil {
		t.Fatal(err)
	}
	inv, _ = Load(dir)
	if len(inv) != 2 || inv.IDs()[1] != "kamailio" {
		t.Errorf("%v", inv)
	}
}

func TestAssess(t *testing.T) {
	now := time.Now()
	rel := func(tag string, d int) *model.Release {
		return &model.Release{Product: "kamailio", Tag: tag, Date: now.AddDate(0, 0, -d)}
	}
	st := &model.ProductState{Product: &catalog.Product{ID: "kamailio"},
		Releases: []*model.Release{rel("6.1.0", 1), rel("6.0.3", 5), rel("6.0.2", 30), rel("5.8.7", 40)}}
	high := &model.Vuln{ID: "CVE-1", Severity: "high", Published: now, Ranges: []model.Range{{StartIncl: "6.0.0", EndExcl: "6.0.3"}}}
	nodata := &model.Vuln{ID: "CVE-2", Severity: "medium", Published: now}
	a := Assess(st, "6.0.2", "", Context{Vulns: []*model.Vuln{high, nodata}})
	if a.Branch != "6.0" || a.Latest.Tag != "6.0.3" || len(a.Behind) != 1 || a.Mine == nil {
		t.Fatalf("%+v", a)
	}
	if len(a.Affected) != 1 || len(a.Unknown) != 1 || a.Urgency != Urgent || a.Target() != "6.0.3" {
		t.Errorf("%+v", a)
	}
	a = Assess(st, "6.0.2", "", Context{Vulns: []*model.Vuln{high}, KEV: func(*model.Vuln) bool { return true }})
	if len(a.KEV) != 1 || a.Action != "URGENT: %d exploited vulnerabilities (KEV)" {
		t.Errorf("%+v", a)
	}
	mod := &model.Vuln{ID: "CVE-3", Severity: "critical", Published: now, Module: "endpoint",
		Ranges: []model.Range{{StartIncl: "6.0", EndExcl: "6.0.9"}}}
	a = Assess(st, "6.0.3", "", Context{Vulns: []*model.Vuln{mod}, KEV: func(*model.Vuln) bool { return true }})
	if len(a.Affected) != 0 || len(a.KEV) != 0 || len(a.Modules) != 1 || a.Action != "check the modules: %d exploited (KEV)" {
		t.Errorf("the versions of a module are not mine: %+v", a)
	}
	a = Assess(st, "6.0.3", "", Context{Support: []eol.Branch{{Branch: "6.0", Status: "security"}}})
	if a.Urgency != Info || a.Action != "branch is security-only" {
		t.Errorf("%+v", a)
	}
	a = Assess(st, "5.8.7", "", Context{Support: []eol.Branch{{Branch: "5.8", Status: "eol"}}})
	if a.Urgency != Update {
		t.Errorf("%+v", a)
	}
}
