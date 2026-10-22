// Package market follows the commercial segment: stock quotes of the
// vendors (the Nasdaq API) and their news (Google News).
package market

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/napadailo/voip-info/internal/catalog"
	"github.com/napadailo/voip-info/internal/fetch"
	"github.com/napadailo/voip-info/internal/news"
)

// API of the Nasdaq quotes; it answers browsers only.
var (
	API       = "https://api.nasdaq.com/api/quote/"
	UserAgent = "Mozilla/5.0 (Macintosh; Intel Mac OS X 14_5) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/129.0 Safari/537.36"
)

// Quote is the closing prices of about 90 trading days.
type Quote struct {
	Price  float64
	Closes []float64
	AsOf   string
	Err    error
}

// Change in percent over the last days trading days.
func (q *Quote) Change(days int) (float64, bool) {
	if q == nil || len(q.Closes) <= days || q.Closes[len(q.Closes)-days-1] == 0 {
		return 0, false
	}
	last, then := q.Closes[len(q.Closes)-1], q.Closes[len(q.Closes)-days-1]
	return (last - then) / then * 100, true
}

// FetchQuote reads the chart of a ticker for 130 days (cached for 6 hours).
func FetchQuote(ctx context.Context, f *fetch.Fetcher, ticker string, force bool) *Quote {
	today := time.Now().UTC()
	u := fmt.Sprintf("%s%s/chart?assetclass=stocks&fromdate=%s&todate=%s", API, ticker,
		today.AddDate(0, 0, -130).Format(time.DateOnly), today.Format(time.DateOnly))
	res := f.Get(ctx, u, fetch.Options{TTL: 6 * time.Hour, Force: force, Headers: map[string]string{"User-Agent": UserAgent}})
	if res.Body == nil {
		return &Quote{Err: res.Err}
	}
	var data struct {
		Data struct {
			Chart []struct {
				Y json.RawMessage `json:"y"`
			} `json:"chart"`
			AsOf string `json:"timeAsOf"`
		} `json:"data"`
	}
	if err := json.Unmarshal(res.Body, &data); err != nil {
		return &Quote{Err: err}
	}
	q := &Quote{AsOf: data.Data.AsOf}
	for _, p := range data.Data.Chart {
		if v, err := strconv.ParseFloat(strings.Trim(string(p.Y), `"`), 64); err == nil {
			q.Closes = append(q.Closes, v)
		}
	}
	if len(q.Closes) > 0 {
		q.Price = q.Closes[len(q.Closes)-1]
	}
	return q
}

// VendorFeed is the Google News search of a vendor for 30 days.
func VendorFeed(v catalog.Vendor) catalog.Feed { return news.GoogleNews(v.Name, v.Query+" when:30d") }

// EventFeed is the Google News search of an event.
func EventFeed(e catalog.Event) catalog.Feed { return news.GoogleNews(e.Name, e.Query) }

// NextMonth is the usual month of the next edition of an event (zero when it varies).
func NextMonth(e catalog.Event, now time.Time) time.Time {
	if e.Month == 0 {
		return time.Time{}
	}
	year := now.Year()
	if e.Month < int(now.Month()) {
		year++
	}
	return time.Date(year, time.Month(e.Month), 1, 0, 0, 0, 0, time.UTC)
}
