package upgrade

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/napadailo/voip-info/internal/catalog"
	"github.com/napadailo/voip-info/internal/fetch"
	"github.com/napadailo/voip-info/internal/model"
	"github.com/napadailo/voip-info/internal/store"
)

type roundTrip func(*http.Request) *http.Response

func (f roundTrip) RoundTrip(r *http.Request) (*http.Response, error) { return f(r), nil }

func TestNotesFromPage(t *testing.T) {
	page := `<div class="Box"><div class="markdown-body my-3"><h2>Fixes</h2><div class="x"><p>a</p></div><p>b</p></div><div>footer</div></div>`
	if got := NotesFromPage([]byte(page)); got != `<h2>Fixes</h2><div class="x"><p>a</p></div><p>b</p>` {
		t.Errorf("%q", got)
	}
	if NotesFromPage([]byte("<p>x</p>")) != "" {
		t.Error("no markdown body")
	}
}

func TestBuildReadsOlderTagsAndPages(t *testing.T) {
	st0, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer st0.Close()
	f := fetch.New(st0)
	now := time.Now().UTC()
	entry := func(tag string, d int, notes string) string {
		return fmt.Sprintf(`<entry><id>tag:github.com,2008:Repository/1/%s</id><updated>%s</updated><link rel="alternate" href="https://github.com/o/r/releases/tag/%s"/><title>%s</title><content type="html">%s</content></entry>`,
			tag, now.AddDate(0, 0, -d).Format(time.RFC3339), tag, tag, notes)
	}
	var pages []string
	f.SetTransport(roundTrip(func(r *http.Request) *http.Response {
		rec := httptest.NewRecorder()
		pages = append(pages, r.URL.Path+"?"+r.URL.RawQuery)
		switch {
		case strings.HasSuffix(r.URL.Path, "/tags.atom") && r.URL.Query().Get("after") == "6.1.0":
			fmt.Fprintf(rec, `<feed xmlns="http://www.w3.org/2005/Atom">%s%s</feed>`, entry("6.0.1", 60, ""), entry("6.0.0", 90, ""))
		case strings.HasSuffix(r.URL.Path, "/ChangeLog-6.0.3.md"):
			fmt.Fprint(rec, "## Commit List\n\n- res_x: Fix crash on reload\n- Add a new option to y\n")
		case strings.Contains(r.URL.Path, "/releases/tag/6.0.3"):
			fmt.Fprint(rec, `<div class="markdown-body"><p>Summary only</p><ul><li><a href="https://downloads.example.org/ChangeLog-6.0.3.md">Full ChangeLog</a></li></ul></div>`)
		case strings.Contains(r.URL.Path, "/releases/tag/6.0.2"):
			fmt.Fprint(rec, `<div class="markdown-body"><ul><li>Fix CVE-2026-1 in the parser</li><li>Breaking change: option x removed</li></ul></div>`)
		case strings.Contains(r.URL.Path, "/releases/tag/"):
			fmt.Fprint(rec, `<div class="markdown-body"><p>maintenance release with fixes</p></div>`)
		default:
			rec.WriteHeader(http.StatusNotFound)
		}
		return rec.Result()
	}))
	p := &catalog.Product{ID: "kamailio", Repo: "o/r"}
	rel := func(tag string, d int) *model.Release {
		return &model.Release{Product: "kamailio", Tag: tag, Date: now.AddDate(0, 0, -d)}
	}
	st := &model.ProductState{Product: p, Releases: []*model.Release{rel("6.0.3", 2), rel("6.0.2", 20), rel("6.1.0", 30)}}
	closes := &model.Vuln{ID: "CVE-2026-1", Severity: "high", Ranges: []model.Range{{EndExcl: "6.0.2"}}}
	remains := &model.Vuln{ID: "CVE-2026-2", Severity: "low", Ranges: []model.Range{{EndIncl: "6.0.3"}}}
	plan := Build(context.Background(), f, Snapshot(st), "6.0.1", "", []*model.Vuln{closes, remains})
	if plan.Target == nil || plan.Target.Tag != "6.0.3" || !plan.Complete || plan.From == nil || plan.Days() != 58 {
		t.Fatalf("%+v %v", plan, pages)
	}
	if len(plan.Path) != 2 || plan.Path[0].Tag != "6.0.2" || plan.Risk() != "high" || plan.With("SEC") != 1 {
		t.Errorf("path %+v notes %+v", plan.Path, plan.Notes)
	}
	if i := plan.Notes["6.0.3"]; !i.Has("FIX") || !i.Has("FEAT") {
		t.Errorf("the linked change log is not read: %+v", i)
	}
	if len(plan.Closes) != 1 || len(plan.Remains) != 1 {
		t.Errorf("closes %v remains %v", plan.Closes, plan.Remains)
	}
	if st.Releases[1].NotesHTML != "" {
		t.Error("the feed's release was changed")
	}
}
