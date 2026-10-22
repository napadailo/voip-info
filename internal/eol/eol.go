// Package eol computes how long release branches are supported.
//
// Exact policies:
//
//	Asterisk - LTS (even majors): 4 years of full support + 1 year of security
//	           fixes; standard (odd): 1 + 1 year; counted from x.0.0.
//	Kamailio - the two latest x.y series are maintained.
//
// Other products: a heuristic by the activity of the branch.
package eol

import (
	"context"
	"encoding/json"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/napadailo/voip-info/internal/fetch"
	"github.com/napadailo/voip-info/internal/model"
)

const year = 365 * 24 * time.Hour

// N marks text translated by the UI when it is shown.
func N(s string) string { return s }

// Branch is the support status of a release branch.
type Branch struct {
	Branch    string
	Kind      string // LTS, standard, - (unknown)
	Released  time.Time
	FullUntil time.Time // zero = unknown
	EOL       time.Time
	Status    string // full, security, eol, upcoming, active?, stale?, eol?
	Source    string // policy or heuristic
	Latest    *model.Release
	Note      string // English, translated by the UI
	NoteArg   string
}

// Next is the nearest change: the end of full support or the EOL.
func (b Branch) Next(now time.Time) (string, time.Time, bool) {
	if !b.FullUntil.IsZero() && b.FullUntil.After(now) {
		return N("security-only"), b.FullUntil, true
	}
	if !b.EOL.IsZero() && b.EOL.After(now) {
		return N("EOL"), b.EOL, true
	}
	return "", time.Time{}, false
}

// DaysLeft until the EOL, ok false when unknown.
func (b Branch) DaysLeft(now time.Time) (int, bool) {
	if b.EOL.IsZero() {
		return 0, false
	}
	return int(b.EOL.Sub(now).Hours() / 24), true
}

var htmlTime = regexp.MustCompile(`datetime="(\d{4}-\d{2}-\d{2}T[^"]+)"`)

// AsteriskMajorDates finds when each x.0.0 came out: the GitHub API, else
// the release page (no rate limit). Cached for 30 days.
func AsteriskMajorDates(ctx context.Context, f *fetch.Fetcher, majors []int, api, web string) map[int]time.Time {
	out := map[int]time.Time{}
	for _, m := range majors {
		tag := strconv.Itoa(m) + ".0.0"
		res := f.Get(ctx, api+"/repos/asterisk/asterisk/releases/tags/"+tag, fetch.Options{TTL: 30 * 24 * time.Hour, API: true})
		if res.Body != nil {
			var v struct {
				Published string `json:"published_at"`
			}
			if json.Unmarshal(res.Body, &v) == nil {
				if t, err := time.Parse(time.RFC3339, v.Published); err == nil {
					out[m] = t
					continue
				}
			}
		}
		page := f.Get(ctx, web+"/asterisk/asterisk/releases/tag/"+tag, fetch.Options{TTL: 30 * 24 * time.Hour})
		if page.Body != nil {
			if mt := htmlTime.FindSubmatch(page.Body); mt != nil {
				if t, err := time.Parse(time.RFC3339, string(mt[1])); err == nil {
					out[m] = t
				}
			}
		}
	}
	return out
}

func latestByBranch(st *model.ProductState) map[string]*model.Release {
	out := map[string]*model.Release{}
	for _, r := range st.Stable() {
		if !r.Commit {
			if _, ok := out[r.Branch()]; !ok {
				out[r.Branch()] = r
			}
		}
	}
	return out
}

