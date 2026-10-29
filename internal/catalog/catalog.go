// Package catalog holds what voip-info tracks: products, news feeds, vendors
// and events. The data is embedded YAML (data/*.yaml), edited by hand.
package catalog

import (
	"embed"
	"fmt"
	"regexp"

	"gopkg.in/yaml.v3"
)

//go:embed data/*.yaml
var data embed.FS

// Product is a tracked open-source project.
type Product struct {
	ID         string   `yaml:"id"`
	Name       string   `yaml:"name"`
	Category   string   `yaml:"category"`
	Repo       string   `yaml:"repo"` // GitHub owner/name
	Homepage   string   `yaml:"homepage"`
	Desc       Text     `yaml:"desc"`
	TagFilter  string   `yaml:"tag_filter"`  // regex: which tags are releases
	NVD        string   `yaml:"nvd"`         // NVD keyword search
	NVDFilter  string   `yaml:"nvd_filter"`  // regex over CVE descriptions
	NVDExclude string   `yaml:"nvd_exclude"` // regex: CVE descriptions to drop
	Modules    string   `yaml:"modules"`     // a modular product: the module whose versions are the product's
	Advisories string   `yaml:"advisories"`  // GitHub owner/name of the security advisories, when not Repo
	NewsMatch  *string  `yaml:"news_match"`  // regex for news; "" = never tag
	Deps       []string `yaml:"deps"`        // "id:kind"
	Track      string   `yaml:"track"`       // releases (default) or commits
	NPM        []string `yaml:"npm"`
	PyPI       []string `yaml:"pypi"`
	Docker     []string `yaml:"docker"`

	tagRe *regexp.Regexp
}

// URL is the GitHub page of the project.
func (p *Product) URL() string { return "https://github.com/" + p.Repo }

// TagMatches reports whether a tag passes tag_filter.
func (p *Product) TagMatches(tag string) bool { return p.tagRe == nil || p.tagRe.MatchString(tag) }

// Feed is a news source.
type Feed struct {
	Name    string `yaml:"name"`
	URL     string `yaml:"url"`
	Kind    string `yaml:"kind"` // project, industry, market, regional, community, lists
	Product string `yaml:"product"`
	Type    string `yaml:"type"`   // rss (default), hn, pipermail
	Filter  string `yaml:"filter"` // regex over titles
}

// Vendor is a commercial company.
type Vendor struct {
	ID      string `yaml:"id"`
	Name    string `yaml:"name"`
	Segment string `yaml:"segment"`
	Ticker  string `yaml:"ticker"`
	Query   string `yaml:"query"`
	Desc    Text   `yaml:"desc"`
}

// Event is a conference or meeting.
type Event struct {
	ID    string `yaml:"id"`
	Name  string `yaml:"name"`
	Where Text   `yaml:"where"`
	Month int    `yaml:"month"`
	URL   string `yaml:"url"`
	Query string `yaml:"query"`
}

// Category is a product group with its prompt alias and hotkey position.
type Category struct{ Alias, Name string }

// Categories in hotkey order (1-9).
var Categories = []Category{
	{"pbx", "PBX"}, {"sip", "SIP Proxy"}, {"media", "Media/RTP"}, {"webrtc", "WebRTC"}, {"stack", "SIP Stack"},
	{"mon", "Monitoring"}, {"ai", "CPaaS/AI"}, {"codec", "Codec/DSP"}, {"test", "Testing"},
}

// CategoryByAlias returns the category name of an alias.
func CategoryByAlias(alias string) (string, bool) {
	for _, c := range Categories {
		if c.Alias == alias {
			return c.Name, true
		}
	}
	return "", false
}

func load(name string, v any) error {
	b, err := data.ReadFile("data/" + name + ".yaml")
	if err != nil {
		return err
	}
	if err := yaml.Unmarshal(b, v); err != nil {
		return fmt.Errorf("%s.yaml: %w", name, err)
	}
	return nil
}

// Products loads the catalog.
func Products() ([]*Product, error) {
	var f struct {
		Products []*Product `yaml:"products"`
	}
	if err := load("catalog", &f); err != nil {
		return nil, err
	}
	seen := map[string]bool{}
	for _, p := range f.Products {
		if seen[p.ID] {
			return nil, fmt.Errorf("catalog: duplicate id %q", p.ID)
		}
		seen[p.ID] = true
		if p.TagFilter != "" {
			re, err := regexp.Compile(p.TagFilter)
			if err != nil {
				return nil, fmt.Errorf("catalog: %s: tag_filter: %w", p.ID, err)
			}
			p.tagRe = re
		}
	}
	return f.Products, nil
}

// Feeds loads the news sources.
func Feeds() ([]Feed, error) {
	var f struct {
		Feeds []Feed `yaml:"feeds"`
	}
	err := load("feeds", &f)
	return f.Feeds, err
}

// Vendors loads the commercial vendors.
func Vendors() ([]Vendor, error) {
	var f struct {
		Vendors []Vendor `yaml:"vendors"`
	}
	err := load("vendors", &f)
	return f.Vendors, err
}

// Events loads the conferences.
func Events() ([]Event, error) {
	var f struct {
		Events []Event `yaml:"events"`
	}
	err := load("events", &f)
	return f.Events, err
}
