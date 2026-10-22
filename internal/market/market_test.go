package market

import (
	"testing"
	"time"

	"github.com/napadailo/voip-info/internal/catalog"
)

func TestChangeAndNextMonth(t *testing.T) {
	q := &Quote{Closes: []float64{10, 11, 12, 15}}
	if p, ok := q.Change(3); !ok || p != 50 {
		t.Errorf("%v %v", p, ok)
	}
	if _, ok := q.Change(4); ok {
		t.Error("not enough history")
	}
	now := time.Date(2026, 10, 21, 0, 0, 0, 0, time.UTC)
	if m := NextMonth(catalog.Event{Month: 2}, now); m.Year() != 2027 || m.Month() != 2 {
		t.Errorf("%v", m)
	}
	if m := NextMonth(catalog.Event{Month: 10}, now); m.Year() != 2026 {
		t.Errorf("the current month stays: %v", m)
	}
}
