package health

import (
	"testing"
	"time"

	"github.com/napadailo/voip-info/internal/catalog"
	"github.com/napadailo/voip-info/internal/model"
)

func TestCoverageCorrection(t *testing.T) {
	now := time.Now()
	st := &model.ProductState{Product: &catalog.Product{ID: "x"}}
	for _, d := range []int{2, 12, 22, 32} {
		st.Releases = append(st.Releases, &model.Release{Product: "x", Tag: "1.0." + string(rune('0'+d%10)), Date: now.Add(-time.Duration(d) * 24 * time.Hour)})
	}
	h := Compute(st, Security{})
	// only release (25) and cadence (10) are known: 35% coverage, raw 100 is pulled toward 50
	if h.Raw != 100 || h.Coverage < .34 || h.Coverage > .36 || h.Score != 68 || h.Label() != "68 B~" {
		t.Fatalf("%+v", h)
	}
	h = Compute(st, Security{Tracked: true, SeriousYear: 6, CriticalYear: 2, Exploited: 1})
	if sec := h.Parts[5]; !sec.Known || sec.Score != 0 {
		t.Fatalf("security %+v", sec)
	}
}
