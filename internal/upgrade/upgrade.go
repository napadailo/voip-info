// Package upgrade plans an update within a branch: the releases between my
// version and the target, what they change and which vulnerabilities the
// update closes. The branch history comes from the feed, then from older
// tags.atom pages (no API limits); the notes of each release on the path
// come from its GitHub page when the feed has none.
package upgrade

import (
	"context"
	"html"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/napadailo/voip-info/internal/fetch"
	"github.com/napadailo/voip-info/internal/model"
	"github.com/napadailo/voip-info/internal/relnotes"
	"github.com/napadailo/voip-info/internal/sources/github"
	"github.com/napadailo/voip-info/internal/versions"
)

var (
	mdStart = regexp.MustCompile(`<div[^>]*class="markdown-body[^"]*"[^>]*>`)
	divRe   = regexp.MustCompile(`(?i)<(/?)div\b`)
)

// NotesFromPage cuts the body of the release notes out of a release page,
// counting nested divs.
func NotesFromPage(html []byte) string {
	m := mdStart.FindIndex(html)
	if m == nil {
		return ""
	}
	depth := 1
	for _, t := range divRe.FindAllSubmatchIndex(html[m[1]:], -1) {
		if t[3] > t[2] { // "/"
			depth--
		} else {
			depth++
		}
		if depth == 0 {
			return string(html[m[1] : m[1]+t[0]])
		}
	}
	return string(html[m[1]:])
}

var (
	changeLogRe = regexp.MustCompile(`href="(https?://[^"]+ChangeLog[^"/]*\.(?:html|md|txt))"`)
	headRe      = regexp.MustCompile(`(?is)<head\b.*?</head>|<script\b.*?</script>|<style\b.*?</style>`)
)

// fullChangeLog reads the change log a release links to ("Full ChangeLog"):
// final Asterisk releases keep only a summary in the notes.
func fullChangeLog(ctx context.Context, f *fetch.Fetcher, notes string) string {
	m := changeLogRe.FindStringSubmatch(notes)
	if m == nil {
		return ""
	}
	res := f.Get(ctx, m[1], fetch.Options{TTL: 30 * 24 * time.Hour})
	if res.Body == nil {
		return ""
	}
	if strings.HasSuffix(m[1], ".html") {
		return headRe.ReplaceAllString(string(res.Body), "")
	}
	return "<pre>" + html.EscapeString(string(res.Body)) + "</pre>"
}

// Snapshot copies what Build reads of a state.
func Snapshot(st *model.ProductState) *model.ProductState {
	return &model.ProductState{Product: st.Product, Releases: append([]*model.Release(nil), st.Releases...)}
}

// Plan of an update.
type Plan struct {
	ID       string
	Current  string
	Target   *model.Release
	From     *model.Release   // my release, when the history has it
	Path     []*model.Release // oldest first, the target last
	Notes    map[string]relnotes.Info
	Complete bool // the branch history reaches my version
	Closes   []*model.Vuln
	Remains  []*model.Vuln
}

// Risk of the whole path: the highest of its releases, "?" when no notes say anything.
func (p *Plan) Risk() string {
	risk := "?"
	for _, r := range p.Path {
		switch p.Notes[r.Tag].Risk() {
		case "high":
			return "high"
		case "medium":
			risk = "medium"
		case "low":
			if risk == "?" {
				risk = "low"
			}
		}
	}
	return risk
}

// Days from my release (or the first one on the path) to the target.
func (p *Plan) Days() int {
	if p.Target == nil || len(p.Path) == 0 {
		return 0
	}
	from := p.Path[0].Date
	if p.From != nil {
		from = p.From.Date
	}
	return int(p.Target.Date.Sub(from).Hours() / 24)
}

// With counts the releases on the path having a tag.
func (p *Plan) With(tag string) int {
	n := 0
	for _, r := range p.Path {
		if p.Notes[r.Tag].Has(tag) {
			n++
		}
	}
	return n
}

// Count sums a tag over the path.
func (p *Plan) Count(tag string) int {
	n := 0
	for _, r := range p.Path {
		n += p.Notes[r.Tag].Counts[tag]
	}
	return n
}

