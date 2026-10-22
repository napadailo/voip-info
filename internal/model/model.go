// Package model holds releases and the per-product state.
package model

import (
	"regexp"
	"strconv"
	"time"

	"github.com/napadailo/voip-info/internal/catalog"
	"github.com/napadailo/voip-info/internal/ghapi"
)

var (
	preRe   = regexp.MustCompile(`(?i)(?:^|[^a-z])(rc|alpha|beta|pre|preview|dev|nightly|snapshot)\d*|\d(?:a|b)\d+$`)
	verRe   = regexp.MustCompile(`(\d+)\.(\d+)(?:\.(\d+))?(?:\.(\d+))?`)
	buildRe = regexp.MustCompile(`(\d+)$`)
)

// majorOnly products have support branches by major version (Asterisk 20 LTS, 22 LTS...).
var majorOnly = map[string]bool{"asterisk": true}

// Release is a release, a tag or (for projects without releases) a commit.
type Release struct {
	Product, Tag, Title, URL string
	Date                     time.Time
	NotesHTML                string
	Commit                   bool
}

// Prerelease reports an rc/beta/alpha/dev or PEP 440 pre-release tag.
func (r *Release) Prerelease() bool { return !r.Commit && preRe.MatchString(r.Tag) }

// Version parses the version numbers from the tag; a build number without
// dots (jitsi-meet_11311) is a one-element version; nil when there is none.
func (r *Release) Version() []int {
	if r.Commit {
		return nil
	}
	if m := verRe.FindStringSubmatch(r.Tag); m != nil {
		var v []int
		for _, s := range m[1:] {
			if s == "" {
				break
			}
			n, _ := strconv.Atoi(s)
			v = append(v, n)
		}
		return v
	}
	if m := buildRe.FindStringSubmatch(r.Tag); m != nil {
		n, _ := strconv.Atoi(m[1])
		return []int{n}
	}
	return nil
}

// Branch is major.minor, or major for majorOnly products; "main" for
// commits, "stable" for build numbers.
func (r *Release) Branch() string {
	if r.Commit {
		return "main"
	}
	v := r.Version()
	switch {
	case len(v) == 0:
		return "?"
	case len(v) == 1:
		return "stable"
	case v[0] >= 10 && majorOnly[r.Product]:
		return strconv.Itoa(v[0])
	}
	return strconv.Itoa(v[0]) + "." + strconv.Itoa(v[1])
}

// AgeDays is the number of whole days since the release.
func (r *Release) AgeDays() int { return int(time.Since(r.Date).Hours() / 24) }

// CompareVersions compares two versions, missing parts as zero.
func CompareVersions(a, b []int) int {
	for i := 0; i < max(len(a), len(b)); i++ {
		var x, y int
		if i < len(a) {
			x = a[i]
		}
		if i < len(b) {
			y = b[i]
		}
		if x != y {
			if x < y {
				return -1
			}
			return 1
		}
	}
	return 0
}

// GitHub is what the GitHub API told about a product; nil slices are
// "not loaded", empty ones "loaded, nothing there".
type GitHub struct {
	Repo       *ghapi.Repo
	Weekly     []int // commits per week, 52 weeks, oldest first
	Activity   []ghapi.Issue
	Milestones []ghapi.Milestone
	Advisories []ghapi.Advisory
	Errs       []string
	Full       bool // everything loaded, not only the table metrics
	Loading    bool
}

// ProductState is what is known about a product.
type ProductState struct {
	Product  *catalog.Product
	Releases []*Release // newest first
	Err      error
	SeenTag  string
	HasSeen  bool
	Loading  bool
	GH       GitHub
	NVD      []*Vuln // nil until loaded
	NVDErr   error
}

// Stable are the releases that are not pre-releases.
func (s *ProductState) Stable() []*Release {
	var out []*Release
	for _, r := range s.Releases {
		if !r.Prerelease() {
			out = append(out, r)
		}
	}
	return out
}

// Latest is the newest stable release, else the newest one.
func (s *ProductState) Latest() *Release {
	if st := s.Stable(); len(st) > 0 {
		return st[0]
	}
	if len(s.Releases) > 0 {
		return s.Releases[0]
	}
	return nil
}

// LatestPre is the newest release when it is a pre-release.
func (s *ProductState) LatestPre() *Release {
	if len(s.Releases) > 0 && s.Releases[0].Prerelease() {
		return s.Releases[0]
	}
	return nil
}

// Unseen are the releases newer than the one seen last.
func (s *ProductState) Unseen() []*Release {
	if !s.HasSeen {
		return nil
	}
	var out []*Release
	for _, r := range s.Releases {
		if r.Tag == s.SeenTag {
			break
		}
		out = append(out, r)
	}
	return out
}

// IsUnseen reports whether r is newer than the one seen last.
func (s *ProductState) IsUnseen(r *Release) bool {
	for _, u := range s.Unseen() {
		if u == r {
			return true
		}
	}
	return false
}
