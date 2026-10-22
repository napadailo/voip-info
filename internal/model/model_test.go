package model

import (
	"reflect"
	"testing"
)

func rel(product, tag string) *Release { return &Release{Product: product, Tag: tag} }

func TestVersionAndBranch(t *testing.T) {
	cases := []struct {
		product, tag, branch string
		version              []int
		pre                  bool
	}{
		{"kamailio", "6.0.8", "6.0", []int{6, 0, 8}, false},
		{"asterisk", "20.18.2", "20", []int{20, 18, 2}, false}, // support branches by major
		{"asterisk", "24.0.0-rc1", "24", []int{24, 0, 0}, true},
		{"freepbx", "release/17.0.33.2", "17.0", []int{17, 0, 33, 2}, false},
		{"rtpengine", "mr13.5.1.27", "13.5", []int{13, 5, 1, 27}, false},
		{"livekit-agents", "livekit-agents@1.8.4", "1.8", []int{1, 8, 4}, false},
		{"vocode", "v0.1.114a2", "0.1", []int{0, 1, 114}, true}, // PEP 440
		{"vocode", "v0.1.113", "0.1", []int{0, 1, 113}, false},
		{"jitsi-meet", "jitsi-meet_11311", "stable", []int{11311}, false},
	}
	for _, c := range cases {
		r := rel(c.product, c.tag)
		if got := r.Version(); !reflect.DeepEqual(got, c.version) {
			t.Errorf("%s: version %v, want %v", c.tag, got, c.version)
		}
		if got := r.Branch(); got != c.branch {
			t.Errorf("%s: branch %q, want %q", c.tag, got, c.branch)
		}
		if got := r.Prerelease(); got != c.pre {
			t.Errorf("%s: prerelease %v, want %v", c.tag, got, c.pre)
		}
	}
}

func TestCommitPseudoRelease(t *testing.T) {
	r := &Release{Product: "sipssert", Tag: "fe85e7c", Commit: true}
	if r.Version() != nil || r.Branch() != "main" || r.Prerelease() {
		t.Errorf("commit: version %v branch %q pre %v", r.Version(), r.Branch(), r.Prerelease())
	}
}

func TestCompareVersions(t *testing.T) {
	if CompareVersions([]int{20, 18}, []int{20, 18, 0}) != 0 || CompareVersions([]int{20, 9}, []int{20, 18}) != -1 {
		t.Error("compare")
	}
}

func TestUnseen(t *testing.T) {
	st := &ProductState{Releases: []*Release{rel("x", "3"), rel("x", "2"), rel("x", "1")}, SeenTag: "2", HasSeen: true}
	if u := st.Unseen(); len(u) != 1 || u[0].Tag != "3" {
		t.Errorf("unseen %v", u)
	}
}
