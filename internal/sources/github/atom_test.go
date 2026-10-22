package github

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/napadailo/voip-info/internal/catalog"
	"github.com/napadailo/voip-info/internal/fetch"
	"github.com/napadailo/voip-info/internal/store"
)

const releasesAtom = `<?xml version="1.0" encoding="UTF-8"?>
<feed xmlns="http://www.w3.org/2005/Atom">
  <entry><id>tag:github.com,2008:Repository/1/20.21.0</id><updated>2026-08-27T10:00:00Z</updated>
    <link rel="alternate" href="https://github.com/a/b/releases/tag/20.21.0"/><title>Asterisk Release 20.21.0</title>
    <content type="html">&lt;p&gt;notes&lt;/p&gt;</content></entry>
  <entry><id>tag:github.com,2008:Repository/1/certified-20.7-cert11</id><updated>2026-09-01T10:00:00Z</updated>
    <link rel="alternate" href="x"/><title>certified</title></entry>
  <entry><id>tag:github.com,2008:Repository/1/stable/jitsi-meet_11248</id><updated>2026-09-14T10:00:00Z</updated>
    <link rel="alternate" href="y"/><title></title></entry>
</feed>`

func TestParseReleasesFilterAndSlashTags(t *testing.T) {
	ps, err := catalog.Products()
	if err != nil {
		t.Fatal(err)
	}
	var ast *catalog.Product
	for _, p := range ps {
		if p.ID == "asterisk" {
			ast = p
		}
	}
	rels, err := ParseReleases(ast, []byte(releasesAtom))
	if err != nil {
		t.Fatal(err)
	}
	// tag_filter '^\d' drops the certified line and the jitsi-style tag
	if len(rels) != 1 || rels[0].Tag != "20.21.0" || rels[0].NotesHTML != "<p>notes</p>" {
		t.Fatalf("got %+v", rels)
	}
	all, _ := ParseReleases(&catalog.Product{ID: "jvb"}, []byte(releasesAtom))
	if all[0].Tag != "stable/jitsi-meet_11248" || all[0].Title != "stable/jitsi-meet_11248" {
		t.Errorf("newest first, tag with slash, title defaults to tag: %+v", all[0])
	}
}

func TestParseCommits(t *testing.T) {
	atom := `<feed xmlns="http://www.w3.org/2005/Atom"><entry><id>tag:github.com,2008:Grit::Commit/fe85e7c1234567</id>
<updated>2026-09-08T10:00:00Z</updated><link href="u"/><title>github: name the tests
second line</title><author><name>dev</name></author></entry></feed>`
	rels, err := ParseCommits(&catalog.Product{ID: "sipssert"}, []byte(atom))
	if err != nil || len(rels) != 1 {
		t.Fatal(err, rels)
	}
	if r := rels[0]; r.Tag != "fe85e7c" || r.Title != "github: name the tests" || !r.Commit {
		t.Errorf("%+v", r)
	}
}

type rt func(*http.Request) *http.Response

func (f rt) RoundTrip(r *http.Request) (*http.Response, error) { return f(r), nil }

func entry(tag, date string) string {
	return `<entry><id>tag:github.com,2008:Repository/1/` + tag + `</id><updated>` + date + `</updated><link href="u"/><title>` + tag + `</title></entry>`
}

// A feed of only release candidates is extended with older tags until a stable release turns up.
func TestFetchReleasesReadsOlderTagsForStable(t *testing.T) {
	st, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	f := fetch.New(st)
	var asked []string
	f.SetTransport(rt(func(r *http.Request) *http.Response {
		asked = append(asked, r.URL.Path+"?"+r.URL.RawQuery)
		rec := httptest.NewRecorder()
		switch {
		case strings.HasSuffix(r.URL.Path, "/releases.atom"):
			rec.WriteString(`<feed>` + entry("24.0.0-rc2", "2026-10-08T00:00:00Z") + entry("23.6.0-rc2", "2026-10-08T00:00:00Z") + `</feed>`)
		case strings.HasSuffix(r.URL.Path, "/tags.atom") && r.URL.Query().Get("after") == "23.6.0-rc2":
			rec.WriteString(`<feed>` + entry("23.5.0", "2026-08-28T00:00:00Z") + entry("22.11.0", "2026-08-28T00:00:00Z") + `</feed>`)
		default:
			rec.WriteHeader(404)
		}
		return rec.Result()
	}))
	rels, err := FetchReleases(context.Background(), f, &catalog.Product{ID: "asterisk", Repo: "asterisk/asterisk"}, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(rels) != 4 || rels[2].Tag != "23.5.0" {
		t.Fatalf("got %d releases %v; requests %v", len(rels), rels, asked)
	}
}
