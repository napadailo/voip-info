package catalog

import (
	"regexp"
	"strings"
	"testing"
)

// Every regex of the data compiles under RE2 (no lookarounds, no backreferences).
func TestCatalogLoadsAndRegexesCompile(t *testing.T) {
	ps, err := Products()
	if err != nil {
		t.Fatal(err)
	}
	if len(ps) < 60 {
		t.Fatalf("only %d products", len(ps))
	}
	ids := map[string]bool{}
	cats := map[string]bool{}
	for _, c := range Categories {
		cats[c.Name] = true
	}
	for _, p := range ps {
		ids[p.ID] = true
		if !cats[p.Category] {
			t.Errorf("%s: unknown category %q", p.ID, p.Category)
		}
		for _, rx := range []string{p.NVDFilter, p.NVDExclude} {
			if _, err := regexp.Compile(rx); err != nil {
				t.Errorf("%s: %v", p.ID, err)
			}
		}
		if p.NewsMatch != nil {
			if _, err := regexp.Compile(*p.NewsMatch); err != nil {
				t.Errorf("%s: news_match: %v", p.ID, err)
			}
		}
	}
	for _, p := range ps {
		for _, d := range p.Deps {
			if id, _, _ := strings.Cut(d, ":"); !ids[id] {
				t.Errorf("%s: dependency %q is not in the catalog", p.ID, id)
			}
		}
	}
	feeds, err := Feeds()
	if err != nil || len(feeds) == 0 {
		t.Fatal(err)
	}
	for _, f := range feeds {
		if _, err := regexp.Compile(f.Filter); err != nil {
			t.Errorf("feed %s: %v", f.Name, err)
		}
	}
	if v, err := Vendors(); err != nil || len(v) == 0 {
		t.Fatal(err)
	}
	if e, err := Events(); err != nil || len(e) == 0 {
		t.Fatal(err)
	}
}

func TestTextsInBothLanguages(t *testing.T) {
	ps, _ := Products()
	for _, p := range ps {
		if p.Desc["en"] == "" || p.Desc["ru"] == "" {
			t.Errorf("%s: desc %v", p.ID, p.Desc)
		}
	}
	vs, _ := Vendors()
	for _, v := range vs {
		if v.Desc["en"] == "" || v.Desc["ru"] == "" {
			t.Errorf("%s: desc %v", v.ID, v.Desc)
		}
	}
	if (Text{"en": "Brussels", "ru": "Брюссель"}).In("ru") != "Брюссель" || (Text{"en": "x"}).In("ru") != "x" || (Text{"ru": "y"}).In("en") != "y" {
		t.Error("Text.In")
	}
}
