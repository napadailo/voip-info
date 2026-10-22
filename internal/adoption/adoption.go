// Package adoption measures the real use of the products: npm and PyPI
// downloads and Docker Hub pulls. npm gives daily downloads (the trend is
// week over week), pypistats the last day, week and month, Docker Hub a
// running pull count whose growth comes from the daily snapshots.
package adoption

import (
	"context"
	"encoding/json"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/napadailo/voip-info/internal/catalog"
	"github.com/napadailo/voip-info/internal/fetch"
)

// Sources, replaceable in tests.
var (
	NPM    = "https://api.npmjs.org/downloads/range/last-month/"
	PyPI   = "https://pypistats.org/api/packages/"
	Docker = "https://hub.docker.com/v2/repositories/"
	// PyPIPause: pypistats limits the rate strictly, one request at a time.
	PyPIPause = 2 * time.Second
)

// Stat of a package.
type Stat struct {
	Ecosystem string // npm, pypi, docker
	Name      string
	Month     int     // downloads in 30 days (npm, pypi)
	Week      int     // downloads in the last week
	Trend     float64 // %, the last week against the previous one (npm) or the average week (pypi)
	HasTrend  bool
	Total     int   // docker: all pulls
	Daily     []int // npm: by day
	Err       error
}

// URL of the package page.
func (s *Stat) URL() string {
	switch s.Ecosystem {
	case "npm":
		return "https://www.npmjs.com/package/" + s.Name
	case "pypi":
		return "https://pypi.org/project/" + s.Name + "/"
	}
	return "https://hub.docker.com/r/" + s.Name
}

func getJSON(ctx context.Context, f *fetch.Fetcher, u string, force bool, v any) (bool, error) {
	res := f.Get(ctx, u, fetch.Options{TTL: 24 * time.Hour, Force: force})
	if res.Body == nil {
		return false, res.Err
	}
	return res.FromCache, json.Unmarshal(res.Body, v)
}

func npmStat(ctx context.Context, f *fetch.Fetcher, name string, force bool) *Stat {
	s := &Stat{Ecosystem: "npm", Name: name}
	var data struct {
		Downloads []struct {
			Downloads int `json:"downloads"`
		} `json:"downloads"`
	}
	if _, s.Err = getJSON(ctx, f, NPM+strings.ReplaceAll(url.PathEscape(name), "%40", "@"), force, &data); s.Err != nil {
		return s
	}
	for _, d := range data.Downloads {
		s.Daily = append(s.Daily, d.Downloads)
		s.Month += d.Downloads
	}
	if n := len(s.Daily); n >= 14 {
		cur, prev := 0, 0
		for i := n - 7; i < n; i++ {
			cur += s.Daily[i]
		}
		for i := n - 14; i < n-7; i++ {
			prev += s.Daily[i]
		}
		s.Week = cur
		if prev > 0 {
			s.Trend, s.HasTrend = float64(cur-prev)/float64(prev)*100, true
		}
	}
	return s
}

var pypiMu sync.Mutex

func pypiStat(ctx context.Context, f *fetch.Fetcher, name string, force bool) *Stat {
	s := &Stat{Ecosystem: "pypi", Name: name}
	var data struct {
		Data struct {
			Month int `json:"last_month"`
			Week  int `json:"last_week"`
		} `json:"data"`
	}
	pypiMu.Lock()
	cached, err := getJSON(ctx, f, PyPI+strings.ToLower(name)+"/recent", force, &data)
	if !cached {
		time.Sleep(PyPIPause)
	}
	pypiMu.Unlock()
	if s.Err = err; err != nil {
		return s // pypistats often answers 429: yesterday's cache stays
	}
	s.Month, s.Week = data.Data.Month, data.Data.Week
	if s.Month > 0 && s.Week > 0 {
		avg := float64(s.Month) / 4.3
		s.Trend, s.HasTrend = (float64(s.Week)-avg)/avg*100, true
	}
	return s
}

func dockerStat(ctx context.Context, f *fetch.Fetcher, name string, force bool) *Stat {
	s := &Stat{Ecosystem: "docker", Name: name}
	var data struct {
		Pulls int `json:"pull_count"`
	}
	_, s.Err = getJSON(ctx, f, Docker+name+"/", force, &data)
	s.Total = data.Pulls
	return s
}

// Product reads the stats of every package of a product.
func Product(ctx context.Context, f *fetch.Fetcher, p *catalog.Product, force bool) []*Stat {
	type job func() *Stat
	var jobs []job
	for _, n := range p.NPM {
		jobs = append(jobs, func() *Stat { return npmStat(ctx, f, n, force) })
	}
	for _, n := range p.PyPI {
		jobs = append(jobs, func() *Stat { return pypiStat(ctx, f, n, force) })
	}
	for _, n := range p.Docker {
		jobs = append(jobs, func() *Stat { return dockerStat(ctx, f, n, force) })
	}
	out := make([]*Stat, len(jobs))
	var wg sync.WaitGroup
	for i, j := range jobs {
		wg.Add(1)
		go func() { defer wg.Done(); out[i] = j() }()
	}
	wg.Wait()
	return out
}
