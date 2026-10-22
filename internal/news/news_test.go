package news

import (
	"strings"
	"testing"

	"github.com/napadailo/voip-info/internal/catalog"
)

func TestParseRSSAndGoogleNews(t *testing.T) {
	rss := `<?xml version="1.0"?><rss><channel><atom:link href="x" rel="self"/>
<item><title>Sangoma posts results - Reuters</title><link>https://n/1</link><pubDate>Mon, 19 Oct 2026 10:00:00 GMT</pubDate>
<description>&lt;b&gt;ignored&lt;/b&gt; for market</description><source url="https://reuters.com">Reuters</source></item>
<item><title>Other</title><link>https://n/2</link><pubDate>Tue, 20 Oct 2026 10:00:00 +0000</pubDate></item>
</channel></rss>`
	items, err := Parse(catalog.Feed{Name: "GN: x", Kind: "market", Filter: "(?i)sangoma"}, []byte(rss))
	if err != nil || len(items) != 1 {
		t.Fatalf("%v %v", items, err)
	}
	n := items[0]
	if n.Title != "Sangoma posts results" || n.Source != "GN: x · Reuters" || n.Summary != "" || n.Date.Day() != 19 {
		t.Errorf("%+v", n)
	}
}

func TestParseAtomAndThreads(t *testing.T) {
	atom := `<feed xmlns="http://www.w3.org/2005/Atom">
<entry><title>Re: [sr-users] TLS problem</title><link rel="alternate" href="https://l/2"/><updated>2026-10-12T10:00:00Z</updated></entry>
<entry><title>TLS problem</title><link rel="alternate" href="https://l/1"/><updated>2026-10-10T10:00:00Z</updated><summary type="html">&lt;p&gt;Hello &amp;amp; help&lt;/p&gt;</summary></entry>
</feed>`
	items, err := Parse(catalog.Feed{Name: "sr-users", Kind: "lists", Product: "kamailio"}, []byte(atom))
	if err != nil || len(items) != 1 {
		t.Fatalf("%v %v", items, err)
	}
	n := items[0]
	if n.Title != "TLS problem" || n.Comments != 1 || n.Date.Day() != 12 || n.Summary != "Hello & help" || !n.Has("kamailio") {
		t.Errorf("%+v", n)
	}
}

func TestParseMbox(t *testing.T) {
	mbox := `From bob at example.com  Mon Oct  5 10:00:00 2026
From: bob at example.com (Bob)
Date: Mon, 05 Oct 2026 10:00:00 +0000
Subject: [OpenSIPS-Users] =?utf-8?q?dialog_=C3=A9vent?=

Body text

From ann at example.com  Tue Oct  6 10:00:00 2026
From: ann at example.com (Ann)
Date: Tue, 06 Oct 2026 10:00:00 +0000
Subject: Re: [OpenSIPS-Users] dialog évent

Reply
`
	items := ParseMbox(catalog.Feed{Name: "OpenSIPS users", Kind: "lists", Product: "opensips"}, mbox, "https://l/", "2026-October")
	if len(items) != 1 {
		t.Fatalf("%+v", items)
	}
	n := items[0]
	if n.Title != "dialog évent" || n.Comments != 1 || n.Date.Day() != 6 || !strings.HasPrefix(n.Summary, "Bob\n\nBody text") ||
		!strings.HasSuffix(n.Summary, "Last reply: Ann, 2026-10-06") || n.URL != "https://l/2026-October/thread.html" {
		t.Errorf("%+v", n)
	}
}

func TestTagAndDedupe(t *testing.T) {
	re := `\bMr\.SIP\b`
	ps := []*catalog.Product{{ID: "asterisk", Name: "Asterisk"}, {ID: "libre", Name: "libre (re)"}, {ID: "mr-sip", Name: "Mr.SIP", NewsMatch: &re},
		{ID: "kamailio", Name: "Kamailio"}}
	items := []*Item{{Title: "Asterisk 23 is out"}, {Title: "Footnote asterisk *"}, {Title: "Mr.SIP and kamailio"}}
	Tag(items, Matchers(ps))
	if !items[0].Has("asterisk") || items[1].Has("asterisk") || !items[2].Has("mr-sip") || !items[2].Has("kamailio") {
		t.Errorf("%+v %+v %+v", items[0], items[1], items[2])
	}
	gs := Dedupe([]*Item{{Title: "Twilio reports third quarter revenue growth"}, {Title: "Twilio third quarter revenue beats estimates"},
		{Title: "Zoom launches phone"}})
	if len(gs) != 2 || gs[0].Similar != 1 {
		t.Errorf("%+v", gs)
	}
}

func TestTitleHas(t *testing.T) {
	items := []*Item{{Title: "Avaya names new CEO"}, {Title: "Girls flag football player of the year"}, {Title: "Avaya's cloud"}}
	if got := TitleHas(items, FirstWord("Avaya")); len(got) != 2 {
		t.Errorf("%+v", got)
	}
	if FirstWord("Vonage (Ericsson)") != "Vonage" || len(TitleHas([]*Item{{Title: "The RTC.ON 2026 talks"}}, "RTC.ON")) != 1 {
		t.Error("first word")
	}
}
