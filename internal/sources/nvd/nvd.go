// Package nvd reads the CVE history of a product from the NVD (NIST) API 2.0
// by keyword. Without an API key the limit is 5 requests per 30 seconds,
// so the requests go one by one with a pause; NVD_API_KEY raises it to 50.
package nvd

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/napadailo/voip-info/internal/catalog"
	"github.com/napadailo/voip-info/internal/fetch"
	"github.com/napadailo/voip-info/internal/model"
	"github.com/napadailo/voip-info/internal/secret"
)

// API is the address of the CVE API; tests point it elsewhere.
var API = "https://services.nvd.nist.gov/rest/json/cves/2.0"

type response struct {
	Vulnerabilities []struct {
		CVE struct {
			ID           string `json:"id"`
			Published    string `json:"published"`
			Status       string `json:"vulnStatus"`
			Descriptions []struct {
				Lang, Value string
			} `json:"descriptions"`
			Metrics        map[string][]metric `json:"metrics"`
			Configurations []struct {
				Nodes []struct {
					CPEMatch []cpeMatch `json:"cpeMatch"`
				} `json:"nodes"`
			} `json:"configurations"`
		} `json:"cve"`
	} `json:"vulnerabilities"`
}

type metric struct {
	BaseSeverity string `json:"baseSeverity"`
	Data         struct {
		Score    float64 `json:"baseScore"`
		Severity string  `json:"baseSeverity"`
	} `json:"cvssData"`
}

type cpeMatch struct {
	Vulnerable bool   `json:"vulnerable"`
	Criteria   string `json:"criteria"`
	StartIncl  string `json:"versionStartIncluding"`
	StartExcl  string `json:"versionStartExcluding"`
	EndIncl    string `json:"versionEndIncluding"`
	EndExcl    string `json:"versionEndExcluding"`
}

func severity(m map[string][]metric) (string, float64) {
	for _, k := range []string{"cvssMetricV40", "cvssMetricV31", "cvssMetricV30", "cvssMetricV2"} {
		for _, x := range m[k] {
			sev := x.Data.Severity
			if sev == "" {
				sev = x.BaseSeverity
			}
			if sev == "" {
				sev = "?"
			}
			return strings.ToLower(sev), x.Data.Score
		}
	}
	return "?", 0
}

var boilerplate = regexp.MustCompile(`(?i)^[\w .\-/]{2,40} (is|are) (an?|the) [^.]{0,120}$`)

// Summarize takes the first sentence that says something: NVD often starts
// with "FreePBX is an open source IP PBX".
func Summarize(desc string) string {
	var sents []string
	start := 0
	for i := 0; i < len(desc); i++ {
		if (desc[i] == '.' || desc[i] == '!' || desc[i] == '?') && (i+1 == len(desc) || desc[i+1] == ' ') {
			if s := strings.TrimSpace(desc[start : i+1]); s != "" {
				sents = append(sents, s)
			}
			start = i + 1
		}
	}
	if s := strings.TrimSpace(desc[start:]); s != "" {
		sents = append(sents, s)
	}
	for len(sents) > 1 && boilerplate.MatchString(strings.TrimSuffix(sents[0], ".")) {
		sents = sents[1:]
	}
	out := desc
	if len(sents) > 0 {
		out = sents[0]
	}
	if r := []rune(out); len(r) > 160 {
		out = string(r[:160])
	}
	return out
}

