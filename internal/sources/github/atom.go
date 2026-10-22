// Package github reads releases from the Atom feeds of github.com (no API
// rate limit) and metrics from the REST API.
package github

import (
	"context"
	"encoding/xml"
	"fmt"
	"net/url"
	"sort"
	"strings"
	"time"

	"github.com/napadailo/voip-info/internal/catalog"
	"github.com/napadailo/voip-info/internal/fetch"
	"github.com/napadailo/voip-info/internal/model"
)

type atomFeed struct {
	Entries []struct {
		ID      string `xml:"id"`
		Title   string `xml:"title"`
		Updated string `xml:"updated"`
		Link    struct {
			Href string `xml:"href,attr"`
		} `xml:"link"`
		Content string `xml:"content"`
		Author  struct {
			Name string `xml:"name"`
		} `xml:"author"`
	} `xml:"entry"`
}

// ParseReleases parses releases.atom or tags.atom. The tag is what follows
// "Repository/<id>/" in the entry id and may contain "/".
func ParseReleases(p *catalog.Product, body []byte) ([]*model.Release, error) {
	var f atomFeed
	if err := xml.Unmarshal(body, &f); err != nil {
		return nil, fmt.Errorf("atom: %w", err)
	}
	var out []*model.Release
	for _, e := range f.Entries {
		tag := e.ID
		if parts := strings.SplitN(e.ID, "/", 3); len(parts) == 3 {
			tag = parts[2]
		}
		if !p.TagMatches(tag) {
			continue
		}
		date, _ := time.Parse(time.RFC3339, e.Updated)
		title := strings.TrimSpace(e.Title)
		if title == "" {
			title = tag
		}
		out = append(out, &model.Release{Product: p.ID, Tag: tag, Title: title, Date: date, URL: e.Link.Href, NotesHTML: e.Content})
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Date.After(out[j].Date) })
	return out, nil
}

// ParseCommits turns commits.atom into pseudo-releases: the short sha as
// the tag, the commit message as the notes.
func ParseCommits(p *catalog.Product, body []byte) ([]*model.Release, error) {
	var f atomFeed
	if err := xml.Unmarshal(body, &f); err != nil {
		return nil, fmt.Errorf("atom: %w", err)
	}
	var out []*model.Release
	for _, e := range f.Entries {
		sha := e.ID[strings.LastIndex(e.ID, "/")+1:]
		if len(sha) > 7 {
			sha = sha[:7]
		}
		title := strings.TrimSpace(e.Title)
		if i := strings.IndexByte(title, '\n'); i >= 0 {
			title = title[:i]
		}
		if title == "" {
			title = sha
		}
		date, _ := time.Parse(time.RFC3339, e.Updated)
		notes := e.Content
		if e.Author.Name != "" {
			notes += "<p>— " + e.Author.Name + "</p>"
		}
		out = append(out, &model.Release{Product: p.ID, Tag: sha, Title: title, Date: date, URL: e.Link.Href, NotesHTML: notes, Commit: true})
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Date.After(out[j].Date) })
	return out, nil
}

// Base is the address of github.com; tests point it at a stub server.
var Base = "https://github.com"

func feedURL(p *catalog.Product, name string) string {
	return Base + "/" + p.Repo + "/" + name + ".atom"
}

// FetchReleases returns the releases of a product; projects without
// releases or tags (and track: commits) are followed by their commits.
func FetchReleases(ctx context.Context, f *fetch.Fetcher, p *catalog.Product, force bool) ([]*model.Release, error) {
	commits := func() ([]*model.Release, error) {
		res := f.Get(ctx, feedURL(p, "commits"), fetch.Options{TTL: 30 * time.Minute, Force: force})
		if res.Body == nil {
			return nil, res.Err
		}
		return ParseCommits(p, res.Body)
	}
	if p.Track == "commits" {
		return commits()
	}
	res := f.Get(ctx, feedURL(p, "releases"), fetch.Options{TTL: 30 * time.Minute, Force: force})
	if res.Body == nil {
		return nil, res.Err
	}
	rels, err := ParseReleases(p, res.Body)
	if err != nil {
		return nil, err
	}
	if len(rels) == 0 {
		return commits()
	}
	// a feed of only release candidates (Asterisk before a release): read
	// older tags until a stable release turns up, so "latest" is a release
	for page := 0; page < 3 && !hasStable(rels); page++ {
		more := f.Get(ctx, feedURL(p, "tags")+"?after="+url.QueryEscape(rels[len(rels)-1].Tag), fetch.Options{TTL: 6 * time.Hour, Force: force})
		if more.Body == nil {
			break
		}
		older, err := ParseReleases(p, more.Body)
		if err != nil || len(older) == 0 {
			break
		}
		rels = appendNew(rels, older)
	}
	return rels, res.Err
}

func hasStable(rels []*model.Release) bool {
	for _, r := range rels {
		if !r.Prerelease() {
			return true
		}
	}
	return false
}

// appendNew adds the releases of b that a does not have, keeping newest first.
func appendNew(a, b []*model.Release) []*model.Release {
	seen := map[string]bool{}
	for _, r := range a {
		seen[r.Tag] = true
	}
	for _, r := range b {
		if !seen[r.Tag] {
			a = append(a, r)
		}
	}
	sort.SliceStable(a, func(i, j int) bool { return a[i].Date.After(a[j].Date) })
	return a
}

// FetchTagsAfter reads the page of tags.atom older than the tag after.
func FetchTagsAfter(ctx context.Context, f *fetch.Fetcher, p *catalog.Product, after string, ttl time.Duration) ([]*model.Release, error) {
	res := f.Get(ctx, feedURL(p, "tags")+"?after="+url.QueryEscape(after), fetch.Options{TTL: ttl})
	if res.Body == nil {
		return nil, res.Err
	}
	return ParseReleases(p, res.Body)
}

// FetchReleasePage reads the HTML page of a release.
func FetchReleasePage(ctx context.Context, f *fetch.Fetcher, p *catalog.Product, tag string) ([]byte, error) {
	res := f.Get(ctx, Base+"/"+p.Repo+"/releases/tag/"+url.PathEscape(tag), fetch.Options{TTL: 30 * 24 * time.Hour})
	return res.Body, res.Err
}
