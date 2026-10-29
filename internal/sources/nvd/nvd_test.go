package nvd

import (
	"testing"

	"github.com/napadailo/voip-info/internal/catalog"
)

const answer = `{"vulnerabilities":[
{"cve":{"id":"CVE-2026-1","published":"2026-09-28T10:00:00.000","vulnStatus":"Analyzed",
 "descriptions":[{"lang":"en","value":"FreePBX is an open source IP PBX. Prior to 17.0.7, a critical RCE exists in the backup module."}],
 "metrics":{"cvssMetricV31":[{"cvssData":{"baseScore":8.6,"baseSeverity":"HIGH"}}]},
 "configurations":[{"nodes":[{"cpeMatch":[
   {"vulnerable":true,"criteria":"cpe:2.3:a:sangoma:freepbx:*:*:*:*:*:*:*:*","versionStartIncluding":"17.0.0","versionEndExcluding":"17.0.7"},
   {"vulnerable":false,"criteria":"cpe:2.3:o:debian:linux:12:*:*:*:*:*:*:*"}]}]}]}},
{"cve":{"id":"CVE-2026-2","published":"2026-09-01T10:00:00.000","vulnStatus":"Rejected","descriptions":[{"lang":"en","value":"rejected"}]}},
{"cve":{"id":"CVE-2026-3","published":"2026-08-01T10:00:00.000","vulnStatus":"Analyzed",
 "descriptions":[{"lang":"en","value":"freerdp_dsp_decode_opus in FreeRDP overflows."}],"metrics":{}}}
]}`

func TestParse(t *testing.T) {
	p := &catalog.Product{ID: "freepbx", Name: "FreePBX", NVD: "FreePBX", NVDExclude: "(?i)freerdp"}
	vs, err := Parse(p, []byte(answer))
	if err != nil {
		t.Fatal(err)
	}
	if len(vs) != 1 {
		t.Fatalf("rejected and excluded CVEs must be dropped: %d", len(vs))
	}
	v := vs[0]
	if v.Severity != "high" || v.Score != 8.6 || v.Published.Format("2006-01-02") != "2026-09-28" {
		t.Errorf("%+v", v)
	}
	if v.Summary != "Prior to 17.0.7, a critical RCE exists in the backup module." {
		t.Errorf("boilerplate not skipped: %q", v.Summary)
	}
	if len(v.Ranges) != 1 || v.Ranges[0].StartIncl != "17.0.0" || v.Ranges[0].EndExcl != "17.0.7" {
		t.Errorf("own CPE ranges only: %+v", v.Ranges)
	}
}

func TestRangesOfOtherProductsAreDropped(t *testing.T) {
	p := &catalog.Product{ID: "freepbx", Name: "FreePBX", NVD: "FreePBX"}
	other := []cpeMatch{
		{Vulnerable: true, Criteria: "cpe:2.3:a:sangoma:asterisk:*:*:*:*:*:*:*:*", EndExcl: "18.26.3"},
		{Vulnerable: true, Criteria: "cpe:2.3:a:sangoma:certified_asterisk:18.9:*:*:*:*:*:*:*"},
	}
	if r := ranges(p, other); r != nil {
		t.Errorf("Asterisk ranges for FreePBX: %+v", r)
	}
	pj := &catalog.Product{ID: "pjsip", Name: "PJSIP", NVD: "PJSIP"}
	r := ranges(pj, []cpeMatch{{Vulnerable: true, Criteria: "cpe:2.3:a:teluu:pjproject:*:*:*:*:*:*:*:*", EndExcl: "2.15"}})
	if len(r) != 1 || r[0].Fixed != "2.15" {
		t.Errorf("one other CPE product is the product itself: %+v", r)
	}
}

func TestModule(t *testing.T) {
	p := &catalog.Product{ID: "freepbx", Name: "FreePBX", Modules: "framework", Deps: []string{"asterisk:platform"}}
	cpe := func(product string) []cpeMatch {
		return []cpeMatch{{Vulnerable: true, Criteria: "cpe:2.3:a:sangoma:" + product + ":*:*:*:*:*:*:*:*"}}
	}
	for _, c := range []struct {
		desc    string
		matches []cpeMatch
		want    string
	}{
		{"FreePBX 15, 16, and 17 endpoints are vulnerable. This issue has been patched in endpoint versions 15.0.66, 16.0.89, and 17.0.3.", cpe("freepbx"), "endpoint"},
		{"Prior to 17.0.5 and 16.0.17, FreePBX module api (PBX API) is vulnerable to privilege escalation.", nil, "api"},
		{"contactmanager is a module for FreePBX@, which is an open source GUI.", nil, "contactmanager"},
		{"Prior to versions 16.0.4 and 17.0.6, the FreePBX Music on Hold (MoH) module contains a flaw.", nil, "music on hold"},
		{"An issue was discovered in FreePBX core before 3.0.122.43.", nil, "core"},
		{"In Contactmanager 13.x before 13.0.45.3 an XSS exists.", cpe("contactmanager"), "contactmanager"},
		{"Prior to 16.0.47 and 17.0.30, the FreePBX Framework module permits a crafted backup.", cpe("freepbx"), ""},
		{"In FreePBX 15, 16, and 17, connections can cause the uninstall function for certain modules.", cpe("freepbx"), ""},
		{"A vulnerability exists in the safe_asterisk script.", cpe("certified_asterisk"), ""},
		{"It does not verify the type of uploaded (valid FreePBX module) files.", nil, ""},
	} {
		if got := module(p, c.desc, c.matches); got != c.want {
			t.Errorf("%q: %q, want %q", c.desc, got, c.want)
		}
	}
	if got := module(&catalog.Product{ID: "asterisk", Name: "Asterisk"}, "the res_pjsip module crashes", nil); got != "" {
		t.Errorf("a product without modules: %q", got)
	}
}
