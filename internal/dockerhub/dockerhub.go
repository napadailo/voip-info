// Package dockerhub finds the Docker images of a product (the catalog plus
// a Docker Hub search) and tells how fresh they are: which version of the
// product an image carries by its tags and how far behind the releases it is.
package dockerhub

import (
	"context"
	"encoding/json"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/napadailo/voip-info/internal/analytics"
	"github.com/napadailo/voip-info/internal/fetch"
	"github.com/napadailo/voip-info/internal/model"
)

// API of Docker Hub.
var API = "https://hub.docker.com/v2"

// N marks a message for the catalog; the UI translates it.
func N(s string) string { return s }

var (
	tagVer   = regexp.MustCompile(`^v?(\d+)\.(\d+)(?:\.(\d+))?(?:\.(\d+))?(?:[-_.]|$)`)
	descVer  = regexp.MustCompile(`(\d+\.\d+(?:\.\d+)?)`)
	nonAlnum = regexp.MustCompile(`[^a-z0-9]`)
)

// Tag of an image.
type Tag struct {
	Name    string
	Updated time.Time
	Size    int64
	Archs   []string
}

// Image on Docker Hub.
type Image struct {
	Name        string
	Catalog     bool // listed in the catalog, else found by the search (community)
	Pulls       int
	Stars       int
	Official    bool
	Description string
	Updated     time.Time
	Tags        []Tag
	Version     []int  // the highest version of the product among the tags
	VersionTag  string // the tag (or "x.y.z (description)")
	Branch      string
	Lag         int // releases of the same branch newer than the image
	StaleBranch bool
	Note        string // English format, translated with NoteArgs by the UI
	NoteArgs    []any
}

// URL of the image page.
func (i *Image) URL() string {
	if !strings.Contains(i.Name, "/") {
		return "https://hub.docker.com/_/" + i.Name
	}
	return "https://hub.docker.com/r/" + i.Name
}

// Archs of all tags.
func (i *Image) Archs() []string {
	var out []string
	seen := map[string]bool{}
	for _, t := range i.Tags {
		for _, a := range t.Archs {
			if a != "" && !seen[a] {
				seen[a] = true
				out = append(out, a)
			}
		}
	}
	return out
}

func getJSON(ctx context.Context, f *fetch.Fetcher, u string, ttl time.Duration, v any) error {
	res := f.Get(ctx, u, fetch.Options{TTL: ttl})
	if res.Body == nil {
		return res.Err
	}
	return json.Unmarshal(res.Body, v)
}

func parseTime(s string) time.Time {
	t, _ := time.Parse(time.RFC3339, s)
	return t
}

func (i *Image) fill(ctx context.Context, f *fetch.Fetcher) {
	repo := i.Name
	if !strings.Contains(repo, "/") {
		repo = "library/" + repo
	}
	var info struct {
		Pulls       int    `json:"pull_count"`
		Stars       int    `json:"star_count"`
		Description string `json:"description"`
		Updated     string `json:"last_updated"`
	}
	var tags struct {
		Results []struct {
			Name    string `json:"name"`
			Updated string `json:"last_updated"`
			Size    int64  `json:"full_size"`
			Images  []struct {
				Arch string `json:"architecture"`
			} `json:"images"`
		} `json:"results"`
	}
	var wg sync.WaitGroup
	wg.Add(2)
	go func() { defer wg.Done(); _ = getJSON(ctx, f, API+"/repositories/"+repo+"/", 24*time.Hour, &info) }()
	go func() {
		defer wg.Done()
		_ = getJSON(ctx, f, API+"/repositories/"+repo+"/tags/?page_size=50&ordering=last_updated", 12*time.Hour, &tags)
	}()
	wg.Wait()
	if info.Pulls > 0 {
		i.Pulls = info.Pulls
	}
	if info.Stars > 0 {
		i.Stars = info.Stars
	}
	if info.Description != "" {
		i.Description = info.Description
	}
	i.Updated = parseTime(info.Updated)
	for _, t := range tags.Results {
		tag := Tag{Name: t.Name, Updated: parseTime(t.Updated), Size: t.Size}
		for _, im := range t.Images {
			if im.Arch != "" && im.Arch != "unknown" {
				tag.Archs = append(tag.Archs, im.Arch)
			}
		}
		i.Tags = append(i.Tags, tag)
	}
}

func parseVer(groups []string) []int {
	var v []int
	for _, g := range groups {
		if g != "" {
			n, _ := strconv.Atoi(g)
			v = append(v, n)
		}
	}
	return v
}

