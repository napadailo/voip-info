package github

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/napadailo/voip-info/internal/catalog"
	"github.com/napadailo/voip-info/internal/fetch"
	"github.com/napadailo/voip-info/internal/store"
)

type roundTrip func(*http.Request) *http.Response

func (f roundTrip) RoundTrip(r *http.Request) (*http.Response, error) { return f(r), nil }

func TestFetchAdvisoriesPages(t *testing.T) {
	st, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	f := fetch.New(st)
	calls := 0
	f.SetTransport(roundTrip(func(r *http.Request) *http.Response {
		calls++
		rec := httptest.NewRecorder()
		if r.URL.Query().Get("after") == "" {
			rec.Header().Set("Link", fmt.Sprintf(`<%s/repositories/1/security-advisories?per_page=100&after=c1>; rel="next"`, API))
			fmt.Fprint(rec, `[{"ghsa_id":"GHSA-1"},{"ghsa_id":"GHSA-2"}]`)
		} else {
			fmt.Fprint(rec, `[{"ghsa_id":"GHSA-3"}]`)
		}
		return rec.Result()
	}))
	p := &catalog.Product{ID: "freepbx", Repo: "FreePBX/framework", Advisories: "FreePBX/security-reporting"}
	as, err := FetchAdvisories(context.Background(), f, p, false, false)
	if err != nil || len(as) != 3 || as[2].GHSA != "GHSA-3" || calls != 2 {
		t.Fatalf("both pages: %v %+v, %d calls", err, as, calls)
	}
	// the cursor is kept with the cached page: the cache alone gives all of them
	as, err = FetchAdvisories(context.Background(), f, p, false, true)
	if err != nil || len(as) != 3 || calls != 2 {
		t.Errorf("from the cache: %v %d, %d calls", err, len(as), calls)
	}
}
