package github

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/napadailo/voip-info/internal/catalog"
	"github.com/napadailo/voip-info/internal/fetch"
	"github.com/napadailo/voip-info/internal/ghapi"
)

// API is the address of the GitHub REST API; tests point it elsewhere.
var API = "https://api.github.com"

func getJSON(ctx context.Context, f *fetch.Fetcher, url string, ttl time.Duration, force, cacheOnly bool, v any) error {
	res := f.Get(ctx, url, fetch.Options{TTL: ttl, Force: force, API: true, CacheOnly: cacheOnly})
	if res.Body == nil {
		return res.Err
	}
	if err := json.Unmarshal(res.Body, v); err != nil {
		return fmt.Errorf("json: %w", err)
	}
	return res.Err
}

func repoURL(p *catalog.Product) string { return API + "/repos/" + p.Repo }

// FetchRepo returns the repository metadata.
func FetchRepo(ctx context.Context, f *fetch.Fetcher, p *catalog.Product, force, cacheOnly bool) (*ghapi.Repo, error) {
	var r ghapi.Repo
	if err := getJSON(ctx, f, repoURL(p), 6*time.Hour, force, cacheOnly, &r); err != nil && r.Created == "" {
		return nil, err
	}
	return &r, nil
}

// FetchParticipation returns the commits per week for 52 weeks; GitHub
// answers 202 while it computes them (an error "computing").
func FetchParticipation(ctx context.Context, f *fetch.Fetcher, p *catalog.Product, force, cacheOnly bool) ([]int, error) {
	var v struct {
		All []int `json:"all"`
	}
	if err := getJSON(ctx, f, repoURL(p)+"/stats/participation", 24*time.Hour, force, cacheOnly, &v); err != nil && v.All == nil {
		return nil, err
	}
	return v.All, nil
}

// FetchActivity returns the issues and PRs updated since a day (3 pages of 100 at most).
func FetchActivity(ctx context.Context, f *fetch.Fetcher, p *catalog.Product, since time.Time, force, cacheOnly bool) ([]ghapi.Issue, error) {
	var all []ghapi.Issue
	day := since.UTC().Format("2006-01-02") + "T00:00:00Z" // stable for a day: the cache works
	var lastErr error
	for page := 1; page <= 3; page++ {
		var items []ghapi.Issue
		url := fmt.Sprintf("%s/issues?state=all&sort=updated&direction=desc&per_page=100&page=%d&since=%s", repoURL(p), page, day)
		err := getJSON(ctx, f, url, 3*time.Hour, force, cacheOnly, &items)
		if items == nil {
			if page == 1 {
				return nil, err
			}
			break
		}
		lastErr = err
		all = append(all, items...)
		if len(items) < 100 {
			break
		}
	}
	return all, lastErr
}

// FetchMilestones returns the milestones, open and closed.
func FetchMilestones(ctx context.Context, f *fetch.Fetcher, p *catalog.Product, force, cacheOnly bool) ([]ghapi.Milestone, error) {
	var ms []ghapi.Milestone
	err := getJSON(ctx, f, repoURL(p)+"/milestones?state=all&sort=due_on&direction=desc&per_page=30", 12*time.Hour, force, cacheOnly, &ms)
	if ms == nil && err != nil {
		return nil, err
	}
	if ms == nil {
		ms = []ghapi.Milestone{}
	}
	return ms, nil
}

// FetchAdvisories returns the published security advisories of the repository.
func FetchAdvisories(ctx context.Context, f *fetch.Fetcher, p *catalog.Product, force, cacheOnly bool) ([]ghapi.Advisory, error) {
	var as []ghapi.Advisory
	err := getJSON(ctx, f, repoURL(p)+"/security-advisories?state=published&per_page=50", 12*time.Hour, force, cacheOnly, &as)
	if as == nil && err != nil {
		return nil, err
	}
	if as == nil {
		as = []ghapi.Advisory{}
	}
	return as, nil
}
