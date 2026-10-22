package analytics

import (
	"testing"
	"time"

	"github.com/napadailo/voip-info/internal/model"
)

func TestStatsCadenceByDistinctDays(t *testing.T) {
	now := time.Now()
	day := func(d int) time.Time { return now.Add(-time.Duration(d) * 24 * time.Hour) }
	st := &model.ProductState{Releases: []*model.Release{
		{Product: "rtpengine", Tag: "mr13.5.1.27", Date: day(1)},
		{Product: "rtpengine", Tag: "mr26.0.1.23", Date: day(1)}, // same day, another branch
		{Product: "rtpengine", Tag: "mr13.5.1.26", Date: day(11)},
		{Product: "rtpengine", Tag: "mr13.5.1.25", Date: day(21)},
	}}
	s := Stats(st)
	if s.Cadence < 9.5 || s.Cadence > 10.5 {
		t.Errorf("cadence %.1f, want 10 (days, not releases)", s.Cadence)
	}
	if s.Status != "hot" || len(s.Branches) != 2 || s.Rel90d != 4 {
		t.Errorf("%+v", s)
	}
}

func TestAgeAndSparkline(t *testing.T) {
	if Age(0) != "today" || Age(5) != "5d" || Age(90) != "3mo" || Age(800) != "2y" {
		t.Error(Age(0), Age(5), Age(90), Age(800))
	}
	if Sparkline([]int{0, 5, 10}) != "▁▅█" {
		t.Error(Sparkline([]int{0, 5, 10}))
	}
}

func TestStatusByStableRelease(t *testing.T) {
	now := time.Now()
	st := &model.ProductState{Releases: []*model.Release{
		{Product: "x", Tag: "2.0.0-rc1", Date: now.AddDate(0, 0, -3)},
		{Product: "x", Tag: "1.4.0", Date: now.AddDate(0, -5, 0)},
	}}
	if s := Stats(st); s.Status != "slow" {
		t.Errorf("a fresh rc must not make a project hot: %s", s.Status)
	}
}
