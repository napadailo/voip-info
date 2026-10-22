package analytics

import (
	"sort"
	"time"

	"github.com/napadailo/voip-info/internal/ghapi"
	"github.com/napadailo/voip-info/internal/store"
)

// Activity summarises issues and pull requests of a window.
type Activity struct {
	Days                              int
	IssuesOpened, IssuesClosed        int
	PRsOpened, PRsMerged, PRsRejected int     // rejected: closed without merge
	MedianClose                       float64 // days, issues closed in the window; 0 = none
	TopAuthor                         string  // the most frequent author of merged PRs
	TopShare                          float64 // their share: a bus factor hint
	Authors                           int     // distinct authors of merged PRs
	Truncated                         bool    // the sample hit the 300 item limit
}

// ActivityStats computes Activity over the last days.
func ActivityStats(items []ghapi.Issue, days int) Activity {
	a := Activity{Days: days, Truncated: len(items) >= 300}
	cut := time.Now().Add(-time.Duration(days) * 24 * time.Hour)
	in := func(s string) bool { t := ghapi.Time(s); return !t.IsZero() && !t.Before(cut) }
	var closeTimes []float64
	authors := map[string]int{}
	for _, it := range items {
		if it.PR != nil {
			if in(it.Created) {
				a.PRsOpened++
			}
			switch {
			case in(it.PR.Merged):
				a.PRsMerged++
				authors[it.User.Login]++
			case it.PR.Merged == "" && in(it.Closed):
				a.PRsRejected++
			}
			continue
		}
		if in(it.Created) {
			a.IssuesOpened++
		}
		if in(it.Closed) {
			a.IssuesClosed++
			if c := ghapi.Time(it.Created); !c.IsZero() {
				closeTimes = append(closeTimes, ghapi.Time(it.Closed).Sub(c).Hours()/24)
			}
		}
	}
	a.MedianClose = median(closeTimes)
	total := 0
	for name, n := range authors {
		total += n
		if n > authors[a.TopAuthor] || (n == authors[a.TopAuthor] && name < a.TopAuthor) {
			a.TopAuthor = name
		}
	}
	if total > 0 {
		a.TopShare = float64(authors[a.TopAuthor]) / float64(total)
	}
	a.Authors = len(authors)
	return a
}

// MinTrendBase: with fewer commits in the 12 weeks before, a percentage
// says nothing (3 commits then and 250 now is +8233%).
const MinTrendBase = 10

// CommitTrend returns the commits of the last 12 weeks, of the 12 before and
// the change in percent (ok false when it cannot be computed or the base is
// below MinTrendBase).
func CommitTrend(weekly []int) (cur, prev int, pct float64, ok bool) {
	if len(weekly) < 24 {
		return 0, 0, 0, false
	}
	for _, n := range weekly[len(weekly)-12:] {
		cur += n
	}
	for _, n := range weekly[len(weekly)-24 : len(weekly)-12] {
		prev += n
	}
	if prev < MinTrendBase {
		return cur, prev, 0, false
	}
	return cur, prev, float64(cur-prev) / float64(prev) * 100, true
}

// Delta is the growth of a metric over about days, from daily snapshots
// (ok false with fewer than two points).
func Delta(history []store.Point, days int) (float64, bool) {
	if len(history) < 2 {
		return 0, false
	}
	last := history[len(history)-1]
	lastDay, err := time.Parse(time.DateOnly, last.Day)
	if err != nil {
		return 0, false
	}
	target := lastDay.Add(-time.Duration(days) * 24 * time.Hour).Format(time.DateOnly)
	base := history[0].Value
	i := sort.Search(len(history), func(i int) bool { return history[i].Day > target })
	if i > 0 {
		base = history[i-1].Value
	}
	return last.Value - base, true
}
