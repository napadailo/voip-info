package radar

import (
	"testing"
	"time"

	"github.com/napadailo/voip-info/internal/catalog"
	"github.com/napadailo/voip-info/internal/model"
	"github.com/napadailo/voip-info/internal/news"
)

func TestBuild(t *testing.T) {
	now := time.Date(2026, 10, 21, 12, 0, 0, 0, time.UTC)
	day := func(d int) time.Time { return now.AddDate(0, 0, -d) }
	st := &model.ProductState{Product: &catalog.Product{ID: "asterisk", Name: "Asterisk"}, Releases: []*model.Release{
		{Tag: "23.5.0", Date: day(2), NotesHTML: "<p>res_pjsip: add STIR/SHAKEN attestation</p>"},
		{Tag: "0.1", Date: day(200), NotesHTML: "<p>stir shaken</p>"}, // too old
	}}
	items := []*news.Item{
		{Title: "Robocall rules tightened by the FCC", URL: "u1", Date: day(1), Products: []string{"asterisk"}},
		{Title: "Robocall rules tightened by the FCC", URL: "u1", Date: day(1)}, // the same link
		{Title: "Голосовой робот для колл-центра", URL: "u2", Date: day(20), Kind: "lists"},
	}
	stats := Build(Input{News: items, States: []*model.ProductState{st}, Names: map[string]string{"asterisk": "Asterisk"}, Now: now})
	by := map[string]*Stat{}
	for _, s := range stats {
		by[s.Name] = s
	}
	stir := by["STIR/SHAKEN, caller ID"]
	if stir.Count(7, now) != 2 || stir.BySource(30, now)[Release] != 1 || stir.TopProducts(30, 3, now)[0] != (ProductCount{"Asterisk", 2}) {
		t.Errorf("%+v", stir)
	}
	if ai := by["Voice AI / LLM agents"]; ai.Count(30, now) != 1 || ai.BySource(30, now)[Lists] != 1 || ai.Count(7, now) != 0 {
		t.Errorf("%+v", ai)
	}
	if w := stir.Weekly(4, now); w[3] != 2 {
		t.Errorf("weekly %v", w)
	}
}

func TestRussianWordForms(t *testing.T) {
	for _, c := range []struct{ topic, text string }{
		{"Voice AI / LLM agents", "Голосовой робот для банка"},
		{"Deals and investments", "Крупная сделка на рынке"},
		{"Copper / PSTN switch-off", "отключение медных линий"},
	} {
		found := false
		for _, tp := range Topics {
			if tp.Name == c.topic {
				found = tp.Re.MatchString(c.text)
			}
		}
		if !found {
			t.Errorf("%s: %q", c.topic, c.text)
		}
	}
	for _, tp := range Topics {
		if tp.Name == "Deals and investments" && tp.Re.MatchString("пересделка") {
			t.Error("сделк inside a word")
		}
	}
}
