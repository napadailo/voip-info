package github

import (
	"encoding/json"
	"testing"

	"github.com/napadailo/voip-info/internal/ghapi"
)

func TestAdvisoryModule(t *testing.T) {
	var as []ghapi.Advisory
	err := json.Unmarshal([]byte(`[
{"ghsa_id":"GHSA-c8g7-475j-fwcc","cve_id":"CVE-2025-59429","severity":"high","published_at":"2025-10-14T00:00:00Z",
 "vulnerabilities":[{"package":{"name":"core"},"vulnerable_version_range":"< 16.0.68.39","patched_versions":"16.0.68.39"},
  {"package":{"name":"core"},"vulnerable_version_range":"< 17.0.18.38","patched_versions":"17.0.18.38"}]},
{"ghsa_id":"GHSA-xxxx","cve_id":null,"severity":"high","published_at":"2026-09-17T00:00:00Z",
 "vulnerabilities":[{"package":{"name":"framework "},"vulnerable_version_range":"< 16.0.50","patched_versions":"16.0.50"}]}]`), &as)
	if err != nil {
		t.Fatal(err)
	}
	vs := AdvisoryVulns("freepbx", "framework", as)
	if vs[0].ID != "CVE-2025-59429" || vs[0].Module != "core" || len(vs[0].Ranges) != 2 || !vs[0].Ranges[0].BranchScoped {
		t.Errorf("a module of FreePBX: %+v", vs[0])
	}
	if vs[1].ID != "GHSA-xxxx" || vs[1].Module != "" {
		t.Errorf("framework is FreePBX itself: %+v", vs[1])
	}
	if vs := AdvisoryVulns("asterisk", "", as); vs[0].Module != "" {
		t.Errorf("a product without modules: %q", vs[0].Module)
	}
}
