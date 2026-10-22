// Package versions compares versions and checks them against vulnerable ranges.
package versions

import (
	"regexp"
	"strconv"
	"strings"

	"github.com/napadailo/voip-info/internal/model"
)

var numRe = regexp.MustCompile(`\d+(?:\.\d+)*`)

// Parse takes the first dotted number of s: "v1.10.11" -> [1 10 11],
// "release/17.0.5" -> [17 0 5]; nil when there is none.
func Parse(s string) []int {
	m := numRe.FindString(strings.ReplaceAll(s, "_", "."))
	if m == "" {
		return nil
	}
	var v []int
	for _, p := range strings.Split(m, ".") {
		n, _ := strconv.Atoi(p)
		v = append(v, n)
	}
	return v
}

// String joins a version back.
func String(v []int) string {
	s := make([]string, len(v))
	for i, n := range v {
		s[i] = strconv.Itoa(n)
	}
	return strings.Join(s, ".")
}

// InRange reports whether v is in r; ok false when r cannot be read.
func InRange(v []int, r model.Range) (in, ok bool) {
	if r.Exact != "" {
		e := Parse(r.Exact)
		if e == nil {
			return false, false
		}
		return model.CompareVersions(v, e) == 0, true
	}
	bounds := []struct {
		s    string
		test func(c int) bool
	}{
		{r.StartIncl, func(c int) bool { return c >= 0 }},
		{r.StartExcl, func(c int) bool { return c > 0 }},
		{r.EndIncl, func(c int) bool { return c <= 0 }},
		{r.EndExcl, func(c int) bool { return c < 0 }},
	}
	any := false
	for _, b := range bounds {
		bv := Parse(b.s)
		if bv == nil {
			continue
		}
		any = true
		if !b.test(model.CompareVersions(v, bv)) {
			return false, true
		}
	}
	if !any {
		return false, false
	}
	if r.BranchScoped {
		end := Parse(r.EndIncl)
		if end == nil {
			end = Parse(r.EndExcl)
		}
		if end != nil && len(v) > 0 && end[0] != v[0] {
			return false, true
		}
	}
	return true, true
}

// Affects reports whether version is vulnerable; known false when the
// vulnerability has no ranges that can be read.
func Affects(vu *model.Vuln, version string) (affected, known bool) {
	v := Parse(version)
	if v == nil || len(vu.Ranges) == 0 {
		return false, false
	}
	allOut := true
	for _, r := range vu.Ranges {
		in, ok := InRange(v, r)
		if in {
			return true, true
		}
		if !ok {
			allOut = false
		}
	}
	return false, allOut
}

// FixedIn lists the patched versions of the ranges that contain version.
func FixedIn(vu *model.Vuln, version string) []string {
	v := Parse(version)
	var out []string
	for _, r := range vu.Ranges {
		if in, _ := InRange(v, r); in && r.Fixed != "" {
			out = append(out, r.Fixed)
		}
	}
	return out
}
