// Package analytics derives metrics from little data: the last ten releases,
// weekly commits, daily snapshots.
package analytics

import (
	"sort"
	"time"

	"github.com/napadailo/voip-info/internal/model"
)

// ReleaseStats summarises a product's releases.
type ReleaseStats struct {
	Cadence  float64 // median days between stable releases; 0 = unknown
	Rel90d   int     // releases in the last 90 days, pre-releases included
	Capped   bool    // the feed holds 10 entries: there may be more
	Branches []string
	Status   string // hot, active, slow, stale, ?
}

// Stats computes ReleaseStats.
func Stats(st *model.ProductState) ReleaseStats {
	var s ReleaseStats
	stable := st.Stable()
	// intervals between distinct days: several branches released on one day are one event
	days := distinctDays(stable)
	if len(days) < 3 { // a feed full of pre-releases: count them all
		days = distinctDays(st.Releases)
	}
	if len(days) >= 3 {
		gaps := make([]float64, 0, len(days)-1)
		for i := 1; i < len(days); i++ {
			gaps = append(gaps, days[i-1].Sub(days[i]).Hours()/24)
		}
		s.Cadence = median(gaps)
	}
	for _, r := range st.Releases {
		if r.AgeDays() <= 90 {
			s.Rel90d++
		}
	}
	s.Capped = s.Rel90d == len(st.Releases) && len(st.Releases) >= 10
	seen := map[string]bool{}
	for _, r := range stable {
		if b := r.Branch(); r.AgeDays() <= 180 && !seen[b] {
			seen[b] = true
			s.Branches = append(s.Branches, b)
		}
	}
	// by the latest stable release, as AGE of the table: an rc says little about a project being alive
	s.Status = "?"
	if lt := st.Latest(); lt != nil {
		switch age := lt.AgeDays(); {
		case age <= 30:
			s.Status = "hot"
		case age <= 120:
			s.Status = "active"
		case age <= 365:
			s.Status = "slow"
		default:
			s.Status = "stale"
		}
	}
	return s
}

func distinctDays(rels []*model.Release) []time.Time {
	seen := map[string]bool{}
	var out []time.Time
	for _, r := range rels {
		d := r.Date.UTC().Truncate(24 * time.Hour)
		if k := d.Format(time.DateOnly); !seen[k] {
			seen[k] = true
			out = append(out, d)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].After(out[j]) })
	return out
}

func median(v []float64) float64 {
	s := append([]float64(nil), v...)
	sort.Float64s(s)
	n := len(s)
	if n == 0 {
		return 0
	}
	if n%2 == 1 {
		return s[n/2]
	}
	return (s[n/2-1] + s[n/2]) / 2
}

const spark = "▁▂▃▄▅▆▇█"

// Sparkline draws values as bars; zero is the lowest bar.
func Sparkline(values []int) string {
	hi := 0
	for _, v := range values {
		hi = max(hi, v)
	}
	if hi == 0 {
		hi = 1
	}
	bars := []rune(spark)
	out := make([]rune, len(values))
	for i, v := range values {
		if v == 0 {
			out[i] = bars[0]
			continue
		}
		out[i] = bars[min(7, 1+int(float64(v)/float64(hi)*6.999))]
	}
	return string(out)
}

// Age formats days as today, 5d, 3mo, 2y.
func Age(days int) string {
	switch {
	case days < 1:
		return "today"
	case days < 60:
		return itoa(days) + "d"
	case days < 730:
		return itoa(days/30) + "mo"
	}
	return itoa(days/365) + "y"
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b []byte
	for ; n > 0; n /= 10 {
		b = append([]byte{byte('0' + n%10)}, b...)
	}
	return string(b)
}
