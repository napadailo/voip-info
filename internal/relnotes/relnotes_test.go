package relnotes

import (
	"strings"
	"testing"

	"github.com/napadailo/voip-info/internal/model"
)

func rel(notes string) *model.Release { return &model.Release{Tag: "1.0.0", NotesHTML: notes} }

const asteriskNotes = `<h2>Change Log for Release asterisk-20.18.2</h2>
<ul><li>Security Advisories Resolved: 0</li></ul>
<h3>User Notes:</h3>
<h3>Upgrade Notes:</h3>
<ul><li><h4>http.c: Change httpstatus to default disabled and sanitize output.</h4>
To prevent possible security issues, the <code>/httpstatus</code> page served by the internal web server is now disabled by default.</li></ul>
<h3>Developer Notes:</h3>
<ul><li><h4>ARI: Make ARI applications respect live_dangerously.</h4>ARI applications can no longer call "dangerous" dialplan functions.</li></ul>
<h3>Commit List:</h3><ul><li>res_pjsip: Fix crash in MWI parser (CVE-2026-57185)</li></ul>`

func TestUpgradeNotesAndTags(t *testing.T) {
	i := Classify(rel(asteriskNotes))
	if len(i.Upgrade) != 2 || !strings.HasPrefix(i.Upgrade[0], "http.c: Change httpstatus") || !strings.Contains(i.Upgrade[0], "disabled by default") {
		t.Fatalf("upgrade notes %q", i.Upgrade)
	}
	if !strings.HasPrefix(i.Upgrade[1], DevPrefix+"ARI: Make ARI") {
		t.Errorf("developer note %q", i.Upgrade[1])
	}
	if !i.Has("SEC") || !i.Has("BRK") || !i.Has("FIX") || i.Risk() != "high" || len(i.CVEs) != 1 {
		t.Errorf("%+v", i)
	}
}

func TestTemplateHeadingsAndZeroStatsAreNotSignals(t *testing.T) {
	i := Classify(rel(`<ul><li>Security Advisories Resolved: 0</li></ul><h3>Upgrade Notes:</h3><h3>Developer Notes:</h3><ul><li>Fix crash in chan_pjsip</li></ul>`))
	if i.Badge() != "FIX" || i.Risk() != "low" {
		t.Errorf("%+v", i)
	}
}

func TestMinorReleaseTitleIsFix(t *testing.T) {
	i := Classify(rel(`<p>OpenSIPS Minor Release 3.6.9</p>`))
	if i.Badge() != "FIX" {
		t.Errorf("%+v", i)
	}
	if !Classify(rel("")).Empty {
		t.Error("empty notes")
	}
}
