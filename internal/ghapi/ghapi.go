// Package ghapi holds the GitHub REST API answers voip-info reads.
package ghapi

import "time"

// Repo is the part of GET /repos/{repo} voip-info uses.
type Repo struct {
	Stars         int      `json:"stargazers_count"`
	Forks         int      `json:"forks_count"`
	Watchers      int      `json:"subscribers_count"`
	OpenIssues    int      `json:"open_issues_count"` // issues + PRs
	Language      string   `json:"language"`
	Created       string   `json:"created_at"`
	Pushed        string   `json:"pushed_at"`
	DefaultBranch string   `json:"default_branch"`
	HasIssues     bool     `json:"has_issues"`
	Archived      bool     `json:"archived"`
	Topics        []string `json:"topics"`
	License       *struct {
		SPDX string `json:"spdx_id"`
	} `json:"license"`
}

// LicenseID is the SPDX id of the license, "" when none.
func (r *Repo) LicenseID() string {
	if r.License == nil {
		return ""
	}
	return r.License.SPDX
}

// Issue is an issue or a pull request (pull_request set).
type Issue struct {
	Number   int    `json:"number"`
	Title    string `json:"title"`
	State    string `json:"state"`
	URL      string `json:"html_url"`
	Comments int    `json:"comments"`
	Created  string `json:"created_at"`
	Closed   string `json:"closed_at"`
	Updated  string `json:"updated_at"`
	User     struct {
		Login string `json:"login"`
	} `json:"user"`
	PR *struct {
		Merged string `json:"merged_at"`
	} `json:"pull_request"`
	Body   string `json:"body"` // Markdown
	Labels []struct {
		Name string `json:"name"`
	} `json:"labels"`
}

// Milestone is a GitHub milestone.
type Milestone struct {
	Title       string `json:"title"`
	State       string `json:"state"`
	URL         string `json:"html_url"`
	Description string `json:"description"`
	Open        int    `json:"open_issues"`
	ClosedN     int    `json:"closed_issues"`
	Due         string `json:"due_on"`
	Closed      string `json:"closed_at"`
}

// Advisory is a published GitHub security advisory.
type Advisory struct {
	GHSA        string `json:"ghsa_id"`
	CVE         string `json:"cve_id"`
	Summary     string `json:"summary"`
	Description string `json:"description"`
	Severity    string `json:"severity"`
	URL         string `json:"html_url"`
	Published   string `json:"published_at"`
	CVSS        *struct {
		Score float64 `json:"score"`
	} `json:"cvss"`
	Vulnerabilities []struct {
		Package struct {
			Name string `json:"name"`
		} `json:"package"`
		Range   string `json:"vulnerable_version_range"`
		Patched string `json:"patched_versions"`
	} `json:"vulnerabilities"`
}

// Time parses a GitHub timestamp; zero when empty.
func Time(s string) time.Time {
	t, _ := time.Parse(time.RFC3339, s)
	return t
}
