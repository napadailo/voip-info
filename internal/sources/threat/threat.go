// Package threat reads CISA KEV (vulnerabilities exploited in the wild) and
// EPSS (the probability of exploitation within 30 days, FIRST.org).
package threat

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/napadailo/voip-info/internal/fetch"
)

// Addresses of the feeds; tests point them elsewhere.
var (
	KEVURL  = "https://www.cisa.gov/sites/default/files/feeds/known_exploited_vulnerabilities.json"
	EPSSURL = "https://api.first.org/data/v1/epss?cve="
)

// voipRe picks the KEV entries about telephony and UC, catalog or not (3CX, Mitel, Avaya…).
var voipRe = regexp.MustCompile(`(?i)\b(asterisk|freepbx|sangoma|digium|issabel|elastix|3cx|mitel|avaya|aastra|unify|` +
	`grandstream|yealink|polycom|fanvil|snom|kamailio|opensips|freeswitch|voip|sip\b|pbx|telephon|unified communications|` +
	`call ?manager|webex|teams phone|ringcentral|zoom phone|session border|sbc\b|ribbon|audiocodes|oracle communications|vonage|twilio)`)

// KEV is an entry of the CISA catalog.
type KEV struct {
	CVE, Vendor, Product, Name, Due, Action, Description string
	Added                                                time.Time
	Ransomware                                           bool
}

// VoIP reports whether the entry is about telephony.
func (k *KEV) VoIP() bool {
	return voipRe.MatchString(k.Vendor + " " + k.Product + " " + k.Name + " " + k.Description)
}

// AgeDays since it was added to KEV.
func (k *KEV) AgeDays() int { return int(time.Since(k.Added).Hours() / 24) }

// FetchKEV returns the catalog by CVE id (cached for 12 hours).
func FetchKEV(ctx context.Context, f *fetch.Fetcher, force bool) (map[string]*KEV, error) {
	res := f.Get(ctx, KEVURL, fetch.Options{TTL: 12 * time.Hour, Force: force})
	if res.Body == nil {
		return nil, fmt.Errorf("KEV: %w", res.Err)
	}
	var data struct {
		Vulnerabilities []struct {
			CVE         string `json:"cveID"`
			Vendor      string `json:"vendorProject"`
			Product     string `json:"product"`
			Name        string `json:"vulnerabilityName"`
			Added       string `json:"dateAdded"`
			Due         string `json:"dueDate"`
			Ransomware  string `json:"knownRansomwareCampaignUse"`
			Action      string `json:"requiredAction"`
			Description string `json:"shortDescription"`
		} `json:"vulnerabilities"`
	}
	if err := json.Unmarshal(res.Body, &data); err != nil {
		return nil, fmt.Errorf("KEV: %w", err)
	}
	out := map[string]*KEV{}
	for _, v := range data.Vulnerabilities {
		added, _ := time.Parse(time.DateOnly, v.Added)
		out[v.CVE] = &KEV{CVE: v.CVE, Vendor: v.Vendor, Product: v.Product, Name: v.Name, Due: v.Due, Action: v.Action,
			Description: v.Description, Added: added, Ransomware: strings.EqualFold(v.Ransomware, "known")}
	}
	return out, nil
}

// EPSS is the score of a CVE.
type EPSS struct{ Score, Percentile float64 }

// FetchEPSS scores the CVEs in batches of 80 (cached for a day; the ids are
// sorted, so the batches and their URLs stay the same).
func FetchEPSS(ctx context.Context, f *fetch.Fetcher, cves []string, force bool) (map[string]EPSS, error) {
	set := map[string]bool{}
	for _, c := range cves {
		if strings.HasPrefix(c, "CVE-") {
			set[c] = true
		}
	}
	ids := make([]string, 0, len(set))
	for c := range set {
		ids = append(ids, c)
	}
	sort.Strings(ids)
	out := map[string]EPSS{}
	var mu sync.Mutex
	var wg sync.WaitGroup
	var lastErr error
	for i := 0; i < len(ids); i += 80 {
		chunk := ids[i:min(i+80, len(ids))]
		wg.Add(1)
		go func() {
			defer wg.Done()
			res := f.Get(ctx, EPSSURL+strings.Join(chunk, ",")+"&limit=100", fetch.Options{TTL: 24 * time.Hour, Force: force})
			mu.Lock()
			defer mu.Unlock()
			if res.Body == nil {
				lastErr = fmt.Errorf("EPSS: %w", res.Err)
				return
			}
			var data struct {
				Data []struct{ CVE, EPSS, Percentile string } `json:"data"`
			}
			if json.Unmarshal(res.Body, &data) != nil {
				return
			}
			for _, d := range data.Data {
				s, _ := strconv.ParseFloat(d.EPSS, 64)
				p, _ := strconv.ParseFloat(d.Percentile, 64)
				out[d.CVE] = EPSS{s, p}
			}
		}()
	}
	wg.Wait()
	return out, lastErr
}
