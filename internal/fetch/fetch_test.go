package fetch

import "testing"

func TestNextLink(t *testing.T) {
	// as GitHub sends it for security advisories
	h := `<https://api.github.com/repositories/50272879/security-advisories?state=published&per_page=30&after=Y3Vyc29yOnYyOpHOAAe7mQ%3D%3D>; rel="next"`
	if got := NextLink(h); got != "https://api.github.com/repositories/50272879/security-advisories?state=published&per_page=30&after=Y3Vyc29yOnYyOpHOAAe7mQ%3D%3D" {
		t.Errorf("%q", got)
	}
	if got := NextLink(`<https://x/?page=1>; rel="prev", <https://x/?page=3>; rel="next"`); got != "https://x/?page=3" {
		t.Errorf("next among others: %q", got)
	}
	if NextLink(`<https://x/?page=1>; rel="prev"`) != "" || NextLink("") != "" {
		t.Error("no next")
	}
}
