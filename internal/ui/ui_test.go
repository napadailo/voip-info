package ui

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/gdamore/tcell/v2"

	"github.com/napadailo/voip-tui/i18n"

	"github.com/napadailo/voip-info/internal/adoption"
	"github.com/napadailo/voip-info/internal/config"
	"github.com/napadailo/voip-info/internal/hub"
	"github.com/napadailo/voip-info/internal/secret"
	"github.com/napadailo/voip-info/internal/sources/nvd"
	"github.com/napadailo/voip-info/internal/store"
)

// roundTrip serves requests in process, without a listening socket.
type roundTrip func(*http.Request) *http.Response

func (f roundTrip) RoundTrip(r *http.Request) (*http.Response, error) { return f(r), nil }

// stubGitHub answers every releases.atom with three releases of the repo:
// a release candidate from yesterday, a fresh release and one from a month ago.
func stubGitHub(h *hub.Hub) {
	h.Fetcher.SetTransport(roundTrip(func(r *http.Request) *http.Response {
		rec := httptest.NewRecorder()
		now := time.Now().UTC()
		if stubSecurity(rec, r, now) || stubInfo(rec, r, now) {
			return rec.Result()
		}
		if r.URL.Host == "api.github.com" {
			// headers before the body: the recorder freezes them at the first write
			rec.Header().Set("X-RateLimit-Remaining", "4990")
			rec.Header().Set("X-RateLimit-Limit", "5000")
			ago := func(d int) string { return now.Add(-time.Duration(d) * 24 * time.Hour).Format(time.RFC3339) }
			switch p := r.URL.Path; {
			case strings.HasSuffix(p, "/stats/participation"):
				w := make([]string, 52)
				for i := range w {
					w[i] = fmt.Sprint(5 + i/4)
				}
				fmt.Fprintf(rec, `{"all":[%s]}`, strings.Join(w, ","))
			case strings.HasSuffix(p, "/issues"):
				fmt.Fprintf(rec, `[{"number":101,"title":"crash on reload","state":"closed","html_url":"https://x/101","created_at":%q,"closed_at":%q,"updated_at":%q,"user":{"login":"ann"}},
{"number":102,"title":"add TLS 1.3","state":"closed","html_url":"https://x/102","created_at":%q,"closed_at":%q,"updated_at":%q,"user":{"login":"bob"},"pull_request":{"merged_at":%q}},
{"number":103,"title":"leak in parser","state":"open","html_url":"https://x/103","created_at":%q,"updated_at":%q,"user":{"login":"ann"},
"body":"The parser **leaks** on reload.","labels":[{"name":"bug"}]}]`,
					ago(9), ago(5), ago(5), ago(4), ago(2), ago(2), ago(2), ago(1), ago(1))
			case strings.HasSuffix(p, "/milestones"):
				fmt.Fprint(rec, `[{"title":"9.1","state":"open","open_issues":3,"closed_issues":7,"html_url":"https://x/m"}]`)
			case strings.HasSuffix(p, "/security-advisories"):
				fmt.Fprint(rec, `[]`)
			case strings.Count(p, "/") == 3: // /repos/owner/name
				fmt.Fprintf(rec, `{"stargazers_count":1234,"forks_count":56,"subscribers_count":7,"open_issues_count":42,"language":"C","created_at":%q,"pushed_at":%q,"default_branch":"main","has_issues":true,"license":{"spdx_id":"GPL-2.0"}}`, ago(3000), ago(1))
			default:
				rec.WriteHeader(http.StatusNotFound)
			}
			return rec.Result()
		}
		if !strings.HasSuffix(r.URL.Path, "/releases.atom") {
			rec.WriteHeader(http.StatusNotFound)
			return rec.Result()
		}
		entry := func(tag string, days int, notes string) string {
			return fmt.Sprintf(`<entry><id>tag:github.com,2008:Repository/1/%s</id><updated>%s</updated>
<link rel="alternate" href="https://github.com%s/tag/%s"/><title>Release %s</title><content type="html">%s</content></entry>`,
				tag, now.Add(-time.Duration(days)*24*time.Hour).Format(time.RFC3339), r.URL.Path, tag, tag, notes)
		}
		fmt.Fprintf(rec, `<?xml version="1.0"?><feed xmlns="http://www.w3.org/2005/Atom">%s%s%s</feed>`,
			entry("9.1.0-rc1", 1, ""), entry("9.0.2", 3, "&lt;h2&gt;Fixes&lt;/h2&gt;&lt;ul&gt;&lt;li&gt;crash in parser&lt;/li&gt;&lt;/ul&gt;"),
			entry("9.0.1", 40, "&lt;p&gt;older&lt;/p&gt;"))
		return rec.Result()
	}))
}