func asterisk(st *model.ProductState, x0 map[int]time.Time, now time.Time) []Branch {
	// branches by major, taken from the versions (not from branch names)
	latest := map[string]*model.Release{}
	majors := map[int]bool{}
	for _, r := range st.Stable() {
		if v := r.Version(); len(v) > 0 && !r.Commit {
			majors[v[0]] = true
			if _, ok := latest[strconv.Itoa(v[0])]; !ok {
				latest[strconv.Itoa(v[0])] = r
			}
		}
	}
	for m := range x0 {
		majors[m] = true
	}
	var out []Branch
	// a future branch: only release candidates so far (24.0.0-rc1)
	upcoming := map[int]*model.Release{}
	for _, r := range st.Releases {
		if v := r.Version(); r.Prerelease() && len(v) > 0 && !majors[v[0]] && upcoming[v[0]] == nil {
			upcoming[v[0]] = r
		}
	}
	for m, rc := range upcoming {
		kind, note := "standard", N("in %s; after %s.0.0: 1 year + 1 year of security fixes")
		if m%2 == 0 {
			kind, note = "LTS", N("in %s; after %s.0.0: 4 years of full support + 1 year of security fixes")
		}
		out = append(out, Branch{Branch: strconv.Itoa(m), Kind: kind, Status: "upcoming", Source: N("policy"), Latest: rc,
			Note: note, NoteArg: rc.Tag + "\x00" + strconv.Itoa(m)})
	}
	for m := range majors {
		lts := m%2 == 0
		b := Branch{Branch: strconv.Itoa(m), Kind: "standard", Source: N("policy"), Latest: latest[strconv.Itoa(m)], Status: "?"}
		full := year
		if lts {
			b.Kind, full = "LTS", 4*year
		}
		if rel, ok := x0[m]; ok {
			b.Released, b.FullUntil, b.EOL = rel, rel.Add(full), rel.Add(full+year)
			switch {
			case now.Before(b.FullUntil):
				b.Status = "full"
			case now.Before(b.EOL):
				b.Status = "security"
			default:
				b.Status = "eol"
			}
		} else {
			b.Note = N("the x.0.0 date is unknown")
		}
		out = append(out, b)
	}
	sort.Slice(out, func(i, j int) bool {
		a, _ := strconv.Atoi(out[i].Branch)
		b, _ := strconv.Atoi(out[j].Branch)
		return a > b
	})
	return out
}

func kamailio(st *model.ProductState) []Branch {
	latest := latestByBranch(st)
	branches := make([]string, 0, len(latest))
	for b := range latest {
		branches = append(branches, b)
	}
	sort.Slice(branches, func(i, j int) bool {
		return model.CompareVersions(latest[branches[i]].Version(), latest[branches[j]].Version()) > 0
	})
	var out []Branch
	for i, b := range branches {
		br := Branch{Branch: b, Kind: "-", Source: N("policy"), Latest: latest[b]}
		switch age := latest[b].AgeDays(); {
		case i == 0:
			br.Status, br.Note = "full", N("the latest branch")
		case i == 1:
			br.Status, br.Note = "full", N("maintained (the two latest branches)")
		case age <= 180:
			br.Status, br.Note, br.NoteArg = "security", N("older than the two latest, fixes still come (%s days ago)"), strconv.Itoa(age)
		default:
			br.Status, br.Note = "eol", N("older than the two latest, no release for half a year")
		}
		out = append(out, br)
	}
	return out
}

func heuristic(st *model.ProductState) []Branch {
	latest := latestByBranch(st)
	var out []Branch
	for b, r := range latest {
		br := Branch{Branch: b, Kind: "-", Source: N("heuristic"), Latest: r, Note: N("last release of the branch %s days ago"), NoteArg: strconv.Itoa(r.AgeDays())}
		switch age := r.AgeDays(); {
		case age <= 180:
			br.Status = "active?"
		case age <= 365:
			br.Status = "stale?"
		default:
			br.Status = "eol?"
		}
		out = append(out, br)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Latest.Date.After(out[j].Latest.Date) })
	return out
}

// Matrix is the support of every branch of a product.
func Matrix(st *model.ProductState, x0 map[int]time.Time, now time.Time) []Branch {
	switch {
	case len(st.Releases) > 0 && st.Releases[0].Commit:
		return nil
	case st.Product.ID == "asterisk":
		return asterisk(st, x0, now)
	case st.Product.ID == "kamailio":
		return kamailio(st)
	}
	return heuristic(st)
}

// SplitArgs splits NoteArg into format arguments.
func SplitArgs(s string) []any {
	if s == "" {
		return nil
	}
	var out []any
	for _, p := range strings.Split(s, "\x00") {
		out = append(out, p)
	}
	return out
}
