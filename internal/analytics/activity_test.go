package analytics

import (
	"testing"
	"time"

	"github.com/napadailo/voip-info/internal/ghapi"
	"github.com/napadailo/voip-info/internal/store"
)

func ago(days int) string {
	return time.Now().Add(-time.Duration(days) * 24 * time.Hour).UTC().Format(time.RFC3339)
}

func pr(user, created, merged, closed string) ghapi.Issue {
	i := ghapi.Issue{Created: created, Closed: closed}
	i.User.Login = user
	i.PR = &struct {
		Merged string `json:"merged_at"`
	}{merged}
	return i
}

func TestActivityStats(t *testing.T) {
	items := []ghapi.Issue{
		{Created: ago(10), Closed: ago(6)},  // closed in 4 days
		{Created: ago(40), Closed: ago(20)}, // created before the window, closed in it: 20 days
		{Created: ago(3)},
		pr("alice", ago(5), ago(4), ago(4)),
		pr("alice", ago(8), ago(7), ago(7)),
		pr("bob", ago(9), ago(2), ago(2)),
		pr("eve", ago(6), "", ago(1)), // rejected
	}
	a := ActivityStats(items, 30)
	if a.IssuesOpened != 2 || a.IssuesClosed != 2 || a.PRsOpened != 4 || a.PRsMerged != 3 || a.PRsRejected != 1 {
		t.Fatalf("%+v", a)
	}
	if a.MedianClose < 11.9 || a.MedianClose > 12.1 || a.TopAuthor != "alice" || a.Authors != 2 {
		t.Fatalf("%+v", a)
	}
}

func TestCommitTrendAndDelta(t *testing.T) {
	w := make([]int, 52)
	for i := 28; i < 40; i++ {
		w[i] = 10
	}
	for i := 40; i < 52; i++ {
		w[i] = 15
	}
	if cur, prev, pct, ok := CommitTrend(w); !ok || cur != 180 || prev != 120 || pct != 50 {
		t.Fatal(cur, prev, pct, ok)
	}
	small := make([]int, 24)
	small[0], small[20] = 3, 250
	if cur, prev, _, ok := CommitTrend(small); ok || cur != 250 || prev != 3 {
		t.Fatal("a base under 10 commits gives no trend", cur, prev, ok)
	}
	h := []store.Point{{Day: "2026-10-01", Value: 100}, {Day: "2026-10-10", Value: 120}, {Day: "2026-10-17", Value: 130}}
	if d, ok := Delta(h, 7); !ok || d != 10 {
		t.Fatal(d, ok)
	}
}
