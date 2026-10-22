package eol

import (
	"testing"
	"time"

	"github.com/napadailo/voip-info/internal/catalog"
	"github.com/napadailo/voip-info/internal/model"
)

func state(id string, tags map[string]int) *model.ProductState {
	st := &model.ProductState{Product: &catalog.Product{ID: id}}
	now := time.Now()
	for tag, days := range tags {
		st.Releases = append(st.Releases, &model.Release{Product: id, Tag: tag, Date: now.Add(-time.Duration(days) * 24 * time.Hour)})
	}
	for i := 1; i < len(st.Releases); i++ {
		for j := i; j > 0 && st.Releases[j].Date.After(st.Releases[j-1].Date); j-- {
			st.Releases[j], st.Releases[j-1] = st.Releases[j-1], st.Releases[j]
		}
	}
	return st
}

func TestAsteriskPolicy(t *testing.T) {
	st := state("asterisk", map[string]int{"20.21.0": 40, "23.5.0": 40, "24.0.0-rc2": 10})
	x0 := map[int]time.Time{20: time.Date(2022, 11, 2, 0, 0, 0, 0, time.UTC), 23: time.Date(2025, 10, 15, 0, 0, 0, 0, time.UTC)}
	now := time.Date(2026, 10, 21, 0, 0, 0, 0, time.UTC)
	got := map[string]Branch{}
	for _, b := range Matrix(st, x0, now) {
		got[b.Branch] = b
	}
	// 2022-11-02 + 4 years: full support until 2026-11-01, the EOL a year later
	if b := got["20"]; b.Kind != "LTS" || b.Status != "full" || b.EOL.Year() != 2027 {
		t.Errorf("20: %+v", b)
	}
	if b := got["23"]; b.Kind != "standard" || b.Status != "security" || b.FullUntil.Year() != 2026 {
		t.Errorf("23: %+v", b)
	}
	if b := got["24"]; b.Kind != "LTS" || b.Status != "upcoming" {
		t.Errorf("24: %+v", b)
	}
	if what, when, ok := got["20"].Next(now); !ok || what != "security-only" || when.Format("2006-01-02") != "2026-11-01" {
		t.Errorf("next %s %v", what, when)
	}
}

func TestKamailioTwoLatest(t *testing.T) {
	got := map[string]string{}
	for _, b := range Matrix(state("kamailio", map[string]int{"6.1.4": 10, "6.0.8": 20, "5.8.8": 300}), nil, time.Now()) {
		got[b.Branch] = b.Status
	}
	if got["6.1"] != "full" || got["6.0"] != "full" || got["5.8"] != "eol" {
		t.Errorf("%v", got)
	}
}
