// Package news reads the news sources: RSS and Atom feeds (Google News
// among them), Hacker News (Algolia) and Mailman 2 list archives; it tags
// items with the products they mention and folds headlines about one event.
package news

import (
	"bytes"
	"context"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"net/url"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/napadailo/voip-info/internal/catalog"
	"github.com/napadailo/voip-info/internal/fetch"
	"github.com/napadailo/voip-info/internal/htmltext"
)

// Kinds of feeds, in display order.
var Kinds = []string{"project", "industry", "market", "regional", "community", "lists"}

// Item is a news item, a forum thread or a list thread.
type Item struct {
	Source   string
	Kind     string
	Title    string
	URL      string
	Date     time.Time
	Summary  string // plain text
	Score    int    // HN points
	Comments int    // HN comments, replies in a thread
	Products []string
}

// AgeDays since publication.
func (n *Item) AgeDays() int { return int(time.Since(n.Date).Hours() / 24) }

// Key identifies an item.
func (n *Item) Key() string { return n.URL }

// Has reports whether the item is tagged with a product.
func (n *Item) Has(id string) bool {
	for _, p := range n.Products {
		if p == id {
			return true
		}
	}
	return false
}

// TTL of the news sources.
const TTL = 3 * time.Hour

type rssItem struct {
	Title       string `xml:"title"`
	Link        string `xml:"link"`
	PubDate     string `xml:"pubDate"`
	Date        string `xml:"date"` // dc:date (RSS 1.0)
	Description string `xml:"description"`
	Source      string `xml:"source"`
}

type atomEntry struct {
	Title string `xml:"title"`
	Links []struct {
		Href string `xml:"href,attr"`
		Rel  string `xml:"rel,attr"`
	} `xml:"link"`
	Updated   string `xml:"updated"`
	Published string `xml:"published"`
	Summary   string `xml:"summary"`
	Content   string `xml:"content"`
}

type feedDoc struct {
	Channel struct {
		Items []rssItem `xml:"item"`
	} `xml:"channel"`
	Items   []rssItem   `xml:"item"` // RSS 1.0 keeps items at the root
	Entries []atomEntry `xml:"entry"`
}

var layouts = []string{time.RFC1123Z, time.RFC1123, "Mon, 2 Jan 2006 15:04:05 -0700", "Mon, 2 Jan 2006 15:04:05 MST",
	"2 Jan 2006 15:04:05 -0700", time.RFC3339, "2006-01-02T15:04:05Z0700", "2006-01-02T15:04:05", "2006-01-02"}

// ParseDate reads the dates of RSS (RFC 822) and Atom (ISO 8601).
func ParseDate(s string) (time.Time, bool) {
	s = strings.TrimSpace(s)
	for _, l := range layouts {
		if t, err := time.Parse(l, s); err == nil {
			return t, true
		}
	}
	return time.Time{}, false
}

func summary(html string) string {
	s := strings.TrimSpace(htmltext.Text(html))
	if r := []rune(s); len(r) > 1500 {
		s = string(r[:1500])
	}
	return s
}

// Parse reads an RSS or Atom feed.
func Parse(feed catalog.Feed, body []byte) ([]*Item, error) {
	var doc feedDoc
	d := xml.NewDecoder(bytes.NewReader(bytes.TrimSpace(body)))
	d.Strict = false // no HTML auto-closing: <link> and <source> are elements of RSS
	d.Entity = xml.HTMLEntity
	if err := d.Decode(&doc); err != nil {
		return nil, fmt.Errorf("%s: %w", feed.Name, err)
	}
	now := time.Now().UTC()
	var out []*Item
	for _, it := range append(doc.Channel.Items, doc.Items...) {
		title, src := strings.TrimSpace(it.Title), strings.TrimSpace(it.Source)
		n := &Item{Source: feed.Name, Kind: feed.Kind, URL: strings.TrimSpace(it.Link), Date: now}
		if feed.Kind == "market" && src != "" {
			title = strings.TrimSuffix(title, " - "+src) // Google News: "Title - Source"
			n.Source = feed.Name + " · " + src
		} else {
			n.Summary = summary(it.Description)
		}
		n.Title = title
		if t, ok := ParseDate(it.PubDate); ok {
			n.Date = t
		} else if t, ok := ParseDate(it.Date); ok {
			n.Date = t
		}
		out = append(out, n)
	}
	for _, e := range doc.Entries {
		n := &Item{Source: feed.Name, Kind: feed.Kind, Title: strings.TrimSpace(e.Title), Date: now}
		for _, l := range e.Links {
			if n.URL == "" || l.Rel == "alternate" {
				n.URL = l.Href
			}
		}
		if t, ok := ParseDate(e.Updated); ok {
			n.Date = t
		} else if t, ok := ParseDate(e.Published); ok {
			n.Date = t
		}
		body := e.Summary
		if body == "" {
			body = e.Content
		}
		n.Summary = summary(body)
		out = append(out, n)
	}
	for _, n := range out {
		if feed.Product != "" {
			n.Products = append(n.Products, feed.Product)
		}
		// event listings put the date of the event in the feed
		if n.Date.After(now.Add(time.Hour)) {
			n.Date = now
		}
	}
	if feed.Filter != "" {
		re, err := regexp.Compile(feed.Filter)
		if err != nil {
			return nil, fmt.Errorf("%s: filter: %w", feed.Name, err)
		}
		kept := out[:0]
		for _, n := range out {
			if re.MatchString(n.Title) {
				kept = append(kept, n)
			}
		}
		out = kept
	}
	if feed.Kind == "lists" {
		out = GroupThreads(out)
	}
	return out, nil
}