// ranges are the vulnerable CPE ranges of the product itself (other CPEs of
// the configuration, an OS for one, are dropped). When none names the product,
// the ranges count only if they all belong to one other CPE product (its CPE
// name differs: pjsip is pjproject); ranges of several other products are
// theirs, the keyword found the CVE in a mention.
func ranges(p *catalog.Product, matches []cpeMatch) []model.Range {
	names := []string{strings.ToLower(p.ID), strings.ToLower(strings.ReplaceAll(p.Name, " ", "_")), strings.ToLower(strings.ReplaceAll(p.NVD, " ", "_"))}
	var own, all []model.Range
	products := map[string]bool{}
	for _, m := range matches {
		parts := strings.Split(m.Criteria, ":")
		if !m.Vulnerable || len(parts) < 6 {
			continue
		}
		r := model.Range{StartIncl: m.StartIncl, StartExcl: m.StartExcl, EndIncl: m.EndIncl, EndExcl: m.EndExcl, Fixed: m.EndExcl}
		if v := parts[5]; v != "*" && v != "-" && v != "" {
			r.Exact = v
		}
		all = append(all, r)
		products[parts[3]+":"+parts[4]] = true
		for _, n := range names {
			if n != "" && strings.Contains(strings.ToLower(parts[4]), n) {
				own = append(own, r)
				break
			}
		}
	}
	switch {
	case len(own) > 0:
		return own
	case len(products) == 1:
		return all
	}
	return nil
}

var (
	parenRe = regexp.MustCompile(`\s*\([^)]*\)`)
	// the ways NVD names the module of FreePBX a CVE is in
	moduleRes = []*regexp.Regexp{
		regexp.MustCompile(`^([\w &-]{2,40}?) is an? (?:[\w ]+ )?module\b`),                          // contactmanager is a module for FreePBX
		regexp.MustCompile(`(?i)\bFreePBX module ([a-z]\w+)`),                                        // FreePBX module api
		regexp.MustCompile(`(?i)\b(?:patched|fixed) in ([a-z]\w+) versions?\b`),                      // patched in endpoint versions
		regexp.MustCompile(`(?i)\bin (?:FreePBX )?([a-z]\w+) (?:\d+\.x )?before\b`),                  // in FreePBX core before
		regexp.MustCompile(`(?i)\b(?:in|before|FreePBX) ([a-z][a-z_]+) (?:up to |through )?\d+\.\d`), // FreePBX cdr 14.0
	}
	moduleWordRe = regexp.MustCompile(`(?i)\bmodule\b`)
	// words that end the name of a module when reading back from "module"
	moduleStop = map[string]bool{"the": true, "a": true, "an": true, "of": true, "in": true, "for": true, "and": true,
		"freepbx": true, "sangoma": true, "within": true, "by": true, "with": true, "to": true, "from": true,
		"via": true, "is": true, "are": true, "that": true, "its": true}
	notModule = map[string]bool{"certain": true, "this": true, "same": true, "each": true, "any": true, "valid": true,
		"affected": true, "vulnerable": true, "asterisk": true, "versions": true, "version": true}
)

// module is the module of a modular product a CVE is in, "" when it is the
// product itself or none is named: a CPE of another product (contactmanager),
// then the description ("the endpoint module", "patched in endpoint versions").
func module(p *catalog.Product, desc string, matches []cpeMatch) string {
	if p.Modules == "" {
		return ""
	}
	own := func(n string) string {
		n = strings.ToLower(strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(n), "FreePBX ")))
		if n == "" || notModule[n] || n == strings.ToLower(p.Modules) || n == strings.ToLower(p.ID) || n == strings.ToLower(p.Name) {
			return ""
		}
		return n
	}
	// CPEs of other products: modules, but not the distribution or a dependency (freepbx_linux_7, certified_asterisk)
	related := []string{strings.ToLower(p.ID)}
	for _, d := range p.Deps {
		related = append(related, strings.SplitN(d, ":", 2)[0])
	}
	others := map[string]bool{}
	for _, m := range matches {
		parts := strings.Split(m.Criteria, ":")
		if !m.Vulnerable || len(parts) < 5 || own(parts[4]) == "" {
			continue
		}
		mod := true
		for _, r := range related {
			mod = mod && !strings.Contains(parts[4], r)
		}
		if mod {
			others[own(parts[4])] = true
		}
	}
	if len(others) == 1 {
		for n := range others {
			return n
		}
	}
	desc = parenRe.ReplaceAllString(desc, "")
	for _, re := range moduleRes {
		for _, m := range re.FindAllStringSubmatch(desc, -1) {
			if n := own(m[1]); n != "" {
				return n
			}
		}
	}
	// "... the Endpoint Manager module": up to four words before "module"
	for _, loc := range moduleWordRe.FindAllStringIndex(desc, -1) {
		words := strings.Fields(desc[:loc[0]])
		var name []string
		for i := len(words) - 1; i >= 0 && len(name) < 4; i-- {
			w := words[i]
			if moduleStop[strings.ToLower(w)] || strings.ContainsAny(w, ".,;:") {
				break
			}
			name = append([]string{w}, name...)
		}
		if n := own(strings.Join(name, " ")); n != "" {
			return n
		}
	}
	return ""
}

