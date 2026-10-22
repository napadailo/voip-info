// Package ietf follows the standards of VoIP and RTC: drafts and RFCs of
// the IETF working groups and the IETF meetings (the datatracker API).
package ietf

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/napadailo/voip-info/internal/fetch"
)

// API of the datatracker.
var API = "https://datatracker.ietf.org/api/v1"

// N marks a message for the catalog; the UI translates it.
func N(s string) string { return s }

// WGs are the working groups followed, with what they do.
var WGs = []struct{ Name, Desc string }{
	{"sipcore", N("SIP core")},
	{"stir", N("STIR/SHAKEN: caller identity")},
	{"mmusic", N("SDP, ICE, multimedia sessions")},
	{"avtcore", N("RTP/RTCP, payload formats")},
	{"moq", N("Media over QUIC")},
	{"wish", N("WHIP/WHEP: WebRTC ingest/egress")},
	{"ecrit", N("emergency calls (E911/112)")},
	{"webtrans", N("WebTransport")},
}

// WGDesc is what a working group does.
func WGDesc(wg string) string {
	for _, w := range WGs {
		if w.Name == wg {
			return w.Desc
		}
	}
	return ""
}

// Doc is a draft or an RFC.
type Doc struct {
	Name, Title, WG, Kind, Rev string    // Kind: draft or rfc
	Time                       time.Time // the last revision, or the publication of an RFC
	Expires                    time.Time
	Pages                      int
	Abstract                   string
}

// URL of the document on the datatracker.
func (d *Doc) URL() string { return "https://datatracker.ietf.org/doc/" + d.Name + "/" }

// AgeDays since the time of the document.
func (d *Doc) AgeDays() int { return int(time.Since(d.Time).Hours() / 24) }

// Label is "RFC 9999" or "draft-ietf-x-y-03".
func (d *Doc) Label() string {
	if d.Kind == "rfc" {
		return strings.Replace(strings.ToUpper(d.Name), "RFC", "RFC ", 1)
	}
	return d.Name + "-" + d.Rev
}

// Meeting is an IETF meeting.
type Meeting struct {
	Number, City, Country string
	Date                  time.Time
	Days                  int
}

// URL of the meeting agenda.
func (m *Meeting) URL() string { return "https://datatracker.ietf.org/meeting/" + m.Number + "/" }

func parseTime(s string) time.Time {
	for _, l := range []string{time.RFC3339, "2006-01-02T15:04:05", time.DateOnly} {
		if t, err := time.Parse(l, s); err == nil {
			return t
		}
	}
	return time.Time{}
}

func getJSON(ctx context.Context, f *fetch.Fetcher, u string, ttl time.Duration, force bool, v any) error {
	res := f.Get(ctx, u, fetch.Options{TTL: ttl, Force: force})
	if res.Body == nil {
		return res.Err
	}
	return json.Unmarshal(res.Body, v)
}

func wgDocs(ctx context.Context, f *fetch.Fetcher, wg, kind, since string, force bool) ([]*Doc, error) {
	var data struct {
		Objects []struct {
			Name, Title, Rev, Time, Expires, Abstract string
			Pages                                     int
		} `json:"objects"`
	}
	u := fmt.Sprintf("%s/doc/document/?group__acronym=%s&type=%s&time__gte=%s&limit=100&format=json", API, wg, kind, since)
	if err := getJSON(ctx, f, u, 12*time.Hour, force, &data); err != nil {
		return nil, err
	}
	var out []*Doc
	for _, o := range data.Objects {
		out = append(out, &Doc{Name: o.Name, Title: o.Title, WG: wg, Kind: kind, Rev: o.Rev, Time: parseTime(o.Time),
			Expires: parseTime(o.Expires), Pages: o.Pages, Abstract: strings.Join(strings.Fields(o.Abstract), " ")})
	}
	return out, nil
}

// rfcPublished: the time of an RFC document is its last change, the
// publication is a document event.
func rfcPublished(ctx context.Context, f *fetch.Fetcher, wg, since string, force bool) (map[string]time.Time, error) {
	var data struct {
		Objects []struct{ Doc, Time string } `json:"objects"`
	}
	u := fmt.Sprintf("%s/doc/docevent/?type=published_rfc&doc__group__acronym=%s&time__gte=%s&limit=100&format=json", API, wg, since)
	if err := getJSON(ctx, f, u, 12*time.Hour, force, &data); err != nil {
		return nil, err
	}
	out := map[string]time.Time{}
	for _, o := range data.Objects {
		name := strings.TrimRight(o.Doc, "/")
		name = name[strings.LastIndex(name, "/")+1:]
		out[name] = parseTime(o.Time)
	}
	return out, nil
}

// FetchDocs reads the drafts of a year and the RFCs of two years, newest first.
func FetchDocs(ctx context.Context, f *fetch.Fetcher, force bool) ([]*Doc, error) {
	now := time.Now().UTC()
	sinceDraft, sinceRFC := now.AddDate(-1, 0, 0).Format(time.DateOnly), now.AddDate(-2, 0, 0).Format(time.DateOnly)
	var (
		mu      sync.Mutex
		wg      sync.WaitGroup
		docs    []*Doc
		rfcs    []*Doc
		pub     = map[string]time.Time{}
		lastErr error
	)
	for _, g := range WGs {
		wg.Add(3)
		go func() {
			defer wg.Done()
			d, err := wgDocs(ctx, f, g.Name, "draft", sinceDraft, force)
			mu.Lock()
			docs, lastErr = append(docs, d...), firstErr(lastErr, err)
			mu.Unlock()
		}()
		go func() {
			defer wg.Done()
			d, err := wgDocs(ctx, f, g.Name, "rfc", sinceRFC, force)
			mu.Lock()
			rfcs, lastErr = append(rfcs, d...), firstErr(lastErr, err)
			mu.Unlock()
		}()
		go func() {
			defer wg.Done()
			p, err := rfcPublished(ctx, f, g.Name, sinceRFC, force)
			mu.Lock()
			for k, v := range p {
				pub[k] = v
			}
			lastErr = firstErr(lastErr, err)
			mu.Unlock()
		}()
	}
	wg.Wait()
	for _, d := range rfcs {
		if t, ok := pub[d.Name]; ok {
			d.Time = t
			docs = append(docs, d)
		}
	}
	sort.SliceStable(docs, func(i, j int) bool { return docs[i].Time.After(docs[j].Time) })
	if len(docs) > 0 {
		lastErr = nil
	}
	return docs, lastErr
}

func firstErr(a, b error) error {
	if a != nil {
		return a
	}
	return b
}

// FetchMeetings reads the coming IETF meetings.
func FetchMeetings(ctx context.Context, f *fetch.Fetcher, force bool) ([]*Meeting, error) {
	var data struct {
		Objects []struct {
			Number        json.Number `json:"number"`
			City, Country string
			Date          string
			Days          int
		} `json:"objects"`
	}
	since := time.Now().UTC().AddDate(0, 0, -7).Format(time.DateOnly)
	u := fmt.Sprintf("%s/meeting/meeting/?type=ietf&date__gte=%s&format=json&limit=10", API, since)
	if err := getJSON(ctx, f, u, 7*24*time.Hour, force, &data); err != nil {
		return nil, err
	}
	var out []*Meeting
	for _, o := range data.Objects {
		d := parseTime(o.Date)
		if d.IsZero() {
			continue
		}
		days := o.Days
		if days == 0 {
			days = 7
		}
		out = append(out, &Meeting{Number: o.Number.String(), City: o.City, Country: o.Country, Date: d, Days: days})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Date.Before(out[j].Date) })
	return out, nil
}