// Fetch reads a feed of any type.
func Fetch(ctx context.Context, f *fetch.Fetcher, feed catalog.Feed, force bool) ([]*Item, error) {
	switch feed.Type {
	case "hn":
		return FetchHN(ctx, f, feed, force, 30)
	case "pipermail":
		return FetchPipermail(ctx, f, feed, force)
	}
	res := f.Get(ctx, feed.URL, fetch.Options{TTL: TTL, Force: force})
	if res.Body == nil {
		return nil, fmt.Errorf("%s: %w", feed.Name, res.Err)
	}
	return Parse(feed, res.Body)
}

// GoogleNews is a feed of the Google News search for query (English, US).
func GoogleNews(name, query string) catalog.Feed {
	return catalog.Feed{Name: "GN: " + name, Kind: "market",
		URL: "https://news.google.com/rss/search?q=" + url.QueryEscape(query) + "&hl=en-US&gl=US&ceid=US:en"}
}

// HNAPI is the Algolia search of Hacker News.
var HNAPI = "https://hn.algolia.com/api/v1/search"

// FetchHN searches story titles for each "|"-separated query of the feed.
func FetchHN(ctx context.Context, f *fetch.Fetcher, feed catalog.Feed, force bool, days int) ([]*Item, error) {
	since := time.Now().Add(-time.Duration(days) * 24 * time.Hour).Unix()
	since -= since % 86400 // the same URL all day: the cache works
	seen := map[string]bool{}
	var out []*Item
	var lastErr error
	for _, q := range strings.Split(feed.URL, "|") {
		q = strings.TrimSpace(q)
		// ">" must be escaped: the Algolia front end answers 400 otherwise
		u := fmt.Sprintf("%s?tags=story&hitsPerPage=30&typoTolerance=false&restrictSearchableAttributes=title&query=%s&numericFilters=%s",
			HNAPI, url.QueryEscape(q), url.QueryEscape(fmt.Sprintf("created_at_i>%d,points>5", since)))
		res := f.Get(ctx, u, fetch.Options{TTL: TTL, Force: force})
		if res.Body == nil {
			lastErr = res.Err
			continue
		}
		var data struct {
			Hits []struct {
				ID       string `json:"objectID"`
				Title    string `json:"title"`
				URL      string `json:"url"`
				Created  int64  `json:"created_at_i"`
				Points   int    `json:"points"`
				Comments int    `json:"num_comments"`
			} `json:"hits"`
		}
		if err := json.Unmarshal(res.Body, &data); err != nil {
			lastErr = err
			continue
		}
		term := regexp.MustCompile(`(?i)\b` + regexp.QuoteMeta(q))
		for _, h := range data.Hits {
			link := "https://news.ycombinator.com/item?id=" + h.ID
			if seen[link] || !term.MatchString(h.Title) {
				continue
			}
			seen[link] = true
			target := h.URL
			if target == "" {
				target = link
			}
			out = append(out, &Item{Source: "HN", Kind: "community", Title: h.Title, URL: link,
				Date: time.Unix(h.Created, 0).UTC(), Summary: target, Score: h.Points, Comments: h.Comments})
		}
	}
	if len(out) == 0 && lastErr != nil {
		return nil, fmt.Errorf("%s: %w", feed.Name, lastErr)
	}
	return out, nil
}

var rePrefix = regexp.MustCompile(`(?i)^\s*((re|fwd?|aw|sv)\s*:\s*|\[[^\]]+\]\s*)+`)

// ThreadKey is the subject without "Re:", "Fwd:" and "[list]" prefixes.
func ThreadKey(subject string) string {
	return strings.ToLower(strings.Join(strings.Fields(rePrefix.ReplaceAllString(subject, "")), " "))
}

// GroupThreads turns messages into threads: the latest activity and the number of replies.
func GroupThreads(items []*Item) []*Item {
	sorted := append([]*Item(nil), items...)
	sort.SliceStable(sorted, func(i, j int) bool { return sorted[i].Date.Before(sorted[j].Date) })
	byKey := map[string]*Item{}
	var out []*Item
	for _, n := range sorted {
		key := ThreadKey(n.Title)
		if t := byKey[key]; t != nil {
			t.Comments++
			if n.Date.After(t.Date) {
				t.Date = n.Date
			}
			continue
		}
		if clean := strings.TrimSpace(rePrefix.ReplaceAllString(n.Title, "")); clean != "" {
			n.Title = clean
		}
		byKey[key] = n
		out = append(out, n)
	}
	return out
}

// TitleHas keeps the items whose title has the word: a Google News search
// matches the article text too, and "Avaya" brings in a flag football player.
func TitleHas(items []*Item, word string) []*Item {
	if word == "" {
		return items
	}
	re := regexp.MustCompile(`(?i)(^|[^\pL\pN])` + regexp.QuoteMeta(word) + `($|[^\pL\pN])`)
	var out []*Item
	for _, n := range items {
		if re.MatchString(n.Title) {
			out = append(out, n)
		}
	}
	return out
}

// FirstWord of a name: "Vonage (Ericsson)" is Vonage, "RTC.ON" stays.
func FirstWord(name string) string {
	w, _, _ := strings.Cut(strings.TrimSpace(name), " ")
	return w
}

// Sort orders items newest first.
func Sort(items []*Item) {
	sort.SliceStable(items, func(i, j int) bool { return items[i].Date.After(items[j].Date) })
}
