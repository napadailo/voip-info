// Package health computes a 0-100 health index of a project from what is
// known about it. A component without data does not count; the result is
// pulled toward a neutral 50 in proportion to the missing weight, so that a
// project known by two metrics does not outrank a fully known one.
package health

import (
	"fmt"
	"math"

	"github.com/napadailo/voip-info/internal/analytics"
	"github.com/napadailo/voip-info/internal/model"
)

// N marks text translated by the UI when it is shown.
func N(s string) string { return s }

// Components with their weights, in display order.
var (
	Keys    = []string{"release", "cadence", "activity", "issues", "community", "security"}
	Weights = map[string]float64{"release": 25, "cadence": 10, "activity": 20, "issues": 15, "community": 10, "security": 20}
	Labels  = map[string]string{"release": N("Release freshness"), "cadence": N("Release cadence"), "activity": N("Development pace"),
		"issues": N("Responsiveness"), "community": N("Community"), "security": N("Security")}
)

// Security is what is known about vulnerabilities, filled by the caller.
type Security struct {
	Tracked          bool // a source of vulnerabilities covers the product
	SeriousYear      int  // own high/critical in the last year
	CriticalYear     int
	PendingInherited int // inherited high/critical without a fix
	Exploited        int // own vulnerabilities in CISA KEV (2 years)
}

// Part is a component: a 0..1 score (Known false = no data) and its basis.
type Part struct {
	Key   string
	Score float64
	Known bool
	Note  string // English format, translated with Args by the UI
	Args  []any
}

// Health is the index.
type Health struct {
	Score    int
	Raw      int // without the coverage correction
	Grade    string
	Coverage float64 // share of the weight computed from data
	Parts    []Part
}

// Label is "77 B", with "~" when less than 60% of the weight is known.
func (h Health) Label() string {
	s := fmt.Sprintf("%d %s", h.Score, h.Grade)
	if h.Coverage < 0.6 {
		s += "~"
	}
	return s
}

func step(x float64, limits [][2]float64, def float64) float64 {
	for _, l := range limits {
		if x <= l[0] {
			return l[1]
		}
	}
	return def
}

// Grade of a 0-100 score.
func Grade(score int) string {
	switch {
	case score >= 80:
		return "A"
	case score >= 65:
		return "B"
	case score >= 50:
		return "C"
	case score >= 35:
		return "D"
	}
	return "F"
}

// Compute the index of a product.
func Compute(st *model.ProductState, sec Security) Health {
	var parts []Part
	add := func(key string, score float64, known bool, note string, args ...any) {
		parts = append(parts, Part{key, score, known, note, args})
	}
	rs := analytics.Stats(st)
	if lt := st.Latest(); lt != nil {
		d := float64(lt.AgeDays())
		add("release", step(d, [][2]float64{{30, 1}, {90, .85}, {180, .6}, {365, .3}}, .05), true, N("last release %d days ago"), lt.AgeDays())
	} else {
		add("release", 0, false, N("no releases"))
	}
	if rs.Cadence > 0 {
		add("cadence", step(rs.Cadence, [][2]float64{{30, 1}, {60, .85}, {120, .6}, {240, .35}}, .1), true, N("median interval %.0f days"), rs.Cadence)
	} else {
		add("cadence", 0, false, N("too few releases to tell"))
	}
	gh := st.GH
	if gh.Weekly != nil {
		cur, _, pct, ok := analytics.CommitTrend(gh.Weekly)
		s := math.Min(1, math.Log10(1+float64(cur))/math.Log10(121))
		if ok && pct <= -50 {
			s *= .75
		}
		if ok {
			add("activity", s, true, N("%d commits in 12 weeks, trend %+.0f%%"), cur, pct)
		} else {
			add("activity", s, true, N("%d commits in 12 weeks"), cur)
		}
	} else {
		add("activity", 0, false, N("no GitHub API data"))
	}
	if gh.Activity != nil && (gh.Repo == nil || gh.Repo.HasIssues) {
		a := analytics.ActivityStats(gh.Activity, 30)
		if a.IssuesOpened+a.IssuesClosed >= 3 {
			ratio := math.Min(1, float64(a.IssuesClosed)/math.Max(1, float64(a.IssuesOpened)))
			speed := .5
			if a.MedianClose > 0 {
				speed = step(a.MedianClose, [][2]float64{{7, 1}, {30, .7}, {90, .4}}, .15)
			}
			add("issues", .5*ratio+.5*speed, true, N("closed %d of %d opened, median %.1f days"), a.IssuesClosed, a.IssuesOpened, a.MedianClose)
		} else {
			add("issues", 0, false, N("too few issues in 30 days"))
		}
		if a.PRsMerged > 0 {
			s := step(float64(a.Authors), [][2]float64{{1, .15}, {2, .4}, {4, .6}, {9, .85}}, 1)
			if a.TopShare > .6 && a.PRsMerged >= 5 {
				s *= .7
			}
			add("community", s, true, N("%d PR authors, top @%s %.0f%%"), a.Authors, a.TopAuthor, a.TopShare*100)
		} else {
			add("community", 0, false, N("no merged PRs in 30 days"))
		}
	} else {
		add("issues", 0, false, N("no GitHub API data"))
		add("community", 0, false, N("no GitHub API data"))
	}
	if sec.Tracked || sec.PendingInherited > 0 {
		s := step(float64(sec.SeriousYear), [][2]float64{{0, 1}, {2, .75}, {5, .55}, {10, .35}}, .15)
		s -= .1 * math.Min(3, float64(sec.CriticalYear))
		if sec.PendingInherited > 0 {
			s -= .15
		}
		if sec.Exploited > 0 {
			s -= .25
		}
		add("security", math.Max(0, s), true, N("%d high+ CVE in a year, %d critical, %d inherited without a fix, %d in KEV"),
			sec.SeriousYear, sec.CriticalYear, sec.PendingInherited, sec.Exploited)
	} else {
		add("security", 0, false, N("no vulnerability source yet"))
	}

	var known, total, sum float64
	for _, p := range parts {
		w := Weights[p.Key]
		total += w
		if p.Known {
			known += w
			sum += w * p.Score
		}
	}
	h := Health{Parts: parts, Grade: "?"}
	if known == 0 {
		return h
	}
	raw := sum / known * 100
	h.Coverage = known / total
	h.Raw = int(math.Round(raw))
	h.Score = int(math.Round(raw*h.Coverage + 50*(1-h.Coverage)))
	if gh.Repo != nil && gh.Repo.Archived {
		h.Score = min(h.Score, 10)
	}
	h.Grade = Grade(h.Score)
	return h
}