// stubSecurity answers NVD, CISA KEV and EPSS: Asterisk has an exploited CVE
// fixed in 9.0.2, PJSIP a fresh one its consumers inherit.
func stubSecurity(rec *httptest.ResponseRecorder, r *http.Request, now time.Time) bool {
	day := func(d int) string { return now.AddDate(0, 0, -d).Format("2006-01-02T15:04:05.000") }
	switch r.URL.Host {
	case "services.nvd.nist.gov":
		switch r.URL.Query().Get("keywordSearch") {
		case "Asterisk":
			fmt.Fprintf(rec, `{"vulnerabilities":[{"cve":{"id":"CVE-2026-1001","published":%q,"vulnStatus":"Analyzed",
"descriptions":[{"lang":"en","value":"Asterisk is an open source PBX. Prior to 9.0.2, res_pjsip crashes on a malformed MWI body."}],
"metrics":{"cvssMetricV31":[{"cvssData":{"baseScore":9.1,"baseSeverity":"CRITICAL"}}]},
"configurations":[{"nodes":[{"cpeMatch":[{"vulnerable":true,"criteria":"cpe:2.3:a:sangoma:asterisk:*:*:*:*:*:*:*:*","versionStartIncluding":"9.0.0","versionEndExcluding":"9.0.2"}]}]}]}}]}`, day(5))
		case "PJSIP":
			fmt.Fprintf(rec, `{"vulnerabilities":[{"cve":{"id":"CVE-2026-2002","published":%q,"vulnStatus":"Analyzed",
"descriptions":[{"lang":"en","value":"PJSIP is a multimedia communication library. In 2.15 and prior, the SDP parser overflows the heap."}],
"metrics":{"cvssMetricV31":[{"cvssData":{"baseScore":7.5,"baseSeverity":"HIGH"}}]}}}]}`, day(10))
		default:
			fmt.Fprint(rec, `{"vulnerabilities":[]}`)
		}
	case "www.cisa.gov":
		fmt.Fprintf(rec, `{"vulnerabilities":[{"cveID":"CVE-2026-1001","vendorProject":"Sangoma","product":"Asterisk","vulnerabilityName":"Sangoma Asterisk Crash",
"dateAdded":%q,"dueDate":"2026-12-01","knownRansomwareCampaignUse":"Unknown","requiredAction":"Apply the update.","shortDescription":"Asterisk res_pjsip crash."},
{"cveID":"CVE-2020-1","vendorProject":"Microsoft","product":"Windows","vulnerabilityName":"Windows kernel bug","dateAdded":"2020-01-01"}]}`, now.AddDate(0, 0, -2).Format("2006-01-02"))
	case "api.first.org":
		fmt.Fprint(rec, `{"data":[{"cve":"CVE-2026-1001","epss":"0.42","percentile":"0.97"},{"cve":"CVE-2026-2002","epss":"0.002","percentile":"0.4"}]}`)
	default:
		return false
	}
	return true
}