// Parse reads an NVD answer for a product.
func Parse(p *catalog.Product, body []byte) ([]*model.Vuln, error) {
	var resp response
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("nvd: %w", err)
	}
	var keep, drop *regexp.Regexp
	if p.NVDFilter != "" {
		keep = regexp.MustCompile(p.NVDFilter)
	}
	if p.NVDExclude != "" {
		drop = regexp.MustCompile(p.NVDExclude)
	}
	var out []*model.Vuln
	for _, v := range resp.Vulnerabilities {
		c := v.CVE
		if c.Status == "Rejected" {
			continue
		}
		desc := ""
		for _, d := range c.Descriptions {
			if d.Lang == "en" {
				desc = d.Value
				break
			}
		}
		if (keep != nil && !keep.MatchString(desc)) || (drop != nil && drop.MatchString(desc)) {
			continue
		}
		pub, _ := time.Parse("2006-01-02T15:04:05.000", c.Published)
		sev, score := severity(c.Metrics)
		var matches []cpeMatch
		for _, conf := range c.Configurations {
			for _, n := range conf.Nodes {
				matches = append(matches, n.CPEMatch...)
			}
		}
		out = append(out, &model.Vuln{ID: c.ID, Product: p.ID, Published: pub.UTC(), Severity: sev, Score: score,
			Summary: Summarize(desc), Description: desc, URL: "https://nvd.nist.gov/vuln/detail/" + c.ID,
			Source: "nvd", Ranges: ranges(p, matches), Module: module(p, desc, matches)})
	}
	return out, nil
}

// Client fetches CVEs one product at a time, paced for the NVD limit.
type Client struct {
	f     *fetch.Fetcher
	key   string
	mu    sync.Mutex
	pause time.Duration
}

// Pause between requests: NVD allows 5 in 30 seconds without a key, 50 with one.
var Pause, KeyPause = 6500 * time.Millisecond, 700 * time.Millisecond

// HasKey reports whether there is an NVD API key: requests go 10 times faster.
func HasKey() bool { return secret.NVDKey() != "" }

// NewClient reads the key where secrets.nvd of the config says.
func NewClient(f *fetch.Fetcher) *Client {
	c := &Client{f: f, key: secret.NVDKey(), pause: Pause}
	if c.key != "" {
		c.pause = KeyPause
	}
	return c
}

// Fetch returns the CVEs of a product with an nvd keyword (cached for a day).
func (c *Client) Fetch(ctx context.Context, p *catalog.Product, force bool) ([]*model.Vuln, error) {
	if p.NVD == "" {
		return nil, nil
	}
	u := API + "?keywordSearch=" + url.QueryEscape(p.NVD) + "&resultsPerPage=2000"
	o := fetch.Options{TTL: 24 * time.Hour, Force: force}
	if c.key != "" {
		o.Headers = map[string]string{"apiKey": c.key}
	}
	c.mu.Lock()
	res := c.f.Get(ctx, u, o)
	if !res.FromCache {
		select { // pace the next request
		case <-time.After(c.pause):
		case <-ctx.Done():
		}
	}
	c.mu.Unlock()
	if res.Body == nil {
		return nil, fmt.Errorf("NVD %s: %w", p.Name, res.Err)
	}
	return Parse(p, res.Body)
}
