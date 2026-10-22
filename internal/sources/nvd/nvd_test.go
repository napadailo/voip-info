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