// stubInfo answers the news, market, IETF, npm and Docker Hub sources.
func stubInfo(rec *httptest.ResponseRecorder, r *http.Request, now time.Time) bool {
	rss := func(items ...string) {
		fmt.Fprintf(rec, `<?xml version="1.0"?><rss><channel>%s</channel></rss>`, strings.Join(items, ""))
	}
	item := func(title, link string, days int, source string) string {
		src := ""
		if source != "" {
			src = "<source>" + source + "</source>"
			title += " - " + source
		}
		return fmt.Sprintf(`<item><title>%s</title><link>%s</link><pubDate>%s</pubDate><description>about %s</description>%s</item>`,
			title, link, now.AddDate(0, 0, -days).Format(time.RFC1123Z), title, src)
	}
	q := r.URL.Query()
	switch r.URL.Host {
	case "blog.opensips.org":
		rss(item("OpenSIPS 3.6 brings STIR/SHAKEN improvements", "https://blog.opensips.org/1", 2, ""))
	case "news.google.com":
		switch query := q.Get("q"); {
		case strings.Contains(query, "Sangoma"):
			rss(item("Sangoma reports third quarter revenue growth", "https://n/s1", 1, "Reuters"),
				item("Sangoma third quarter revenue beats estimates", "https://n/s2", 2, "Bloomberg"))
		case strings.Contains(query, "Kamailio World"):
			rss(item("Kamailio World 2027 dates announced", "https://n/kw", 3, "Kamailio"))
		default:
			rss()
		}
	case "hn.algolia.com":
		fmt.Fprintf(rec, `{"hits":[{"objectID":"42","title":"Show HN: a voice agent on top of Asterisk","url":"https://x","created_at_i":%d,"points":120,"num_comments":40}]}`,
			now.Add(-5*time.Hour).Unix())
	case "datatracker.ietf.org":
		switch p := r.URL.Path; {
		case strings.HasSuffix(p, "/meeting/meeting/"):
			fmt.Fprintf(rec, `{"objects":[{"number":"126","city":"Vienna","country":"AT","date":%q,"days":6}]}`, now.AddDate(0, 0, 30).Format(time.DateOnly))
		case strings.HasSuffix(p, "/docevent/") && q.Get("doc__group__acronym") == "stir":
			fmt.Fprintf(rec, `{"objects":[{"doc":"/api/v1/doc/document/rfc9999/","time":%q}]}`, now.AddDate(0, 0, -10).Format(time.RFC3339))
		case q.Get("group__acronym") == "sipcore" && q.Get("type") == "draft":
			fmt.Fprintf(rec, `{"objects":[{"name":"draft-ietf-sipcore-quic","title":"SIP over QUIC","rev":"03","time":%q,"expires":%q,"pages":24,"abstract":"Carries SIP over QUIC."}]}`,
				now.AddDate(0, 0, -4).Format("2006-01-02T15:04:05"), now.AddDate(0, 5, 0).Format("2006-01-02T15:04:05"))
		case q.Get("group__acronym") == "stir" && q.Get("type") == "rfc":
			fmt.Fprint(rec, `{"objects":[{"name":"rfc9999","title":"STIR certificate delegation","rev":"","time":"2026-10-20T00:00:00","pages":30}]}`)
		default:
			fmt.Fprint(rec, `{"objects":[]}`)
		}
	case "api.nasdaq.com":
		var pts []string
		for i := 0; i < 70; i++ {
			pts = append(pts, fmt.Sprintf(`{"y":%.2f}`, 4+float64(i)/20))
		}
		fmt.Fprintf(rec, `{"data":{"chart":[%s],"timeAsOf":"Oct 20, 2026"}}`, strings.Join(pts, ","))
	case "api.npmjs.org":
		var days []string
		for i := 0; i < 30; i++ {
			days = append(days, fmt.Sprintf(`{"downloads":%d}`, 1000+i*10))
		}
		fmt.Fprintf(rec, `{"downloads":[%s]}`, strings.Join(days, ","))
	case "hub.docker.com":
		switch p := r.URL.Path; {
		case strings.HasPrefix(p, "/v2/search/"):
			fmt.Fprint(rec, `{"results":[{"repo_name":"someone/opensips-box","pull_count":500,"short_description":"OpenSIPS 9.0.1 image"},{"repo_name":"other/unrelated","pull_count":9}]}`)
		case strings.HasSuffix(p, "/tags/"):
			fmt.Fprintf(rec, `{"results":[{"name":"latest","last_updated":%q},{"name":"9.0.1","last_updated":%q,"full_size":120000000,"images":[{"architecture":"amd64"},{"architecture":"arm64"}]},{"name":"2024.01","last_updated":%q}]}`,
				now.Format(time.RFC3339), now.Format(time.RFC3339), now.Format(time.RFC3339))
		default:
			fmt.Fprintf(rec, `{"pull_count":1234567,"star_count":12,"description":"OpenSIPS","last_updated":%q}`, now.AddDate(0, 0, -3).Format(time.RFC3339))
		}
	default:
		return false
	}
	return true
}

type harness struct {
	t      *testing.T
	app    *App
	screen tcell.SimulationScreen
}