// branchReleases are the stable releases of the branch, oldest first; when
// the feed does not reach back to older, tags.atom is read further (8 pages).
func branchReleases(ctx context.Context, f *fetch.Fetcher, st *model.ProductState, branch string, older []int) ([]*model.Release, bool) {
	rels := map[string]*model.Release{}
	for _, r := range st.Stable() {
		if !r.Commit && r.Branch() == branch {
			rels[r.Tag] = r
		}
	}
	covered := func() bool {
		var low []int
		for _, r := range rels {
			if v := r.Version(); v != nil && (low == nil || model.CompareVersions(v, low) < 0) {
				low = v
			}
		}
		return low != nil && model.CompareVersions(low, older) <= 0
	}
	after := ""
	if n := len(st.Releases); n > 0 {
		after = st.Releases[n-1].Tag
	}
	for page := 0; page < 8 && !covered() && after != "" && ctx.Err() == nil; page++ {
		got, err := github.FetchTagsAfter(ctx, f, st.Product, after, 7*24*time.Hour)
		if err != nil || len(got) == 0 {
			break
		}
		for _, r := range got {
			if _, ok := rels[r.Tag]; !ok && !r.Prerelease() && r.Branch() == branch {
				rels[r.Tag] = r
			}
		}
		after = got[len(got)-1].Tag
	}
	out := make([]*model.Release, 0, len(rels))
	for _, r := range rels {
		if r.Version() != nil {
			out = append(out, r)
		}
	}
	sort.Slice(out, func(i, j int) bool { return model.CompareVersions(out[i].Version(), out[j].Version()) < 0 })
	return out, covered()
}

// Build the plan from current to target ("" = the newest of the branch).
// It reads the network: call it off the UI goroutine with a copy of the
// state (Snapshot) and the product's vulnerabilities taken on the UI goroutine.
func Build(ctx context.Context, f *fetch.Fetcher, st *model.ProductState, current, target string, vulns []*model.Vuln) *Plan {
	cur := versions.Parse(current)
	p := &Plan{ID: st.Product.ID, Current: current, Notes: map[string]relnotes.Info{}}
	if cur == nil {
		return p
	}
	branch := (&model.Release{Product: st.Product.ID, Tag: current}).Branch()
	rels, complete := branchReleases(ctx, f, st, branch, cur)
	p.Complete = complete
	switch {
	case target == "" && len(rels) > 0:
		p.Target = rels[len(rels)-1]
	case target != "":
		for _, r := range append(rels, st.Stable()...) {
			if r.Tag == target {
				p.Target = r
				break
			}
		}
	}
	if p.Target == nil || p.Target.Version() == nil {
		return p
	}
	tv := p.Target.Version()
	for _, r := range rels {
		switch v := r.Version(); {
		case model.CompareVersions(v, cur) == 0:
			p.From = r
		case model.CompareVersions(v, cur) > 0 && model.CompareVersions(v, tv) <= 0:
			p.Path = append(p.Path, r)
		}
	}
	// the notes of the last 20 releases; a copy, the feed's releases belong to the UI
	from := max(0, len(p.Path)-20)
	for _, r := range p.Path[from:] {
		rr := *r
		if len(rr.NotesHTML) <= 200 && !rr.Commit {
			if body, _ := github.FetchReleasePage(ctx, f, st.Product, rr.Tag); body != nil {
				if notes := NotesFromPage(body); notes != "" {
					rr.NotesHTML = notes
				}
			}
		}
		rr.NotesHTML += fullChangeLog(ctx, f, rr.NotesHTML)
		p.Notes[r.Tag] = relnotes.Classify(&rr)
	}
	tgt := versions.String(tv)
	for _, v := range vulns {
		if hit, _ := versions.Affects(v, current); hit {
			if still, _ := versions.Affects(v, tgt); still {
				p.Remains = append(p.Remains, v)
			} else {
				p.Closes = append(p.Closes, v)
			}
		}
	}
	return p
}
