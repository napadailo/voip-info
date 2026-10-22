// Package fetch is the HTTP client with a persistent cache: a TTL plus
// conditional requests (ETag / Last-Modified). A 304 does not count against
// the GitHub API rate limit, so refreshes are almost free. On a network
// error or a rate limit the stale cached body is returned.
package fetch

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/napadailo/voip-info/internal/secret"
	"github.com/napadailo/voip-info/internal/store"
)

// UserAgent identifies the tool.
const UserAgent = "voip-info (+https://github.com/napadailo/voip-info)"

// Options of a request.
type Options struct {
	TTL       time.Duration
	Force     bool // ignore the TTL, still conditional
	API       bool // GitHub API: token, rate limit accounting
	CacheOnly bool // never go to the network
	Headers   map[string]string
}

// Result of a request.
type Result struct {
	Body      []byte
	Status    int
	FromCache bool
	Err       error // set together with a stale Body when the network failed
}

// RateLimit of the GitHub API as last reported.
type RateLimit struct {
	Limit, Remaining int
	Reset            time.Time
	Known            bool
}

// Fetcher does cached HTTP GETs.
type Fetcher struct {
	store  *store.Store
	client *http.Client
	token  string
	sem    chan struct{}

	mu   sync.Mutex
	rate RateLimit
}

// New creates a fetcher; the GitHub token comes from GITHUB_TOKEN, GH_TOKEN
// or `gh auth token -h github.com`.
func New(s *store.Store) *Fetcher {
	return &Fetcher{store: s, client: &http.Client{Timeout: 30 * time.Second}, token: GitHubToken(), sem: make(chan struct{}, 8)}
}

// GitHubToken is the token for github.com where secrets.github of the
// config says; none without it (command: gh auth token takes the one of gh).
func GitHubToken() string { return secret.GitHubToken() }

// SetTransport replaces the HTTP transport (tests answer requests in process).
func (f *Fetcher) SetTransport(rt http.RoundTripper) { f.client.Transport = rt }

// HasToken reports whether API requests are authenticated.
func (f *Fetcher) HasToken() bool { return f.token != "" }

// Rate returns the last known API rate limit.
func (f *Fetcher) Rate() RateLimit {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.rate
}

// ErrNotCached is returned by a CacheOnly request with nothing cached.
var ErrNotCached = fmt.Errorf("not cached")

// Get fetches url.
func (f *Fetcher) Get(ctx context.Context, url string, o Options) Result {
	cached := f.store.Get(url)
	if cached != nil && !o.Force && (cached.Age() < o.TTL || o.CacheOnly) {
		return Result{Body: cached.Body, Status: cached.Status, FromCache: true}
	}
	if o.CacheOnly {
		return Result{Err: ErrNotCached}
	}
	stale := func(err error) Result {
		if cached != nil {
			return Result{Body: cached.Body, Status: cached.Status, FromCache: true, Err: err}
		}
		return Result{Err: err}
	}
	if o.API && f.token == "" {
		// without a token keep a reserve of the 60 requests per hour
		if r := f.Rate(); r.Known && r.Remaining < 3 && time.Now().Before(r.Reset) {
			return stale(fmt.Errorf("rate limit"))
		}
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return stale(err)
	}
	req.Header.Set("User-Agent", UserAgent)
	if cached != nil {
		if cached.ETag != "" {
			req.Header.Set("If-None-Match", cached.ETag)
		}
		if cached.LastModified != "" {
			req.Header.Set("If-Modified-Since", cached.LastModified)
		}
	}
	if o.API {
		req.Header.Set("Accept", "application/vnd.github+json")
		req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
		if f.token != "" {
			req.Header.Set("Authorization", "Bearer "+f.token)
		}
	}
	for k, v := range o.Headers {
		req.Header.Set(k, v)
	}
	select {
	case f.sem <- struct{}{}:
	case <-ctx.Done():
		return stale(ctx.Err())
	}
	resp, err := f.client.Do(req)
	<-f.sem
	if err != nil {
		return stale(err)
	}
	defer resp.Body.Close()
	if o.API {
		f.noteRate(resp.Header)
	}
	switch {
	case resp.StatusCode == http.StatusNotModified && cached != nil:
		_ = f.store.Touch(url)
		return Result{Body: cached.Body, Status: cached.Status, FromCache: true}
	case resp.StatusCode == http.StatusAccepted:
		// GitHub statistics are being computed: do not cache the empty answer
		return Result{Status: resp.StatusCode, Err: fmt.Errorf("computing")}
	case resp.StatusCode >= 200 && resp.StatusCode < 300:
		body, err := io.ReadAll(resp.Body)
		if err != nil {
			return stale(err)
		}
		_ = f.store.Put(url, resp.Header.Get("ETag"), resp.Header.Get("Last-Modified"), resp.StatusCode, body)
		return Result{Body: body, Status: resp.StatusCode}
	}
	return stale(fmt.Errorf("HTTP %d", resp.StatusCode))
}

func (f *Fetcher) noteRate(h http.Header) {
	rem, err := strconv.Atoi(h.Get("X-RateLimit-Remaining"))
	if err != nil {
		return
	}
	lim, _ := strconv.Atoi(h.Get("X-RateLimit-Limit"))
	reset, _ := strconv.ParseInt(h.Get("X-RateLimit-Reset"), 10, 64)
	f.mu.Lock()
	f.rate = RateLimit{Limit: lim, Remaining: rem, Reset: time.Unix(reset, 0), Known: true}
	f.mu.Unlock()
}