func start(t *testing.T, cmd string) *harness {
	t.Helper()
	t.Setenv("GITHUB_TOKEN", "test")
	t.Setenv("NVD_API_KEY", "")
	t.Setenv("VOIPINFO_HOME", t.TempDir()) // no legacy inventory to import
	nvd.Pause = 0
	adoption.PyPIPause = 0
	_ = i18n.Set("en")
	t.Cleanup(func() { _ = i18n.Set("en") })
	cfg, _ := config.Load(filepath.Join(t.TempDir(), "none.yaml"))
	// the tests set these variables; without settings there is no secret
	cfg.Secrets = secret.Set{GitHub: secret.Secret{Env: "GITHUB_TOKEN"}, NVD: secret.Secret{Env: "NVD_API_KEY"}}
	cfg.Apply()
	st, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	h, err := hub.New(st)
	if err != nil {
		t.Fatal(err)
	}
	stubGitHub(h)
	h.DataDir = t.TempDir()
	_ = h.LoadInventory()
	app := New(cfg, h)
	screen := tcell.NewSimulationScreen("UTF-8")
	if err := screen.Init(); err != nil {
		t.Fatal(err)
	}
	screen.SetSize(200, 50)
	app.SetScreen(screen)
	done := make(chan struct{})
	go func() { _ = app.Start(cmd); close(done) }()
	t.Cleanup(func() { app.Stop(); <-done })
	time.Sleep(50 * time.Millisecond)
	app.QueueUpdateDraw(func() { screen.SetSize(200, 50) })
	return &harness{t: t, app: app, screen: screen}
}

func (h *harness) text() string {
	var out string
	h.app.QueueUpdate(func() {
		cells, w, _ := h.screen.GetContents()
		var b strings.Builder
		for i, c := range cells {
			if len(c.Runes) > 0 {
				b.WriteRune(c.Runes[0])
			} else {
				b.WriteRune(' ')
			}
			if (i+1)%w == 0 {
				b.WriteRune('\n')
			}
		}
		out = b.String()
	})
	return out
}

func (h *harness) waitRe(patterns ...string) string {
	h.t.Helper()
	deadline := time.Now().Add(8 * time.Second)
	var txt string
	for time.Now().Before(deadline) {
		txt = h.text()
		ok := true
		for _, p := range patterns {
			if !regexp.MustCompile(p).MatchString(txt) {
				ok = false
				break
			}
		}
		if ok {
			return txt
		}
		time.Sleep(30 * time.Millisecond)
	}
	h.t.Fatalf("timeout waiting for %q; screen:\n%s", patterns, txt)
	return ""
}

func (h *harness) keys(s string) {
	for _, r := range s {
		h.screen.InjectKey(tcell.KeyRune, r, tcell.ModNone)
		time.Sleep(5 * time.Millisecond)
	}
}

func (h *harness) key(k tcell.Key) {
	h.screen.InjectKey(k, 0, tcell.ModNone)
	time.Sleep(10 * time.Millisecond)
}

func TestProductsListCategoryAndCard(t *testing.T) {
	h := start(t, "")
	// every product gets the stub releases: the latest stable one, the RC aside
	h.waitRe(`Products\[\d{2}\]`, `Asterisk\s+PBX\s+9\.0\.2\s+FIX\s+9\.1\.0-rc1`, `Updated:\s+\d\d:\d\d`)
	h.keys("1") // PBX
	txt := h.waitRe(`Products\[\d\] PBX`, `Category:\s+PBX · \d products`, `<1> PBX\s+<6> Monitoring`)
	if strings.Contains(txt, "Kamailio") {
		t.Fatalf("category filter let Kamailio through:\n%s", txt)
	}
	h.keys("0")
	h.waitRe(`Products\[\d{2}\]`)
	h.keys(":asterisk")
	h.key(tcell.KeyEnter)
	h.waitRe(`<asterisk>`, `1 Releases`, `9\.0\.1`, `Classic open-source PBX and telephony platform`)
	h.keys("j") // 9.0.2: its notes on the right
	h.waitRe(`Fixes`, `crash in parser`)
	// enter opens the release on the whole screen, esc comes back and the list moves again
	h.key(tcell.KeyEnter)
	h.waitRe(`<Asterisk 9\.0\.2>`, `crash in parser`)
	h.key(tcell.KeyEscape)
	h.waitRe(`<asterisk>`)
	h.keys("j")
	h.waitRe(`Release 9\.0\.1`)
}

