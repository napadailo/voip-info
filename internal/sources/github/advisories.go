package github

import (
	"regexp"
	"strings"

	"github.com/napadailo/voip-info/internal/ghapi"
	"github.com/napadailo/voip-info/internal/model"
)

var cmpRe = regexp.MustCompile(`(<=|>=|<|>|=)\s*([\w.\-]+)`)

// AdvisoryRanges reads vulnerable_version_range: ">= 20.0.0, < 20.20.1" or "<= 20.20.0".
func AdvisoryRanges(a ghapi.Advisory) []model.Range {
	var out []model.Range
	for _, v := range a.Vulnerabilities {
		r := model.Range{Fixed: v.Patched}
		set := false
		for _, m := range cmpRe.FindAllStringSubmatch(v.Range, -1) {
			set = true
			switch m[1] {
			case ">=":
				r.StartIncl = m[2]
			case ">":
				r.StartExcl = m[2]
			case "<=":
				r.EndIncl = m[2]
			case "<":
				r.EndExcl = m[2]
			case "=":
				r.Exact = m[2]
			}
		}
		if set {
			out = append(out, r)
		}
	}
	// Asterisk and others list the branches by upper bounds only: such a
	// range is limited to its own major branch
	if len(out) > 1 {
		for i := range out {
			if out[i].StartIncl == "" && out[i].StartExcl == "" && (out[i].EndIncl != "" || out[i].EndExcl != "") {
				out[i].BranchScoped = true
			}
		}
	}
	return out
}

// AdvisoryVulns turns advisories into vulnerabilities of product id.
func AdvisoryVulns(id string, as []ghapi.Advisory) []*model.Vuln {
	var out []*model.Vuln
	for _, a := range as {
		pub := ghapi.Time(a.Published)
		if pub.IsZero() {
			continue
		}
		vid := a.CVE
		if vid == "" {
			vid = a.GHSA
		}
		score := 0.0
		if a.CVSS != nil {
			score = a.CVSS.Score
		}
		summary := a.Summary
		if r := []rune(summary); len(r) > 160 {
			summary = string(r[:160])
		}
		sev := strings.ToLower(a.Severity)
		if sev == "" {
			sev = "?"
		}
		out = append(out, &model.Vuln{ID: vid, Product: id, Published: pub, Severity: sev, Score: score, Summary: summary,
			Description: a.Description, URL: a.URL, Source: "github", Aliases: []string{a.GHSA}, Ranges: AdvisoryRanges(a)})
	}
	return out
}