// freshness finds the version of the product in the tags and the lag behind the releases.
func (i *Image) freshness(st *model.ProductState) {
	var rels []*model.Release
	for _, r := range st.Stable() {
		if !r.Commit && r.Version() != nil {
			rels = append(rels, r)
		}
	}
	if len(rels) == 0 {
		return
	}
	top, low := 0, 1<<30
	for _, r := range rels {
		top, low = max(top, r.Version()[0]), min(low, r.Version()[0])
	}
	floor := max(low-3, top-6, 0)
	var best []int
	for _, t := range i.Tags {
		m := tagVer.FindStringSubmatch(t.Name)
		if m == nil {
			continue
		}
		v := parseVer(m[1:])
		// tags unlike versions of the product: dates, OS versions
		if v[0] > top+1 || v[0] < floor {
			continue
		}
		if best == nil || model.CompareVersions(v, best) > 0 {
			best, i.VersionTag = v, t.Name
		}
	}
	if best == nil {
		// some images name the version in the description only ("Asterisk 20.15.2")
		if m := descVer.FindString(i.Description); m != "" {
			if v := parseVer(strings.Split(m, ".")); v[0] >= top-6 && v[0] <= top+1 {
				best, i.VersionTag = v, m+" (description)"
			}
		}
	}
	if best == nil {
		i.Note = N("no version in the tags (latest/git)")
		return
	}
	i.Version = best
	ver := make([]string, len(best))
	for k, n := range best {
		ver[k] = strconv.Itoa(n)
	}
	i.Branch = (&model.Release{Product: st.Product.ID, Tag: strings.Join(ver, ".")}).Branch()
	var newer []*model.Release // newest first
	anyNewer := false
	for _, r := range rels {
		if model.CompareVersions(r.Version(), best) > 0 {
			anyNewer = true
			if r.Branch() == i.Branch {
				newer = append(newer, r)
			}
		}
	}
	i.Lag = len(newer)
	live := analytics.Stats(st).Branches
	isLive := false
	for _, b := range live {
		isLive = isLive || b == i.Branch
	}
	switch {
	case len(best) == 2 && isLive:
		// "4.0" is a floating tag: it follows the newest release of the branch
		i.Lag = 0
		i.Note, i.NoteArgs = N("floating tag %s follows branch %s"), []any{i.VersionTag, i.Branch}
	case !isLive && anyNewer:
		i.StaleBranch = true
		i.Note, i.NoteArgs = N("branch %s has had no releases for half a year; live branches: %s"), []any{i.Branch, strings.Join(first(live, 3), ", ")}
	case len(newer) > 0:
		i.Note, i.NoteArgs = N("%d behind in branch %s: the latest is %s"), []any{len(newer), i.Branch, newer[0].Tag}
	default:
		i.Note, i.NoteArgs = N("up to date for branch %s"), []any{i.Branch}
	}
}

func first(s []string, n int) []string {
	if len(s) > n {
		return s[:n]
	}
	return s
}

func token(name string) string {
	return nonAlnum.ReplaceAllString(strings.ToLower(strings.Fields(name + " x")[0]), "")
}

// ProductImages are the images of a product: those of the catalog, then the
// most pulled of the search whose name has the product's, up to limit.
// It reads the network: call it off the UI goroutine with a state copy.
func ProductImages(ctx context.Context, f *fetch.Fetcher, st *model.ProductState, limit int) ([]*Image, error) {
	p := st.Product
	byName := map[string]*Image{}
	var all []*Image
	for _, n := range p.Docker {
		img := &Image{Name: n, Catalog: true}
		byName[n] = img
		all = append(all, img)
	}
	var search struct {
		Results []struct {
			Name        string `json:"repo_name"`
			Pulls       int    `json:"pull_count"`
			Stars       int    `json:"star_count"`
			Official    bool   `json:"is_official"`
			Description string `json:"short_description"`
		} `json:"results"`
	}
	q, _, _ := strings.Cut(p.Name, " (")
	err := getJSON(ctx, f, API+"/search/repositories/?query="+url.QueryEscape(q)+"&page_size=25", 24*time.Hour, &search)
	tok := token(p.Name)
	for _, r := range search.Results {
		// the search is fuzzy: keep the images named after the product
		if byName[r.Name] != nil || tok != "" && !strings.Contains(nonAlnum.ReplaceAllString(strings.ToLower(r.Name), ""), tok) {
			continue
		}
		img := &Image{Name: r.Name, Pulls: r.Pulls, Stars: r.Stars, Official: r.Official, Description: r.Description}
		byName[r.Name] = img
		all = append(all, img)
	}
	order := func() {
		sort.SliceStable(all, func(i, j int) bool {
			if all[i].Catalog != all[j].Catalog {
				return all[i].Catalog
			}
			return all[i].Pulls > all[j].Pulls
		})
	}
	order()
	if len(all) > limit {
		all = all[:limit]
	}
	var wg sync.WaitGroup
	for _, img := range all {
		wg.Add(1)
		go func() { defer wg.Done(); img.fill(ctx, f); img.freshness(st) }()
	}
	wg.Wait()
	order()
	if len(all) > 0 {
		err = nil
	}
	return all, err
}