func TestReleasesFeedAndLanguage(t *testing.T) {
	h := start(t, "releases")
	h.waitRe(`Releases\[\d{3}\]`, `9\.1\.0-rc1\s+9\.1\s+pre`)
	h.keys(":lang ru")
	h.key(tcell.KeyEnter)
	h.waitRe(`Релизы\[\d{3}\]`, `ПРОДУКТ`, `Категория:\s+все · продуктов: \d+`, `Обновлено:\s+\S+ · ru · dev`, `<0> все\s+<5> SIP Stack`)
	// the descriptions of the catalog follow the language too
	h.keys(":asterisk")
	h.key(tcell.KeyEnter)
	h.waitRe(`Классическая open-source PBX`)
}

func TestMetricsViewsAndCardTabs(t *testing.T) {
	h := start(t, "")
	// with a token the table gets the API metrics: stars, the commit trend, the health index
	h.waitRe(`Asterisk\s+PBX\s+9\.0\.2.*1\.2k\s+[▁-█]{8} \+\d+%\s+\d+`, `API:\s+4990/5000`, `Tokens:\s+GitHub yes · NVD none`)
	h.keys(":health")
	h.key(tcell.KeyEnter)
	h.waitRe(`Health\[\d{2}\]`, `RELEASE FRESHNESS`, `█+░* +\d+`)
	h.keys(":eol")
	h.key(tcell.KeyEnter)
	h.waitRe(`Support\[\d+\]`, `Kamailio\s+9\.0\s+-\s+9\.0\.2`, `policy`)
	h.keys(":deps")
	h.key(tcell.KeyEnter)
	h.waitRe(`Dependencies\[\d+\]`, `Asterisk\s+→\s+PJSIP\s+bundled`)
	h.keys(":asterisk")
	h.key(tcell.KeyEnter)
	h.waitRe(`stars: 1\.2k`, `health: \d+ [A-F]`)
	h.keys("2")
	h.waitRe(`issues \+2 / closed 1`, `merged 1`, `top author: @bob 100%`, `leak in parser`)
	// enter opens the issue with its description; esc comes back to the card
	h.key(tcell.KeyEnter)
	h.waitRe(`issue #103: leak in parser`, `labels\s+bug`, `The parser leaks on reload\.`)
	h.key(tcell.KeyEscape)
	h.waitRe(`<asterisk>`)
	h.keys("3")
	// the stub gives Asterisk 9.x: a stable branch 9 (not "upcoming"), x.0.0 date unknown
	h.waitRe(`Branch support`, `9\s+standard\s+9\.0\.2`, `9\.1\s+█+░+ 7/10`)
	h.keys("4")
	h.waitRe(`Health index: \d+ [A-F]`, `Development pace\s+20\s+█+`, `Community\s+10\s+█░+\s+15\s+1 PR authors`)
	h.keys("G") // the commits are below the fold
	h.waitRe(`Commits in 52 weeks`, `total \d+ · last 12 weeks 192`)
	h.keys("5")
	// FreePBX, Wazo and IssabelPBX inherit (platform); Fonoster and the testsuite run alongside (runtime)
	h.waitRe(`Depends on`, `├─ PJSIP \(bundled\)`, `Depend on it \(impact radius: 3 over strong links\)`, `Fonoster \(runtime\)`)
}

