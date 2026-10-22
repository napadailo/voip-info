// Package watch keeps the subscription, "my stack": products, radar topics,
// vendors and IETF working groups. The products of the inventory and the
// dependencies of the watched products join it on their own (see hub).
//
// The file is watch.yaml in the data directory.
package watch

import (
	"bufio"
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	"gopkg.in/yaml.v3"
)

// Kinds of subscriptions.
const (
	Products = "products"
	Topics   = "topics"
	Vendors  = "vendors"
	WGs      = "wgs"
)

// Watch is the subscription.
type Watch struct {
	Products []string `yaml:"products,omitempty"`
	Topics   []string `yaml:"topics,omitempty"`
	Vendors  []string `yaml:"vendors,omitempty"`
	WGs      []string `yaml:"wgs,omitempty"`
}

const file = "watch.yaml"

func (w *Watch) list(kind string) *[]string {
	switch kind {
	case Products:
		return &w.Products
	case Topics:
		return &w.Topics
	case Vendors:
		return &w.Vendors
	case WGs:
		return &w.WGs
	}
	return nil
}

// Has reports whether an item is watched.
func (w *Watch) Has(kind, id string) bool {
	l := w.list(kind)
	return l != nil && slices.Contains(*l, id)
}

// Set turns watching an item on or off.
func (w *Watch) Set(kind, id string, on bool) {
	l := w.list(kind)
	if l == nil {
		return
	}
	i := slices.Index(*l, id)
	switch {
	case on && i < 0:
		*l = append(*l, id)
	case !on && i >= 0:
		*l = slices.Delete(*l, i, i+1)
	}
}

// Empty reports whether nothing is watched explicitly.
func (w *Watch) Empty() bool {
	return len(w.Products)+len(w.Topics)+len(w.Vendors)+len(w.WGs) == 0
}

// Load reads the subscription of dir; the first time it imports the file
// of the Python version (~/.voipinfo/watch.toml or $VOIPINFO_HOME), with
// topics renamed by rename (the Python names were Russian).
func Load(dir string, rename func(string) string) (*Watch, error) {
	w := &Watch{}
	data, err := os.ReadFile(filepath.Join(dir, file))
	if os.IsNotExist(err) {
		if old := legacy(rename); old != nil {
			_ = Save(dir, old)
			return old, nil
		}
		return w, nil
	}
	if err != nil {
		return w, err
	}
	return w, yaml.Unmarshal(data, w)
}

// Save writes the subscription of dir.
func Save(dir string, w *Watch) error {
	var b bytes.Buffer
	b.WriteString("# My stack (voip-info): w on a row of any view, :watch <id|topic|vendor|wg>, :unwatch.\n")
	if !w.Empty() {
		enc := yaml.NewEncoder(&b)
		enc.SetIndent(2)
		if err := enc.Encode(w); err != nil {
			return err
		}
	}
	return os.WriteFile(filepath.Join(dir, file), b.Bytes(), 0o644)
}

var lineRe = regexp.MustCompile(`^(\w+)\s*=\s*(\[.*\])\s*$`)

// legacy reads `products = ["asterisk"]` lines: TOML arrays of strings are JSON.
func legacy(rename func(string) string) *Watch {
	dir := os.Getenv("VOIPINFO_HOME")
	if dir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return nil
		}
		dir = filepath.Join(home, ".voipinfo")
	}
	f, err := os.Open(filepath.Join(dir, "watch.toml"))
	if err != nil {
		return nil
	}
	defer f.Close()
	w := &Watch{}
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		m := lineRe.FindStringSubmatch(strings.TrimSpace(sc.Text()))
		if m == nil {
			continue
		}
		var items []string
		if json.Unmarshal([]byte(m[2]), &items) != nil {
			continue
		}
		for _, it := range items {
			if m[1] == Topics && rename != nil {
				it = rename(it)
			}
			w.Set(m[1], it, true)
		}
	}
	if w.Empty() {
		return nil
	}
	return w
}
