package model

import "time"

// Range is a range of vulnerable versions; empty bounds are open.
type Range struct {
	StartIncl, StartExcl, EndIncl, EndExcl, Exact string
	Fixed                                         string // patched version(s), as the source gives them
	// BranchScoped: the source lists branches by upper bounds only
	// ("<= 20.20.0", "<= 21.12.2"): the range is limited to the major of its bound.
	BranchScoped bool
}

// Vuln is a vulnerability of a product: a CVE or a GitHub advisory.
type Vuln struct {
	ID          string // CVE-… or GHSA-…
	Product     string
	Published   time.Time
	Severity    string  // critical, high, medium, low, ?
	Score       float64 // CVSS, 0 = unknown
	Summary     string
	Description string
	URL         string
	Source      string   // nvd or github
	Aliases     []string // GHSA ids of a CVE and the like
	Ranges      []Range
}

// AgeDays since publication.
func (v *Vuln) AgeDays() int { return int(time.Since(v.Published).Hours() / 24) }

// Serious is critical or high.
func (v *Vuln) Serious() bool { return v.Severity == "critical" || v.Severity == "high" }

// IDs are the id and the aliases.
func (v *Vuln) IDs() []string { return append([]string{v.ID}, v.Aliases...) }

// SeverityRank orders severities, critical first.
func SeverityRank(s string) int {
	switch s {
	case "critical":
		return 0
	case "high":
		return 1
	case "medium", "moderate":
		return 2
	case "low":
		return 3
	}
	return 9
}