func TestSecurityViewsInventoryAndUpgrade(t *testing.T) {
	h := start(t, "")
	// the CVE columns of the table and the info line fill in once NVD is read;
	// the PJSIP CVE is waiting for an Asterisk release mentioning it
	h.waitRe(`Asterisk\s+PBX\s+9\.0\.2\s+FIX.*\s1\s+1 ✗\s+\d+`, `CVE/30d:\s+2 \(high\+ 2\) · KEV 1`)
	h.keys(":security")
	h.key(tcell.KeyEnter)
	h.waitRe(`Security\[2\]`, `critical\s+9\.1\s+42\.0%\s+yes\s+Asterisk\s+\+3\s+CVE-2026-1001`, `high\s+7\.5\s+0\.2%\s+PJSIP`)
	h.key(tcell.KeyEnter)
	h.waitRe(`CVE-2026-1001 · Asterisk`, `in CISA KEV since`, `>= 9\.0\.0, < 9\.0\.2`, `Inherited through dependencies`)
	h.key(tcell.KeyEscape)
	h.keys(":kev")
	h.key(tcell.KeyEnter)
	h.waitRe(`Exploited \(KEV\)\[1\]`, `CVE-2026-1001\s+Sangoma\s+Asterisk`)
	h.keys(":have asterisk 9.0.1 prod")
	h.key(tcell.KeyEnter)
	h.keys(":mine")
	h.key(tcell.KeyEnter)
	h.waitRe(`My versions\[1\]`, `Asterisk\s+9\.0\.1\s+prod\s+9\.0\s+\S+\s+9\.0\.2\s+\+1\s+1\s+1\s+1\s+-1\s+0\s+1\s+URGENT`, `URGENT: 1 exploited vulnerabilities \(KEV\) → 9\.0\.2`)
	// enter: the summary over the vulnerabilities of my version, a row each
	h.key(tcell.KeyEnter)
	h.waitRe(`Asterisk 9\.0\.1`, `recommendation\s+URGENT`, `Vulnerabilities\[2\]`, `CVE-2026-1001\s+fixed in 9\.0\.2`)
	h.key(tcell.KeyEnter)
	h.waitRe(`CVE-2026-1001 · Asterisk`)
	h.key(tcell.KeyEscape)
	h.key(tcell.KeyEscape)
	h.waitRe(`My versions\[1\]`)
	h.keys("u")
	h.waitRe(`Asterisk: 9\.0\.1 → 9\.0\.2`, `closes 1 CVEs \(high/critical: 1\), remain 0`, `9\.0\.2\s+\d{4}-\d\d-\d\d\s+FIX`)
	h.key(tcell.KeyEscape)
	h.keys(":asterisk")
	h.key(tcell.KeyEnter)
	h.keys("6")
	// PJSIP is bundled: the Asterisk release after the CVE does not mention it
	h.waitRe(`Vulnerabilities: 1, in the last year 1 \(high\+ 1\), exploited \(KEV\) 1`, `My version 9\.0\.1`,
		`CVE-2026-2002\s+PJSIP\s+\? released, not mentioned`)
	// the list: enter opens a vulnerability, esc comes back
	h.waitRe(`Vulnerabilities\[2\]`, `CVE-2026-1001\s+own`)
	h.key(tcell.KeyEnter)
	h.waitRe(`CVE-2026-1001 · Asterisk`, `in CISA KEV since`)
	h.key(tcell.KeyEscape)
	h.keys("j")
	h.key(tcell.KeyEnter)
	h.waitRe(`CVE-2026-2002 · PJSIP`)
}

