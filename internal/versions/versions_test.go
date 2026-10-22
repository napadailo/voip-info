package versions

import (
	"reflect"
	"testing"

	"github.com/napadailo/voip-info/internal/model"
)

func TestParse(t *testing.T) {
	if !reflect.DeepEqual(Parse("v1.10.11"), []int{1, 10, 11}) || !reflect.DeepEqual(Parse("release/17.0.5"), []int{17, 0, 5}) || Parse("x") != nil {
		t.Error("parse")
	}
}

func TestInRangeBounds(t *testing.T) {
	r := model.Range{StartIncl: "20.0.0", EndExcl: "20.20.1"}
	for v, want := range map[string]bool{"20.18.0": true, "20.20.1": false, "19.9": false} {
		if in, ok := InRange(Parse(v), r); in != want || !ok {
			t.Errorf("%s: %v %v", v, in, ok)
		}
	}
}

// Asterisk lists branches by upper bounds only: "<= 21.12.2" must not catch branch 20.
func TestBranchScopedUpperBounds(t *testing.T) {
	v := &model.Vuln{Ranges: []model.Range{
		{EndIncl: "20.20.0", Fixed: "20.20.1", BranchScoped: true},
		{EndIncl: "21.12.2", Fixed: "21.12.3", BranchScoped: true},
	}}
	cases := map[string]bool{"20.18.0": true, "20.21.0": false, "21.12.3": false, "21.10.0": true}
	for ver, want := range cases {
		if got, known := Affects(v, ver); got != want || !known {
			t.Errorf("%s: affected %v known %v, want %v", ver, got, known, want)
		}
	}
	if f := FixedIn(v, "20.18.0"); len(f) != 1 || f[0] != "20.20.1" {
		t.Errorf("fixed in %v", f)
	}
}

func TestAffectsUnknownWithoutRanges(t *testing.T) {
	if _, known := Affects(&model.Vuln{}, "1.0"); known {
		t.Error("no ranges must be unknown")
	}
}
