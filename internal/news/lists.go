package news

import (
	"context"
	"fmt"
	"io"
	"mime"
	"net/mail"
	"regexp"
	"strings"
	"time"

	"github.com/napadailo/voip-info/internal/catalog"
	"github.com/napadailo/voip-info/internal/fetch"
)

// "From user at host  Mon Oct  6 10:00:00 2026" opens a message of a pipermail mbox
var mboxFrom = regexp.MustCompile(`(?m)^From \S+ at \S+\s+\w{3} \w{3}\s+\d+ [\d:]+ \d{4}$`)

// ParseMbox turns the messages of a monthly Mailman 2 archive into threads:
// the first message, the number of replies and the latest reply.
func ParseMbox(feed catalog.Feed, text, base, month string) []*Item {
	starts := mboxFrom.FindAllStringIndex(text, -1)
	byKey := map[string]*Item{}
	var out []*Item
	dec := new(mime.WordDecoder)
	for i, s := range starts {
		end := len(text)
		if i+1 < len(starts) {
			end = starts[i+1][0]
		}
		chunk := text[s[1]:end]
		msg, err := mail.ReadMessage(strings.NewReader(strings.TrimLeft(chunk, "\n")))
		if err != nil {
			continue
		}
		subj := msg.Header.Get("Subject")
		if d, err := dec.DecodeHeader(subj); err == nil {
			subj = d
		}
		subj = strings.Join(strings.Fields(subj), " ")
		date, err := msg.Header.Date()
		if err != nil {
			continue
		}
		from := msg.Header.Get("From")
		if i := strings.LastIndex(from, "("); i >= 0 {
			from = strings.TrimSuffix(from[i+1:], ")")
		}
		key := ThreadKey(subj)
		if key == "" {
			continue
		}
		if t := byKey[key]; t != nil {
			t.Comments++
			t.Summary, _, _ = strings.Cut(t.Summary, "\n\nLast reply: ")
			t.Summary += fmt.Sprintf("\n\nLast reply: %s, %s", from, date.Format("2006-01-02"))
			if date.After(t.Date) {
				t.Date = date
			}
			continue
		}
		body, _ := io.ReadAll(io.LimitReader(msg.Body, 1200))
		n := &Item{Source: feed.Name, Kind: feed.Kind, Title: strings.TrimSpace(rePrefix.ReplaceAllString(subj, "")),
			URL: base + month + "/thread.html", Date: date, Summary: from + "\n\n" + strings.TrimSpace(string(body))}
		if feed.Product != "" {
			n.Products = []string{feed.Product}
		}
		byKey[key] = n
		out = append(out, n)
	}
	return out
}

// FetchPipermail reads the current and the previous month of a Mailman 2
// archive; feed.URL is its base, like https://lists.opensips.org/pipermail/users/.
func FetchPipermail(ctx context.Context, f *fetch.Fetcher, feed catalog.Feed, force bool) ([]*Item, error) {
	now := time.Now().UTC()
	prev := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC).AddDate(0, 0, -1)
	var out []*Item
	var errs []error
	for _, d := range []time.Time{prev, now} {
		month := d.Format("2006-January")
		res := f.Get(ctx, feed.URL+month+".txt", fetch.Options{TTL: TTL, Force: force})
		if res.Body == nil {
			errs = append(errs, res.Err) // 404: no archive for the month yet
			continue
		}
		out = append(out, ParseMbox(feed, string(res.Body), feed.URL, month)...)
	}
	if len(errs) == 2 {
		return nil, fmt.Errorf("%s: %w", feed.Name, errs[0])
	}
	return out, nil
}

// overrides are the patterns of short or ambiguous product names; "" = never tag.
var overrides = map[string]string{
	"opus":           `(?i)\bopus (codec|1\.\d)`,
	"libre":          "",
	"homer":          `(?i)\bhomer (sip|capture|11)|\bsipcapture`,
	"ten":            `\bTEN (framework|agent)`,
	"pion":           `(?i)\bpion\b`,
	"sems":           `\bSEMS\b`,
	"linphone":       `(?i)\blinphone`,
	"jvb":            `(?i)videobridge`,
	"jitsi-meet":     `(?i)\bjitsi\b`,
	"livekit-agents": `(?i)livekit agents`,
	"asterisk":       `(?i)\basterisk\b($|[^ ]| [^*])`, // not "asterisk *"
	"wazo":           `(?i)\bwazo\b`,
	"routr":          `(?i)\broutr\b`,
}

// Matcher finds a product in a text.
type Matcher struct {
	ID string
	Re *regexp.Regexp
}

// Matchers of the products: news_match of the catalog, the overrides, the name.
func Matchers(products []*catalog.Product) []Matcher {
	var out []Matcher
	for _, p := range products {
		pat, ok := overrides[p.ID]
		if !ok {
			pat = `(?i)\b` + regexp.QuoteMeta(p.Name) + `\b`
		}
		if p.NewsMatch != nil {
			pat = *p.NewsMatch
			if pat != "" && !strings.HasPrefix(pat, "(?") {
				pat = "(?i)" + pat
			}
		}
		if pat == "" {
			continue
		}
		if re, err := regexp.Compile(pat); err == nil {
			out = append(out, Matcher{p.ID, re})
		}
	}
	return out
}

// Tag adds the products the title or the start of the summary mentions.
func Tag(items []*Item, ms []Matcher) {
	for _, n := range items {
		s := n.Summary
		if r := []rune(s); len(r) > 400 {
			s = string(r[:400])
		}
		text := n.Title + " " + s
		for _, m := range ms {
			if !n.Has(m.ID) && m.Re.MatchString(text) {
				n.Products = append(n.Products, m.ID)
			}
		}
	}
}

var (
	wordRe = regexp.MustCompile(`[a-zа-яё0-9]+`)
	stop   = map[string]bool{"the": true, "and": true, "for": true, "with": true, "from": true, "that": true, "this": true,
		"your": true, "how": true, "why": true, "what": true, "new": true, "best": true, "voip": true, "2026": true, "2025": true,
		"2027": true, "review": true, "services": true, "service": true, "business": true, "launches": true, "announces": true, "its": true}
)

func words(title string) map[string]bool {
	out := map[string]bool{}
	for _, w := range wordRe.FindAllString(strings.ToLower(title), -1) {
		if len([]rune(w)) >= 3 && !stop[w] {
			out[w] = true
		}
	}
	return out
}

// Group is a headline and the number of similar ones folded into it.
type Group struct {
	Item    *Item
	Similar int
}

// Dedupe folds headlines about one event: 3 or more common significant
// words making at least 35% of the shorter title.
func Dedupe(items []*Item) []Group {
	type acc struct {
		g     Group
		words map[string]bool
	}
	var out []acc
	for _, n := range items {
		w := words(n.Title)
		merged := false
		for i := range out {
			common := 0
			for x := range w {
				if out[i].words[x] {
					common++
				}
			}
			if common >= 3 && float64(common)/float64(min(len(w), len(out[i].words))) >= 0.35 {
				for x := range w {
					out[i].words[x] = true
				}
				out[i].g.Similar++
				merged = true
				break
			}
		}
		if !merged {
			out = append(out, acc{Group{n, 0}, w})
		}
	}
	gs := make([]Group, len(out))
	for i, a := range out {
		gs[i] = a.g
	}
	return gs
}