func TestNewsMarketStandardsAndDocker(t *testing.T) {
	h := start(t, "")
	h.waitRe(`news 24h: [1-9]`)
	h.keys(":news")
	h.key(tcell.KeyEnter)
	h.waitRe(`News\[\d+\]`, `project\s+OpenSIPS blog\s+OpenSIPS 3\.6 brings STIR/SHAKEN improvements\s+OpenSIPS`,
		`community\s+HN\s+Show HN: a voice agent on top of Asterisk\s+Asterisk\s+120/40`)
	// the kinds of feeds on 1-6 instead of the categories
	h.waitRe(`<0> all\s+<5> community`, `<1> project\s+<6> lists`)
	h.keys("1")
	h.waitRe(`News\[1\] project`)
	h.keys("0")
	h.waitRe(`News\[\d+\] `)
	h.keys(":news community")
	h.key(tcell.KeyEnter)
	h.waitRe(`News\[1\] community`)
	h.key(tcell.KeyEnter)
	h.waitRe(`Show HN: a voice agent`, `120 points, 40 comments`, `products\s+Asterisk`)
	h.key(tcell.KeyEscape)
	h.keys(":radar")
	h.key(tcell.KeyEnter)
	h.waitRe(`Radar\[19\]`, `STIR/SHAKEN, caller ID\s+1\s+[1-9]`, `Voice AI / LLM agents\s+1\s+1\s+`)
	// a topic: w marks it as watched, enter lists its mentions, a row each
	h.keys("w")
	h.waitRe(`watching STIR/SHAKEN, caller ID`, `◉ STIR/SHAKEN, caller ID\s+1`)
	h.key(tcell.KeyEnter)
	h.waitRe(`my stack\s+◉ subscription`, `Mentions\[[1-9]\]`, `news\s+OpenSIPS 3\.6 brings STIR/SHAKEN improvements\s+OpenSIPS`)
	h.key(tcell.KeyEscape)
	h.keys(":ietf")
	h.key(tcell.KeyEnter)
	h.waitRe(`IETF\[2\]`, `RFC\s+stir\s+RFC 9999\s+STIR certificate delegation\s+30`, `draft\s+sipcore\s+draft-ietf-sipcore-quic-03\s+SIP over QUIC\s+24`)
	h.keys(":events")
	h.key(tcell.KeyEnter)
	h.waitRe(`Events\[\d+\]`, `IETF 126\s+Vienna, AT\s+datatracker`, `\?\s+Kamailio World\s+Berlin\s+varies\s+Kamailio World 2027 dates announced`)
	h.keys(":market")
	h.key(tcell.KeyEnter)
	h.waitRe(`Market\[\d+\]`, `Sangoma\s+PBX/UCaaS\s+SANG\s+\$7\.45\s+\+\d+%\s+\+\d+%\s+[▁-█]+\s+1\s+Sangoma reports third quarter`)
	// the packages of a product: one line of the Stats tab
	h.keys(":janus")
	h.key(tcell.KeyEnter)
	h.keys("4")
	h.waitRe(`Usage: npm janus-gateway: 34k/month, week \+6%`)
	h.key(tcell.KeyEscape)
	h.keys(":opensips")
	h.key(tcell.KeyEnter)
	h.keys("7")
	h.waitRe(`OpenSIPS blog\s+OpenSIPS 3\.6 brings`, `about OpenSIPS 3\.6`)
	h.keys("4")
	h.waitRe(`Usage: docker opensips/opensips: 1\.2M pulls`)
	h.keys("8")
	// the catalog image first, the community one found by the search; 9.0.1 is one behind 9.0.2
	h.waitRe(`opensips/opensips\s+1\.2M\s+12\s+3d\s+9\.0\.1\s+-1\s+x86,arm`, `someone/opensips-box`, `1 behind in branch 9\.0: the latest is 9\.0\.2`)
}

func TestStackAndDigest(t *testing.T) {
	h := start(t, "")
	h.waitRe(`Asterisk\s+PBX\s+9\.0\.2`, `news 24h: [1-9]`)
	// w on Asterisk: the subscription brings PJSIP, Opus and SpanDSP in as dependencies; W shows only them
	h.keys(":asterisk")
	h.key(tcell.KeyEnter)
	h.key(tcell.KeyEscape)
	h.keys("w")
	h.waitRe(`watching Asterisk`, `◉ Asterisk`)
	h.keys("W")
	txt := h.waitRe(`Products\[4\] my stack`, `◉ PJSIP`)
	if strings.Contains(txt, "Kamailio") {
		t.Fatalf("the stack filter let Kamailio through:\n%s", txt)
	}
	h.keys(":watch")
	h.key(tcell.KeyEnter)
	h.waitRe(`My stack\[4\] my stack`, `product\s+Asterisk\s+subscription\s+9\.0\.2`, `product\s+PJSIP\s+dependency of Asterisk`)
	h.keys(":watch stir/shaken")
	h.key(tcell.KeyEnter)
	h.waitRe(`watching STIR/SHAKEN, caller ID`, `topic\s+STIR/SHAKEN, caller ID\s+subscription`)
	h.keys("W")
	h.waitRe(`My stack\[5\] `)
	h.keys(":digest 7")
	h.key(tcell.KeyEnter)
	h.waitRe(`VoIP digest · 7 days`, `TL;DR`, `My stack \(4 products with their dependencies\)`, `Asterisk.*CVE-2026-1001`)
	h.keys("e")
	h.waitRe(`saved .*exports/digest-\d{4}-\d\d-\d\d_\d{4}\.md`)
	h.key(tcell.KeyEscape)
	files, _ := filepath.Glob(filepath.Join(h.app.hub.DataDir, "exports", "digest-*.md"))
	if len(files) != 1 {
		t.Errorf("exports %v", files)
	} else if b, _ := os.ReadFile(files[0]); !strings.HasPrefix(string(b), "# VoIP digest") {
		t.Errorf("%.100s", b)
	}
}
