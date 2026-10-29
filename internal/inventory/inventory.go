// Package inventory keeps the versions the user runs ("my versions") and
// tells what follows from them: vulnerabilities, newer releases, support.
//
// The file is inventory.yaml in the data directory:
//
//	asterisk:
//	  version: 20.18.0
//	  note: lab
package inventory

import (
	"bufio"
	"bytes"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/napadailo/voip-info/internal/eol"
	"github.com/napadailo/voip-info/internal/model"
	"github.com/napadailo/voip-info/internal/versions"
)

// N marks a message for the catalog; the UI translates it.
func N(s string) string { return s }

// Entry is a version in use.
type Entry struct {
	Version string `yaml:"version"`
	Note    string `yaml:"note,omitempty"`
}

// Inventory by product id.
type Inventory map[string]Entry

const file = "inventory.yaml"

// Load reads the inventory of dir. The first time it imports the file of the
// Python version (~/.voipinfo/inventory.toml, or $VOIPINFO_HOME).
func Load(dir string) (Inventory, error) {
	inv := Inventory{}
	data, err := os.ReadFile(filepath.Join(dir, file))
	if os.IsNotExist(err) {
		if old := legacy(); old != nil {
			_ = Save(dir, old)
			return old, nil
		}
		return inv, nil
	}
	if err != nil {
		return inv, err
	}
	if err := yaml.Unmarshal(data, &inv); err != nil {
		return Inventory{}, err
	}
	if inv == nil {
		inv = Inventory{}
	}
	return inv, nil
}

// Save writes the inventory of dir.
func Save(dir string, inv Inventory) error {
	var b bytes.Buffer
	b.WriteString("# My versions (voip-info): product id -> version, note. Edit by hand or with :have.\n")
	if len(inv) > 0 {
		enc := yaml.NewEncoder(&b)
		enc.SetIndent(2)
		if err := enc.Encode(inv); err != nil {
			return err
		}
	}
	return os.WriteFile(filepath.Join(dir, file), b.Bytes(), 0o644)
}

var (
	sectionRe = regexp.MustCompile(`^\[([\w.\-]+)\]$`)
	keyRe     = regexp.MustCompile(`^(\w+)\s*=\s*"([^"]*)"$`)
)

// legacy reads the TOML of the Python version: [id] then version = "...", note = "...".
func legacy() Inventory {
	dir := os.Getenv("VOIPINFO_HOME")
	if dir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return nil
		}
		dir = filepath.Join(home, ".voipinfo")
	}
	f, err := os.Open(filepath.Join(dir, "inventory.toml"))
	if err != nil {
		return nil
	}
	defer f.Close()
	inv := Inventory{}
	cur := ""
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		ln := strings.TrimSpace(sc.Text())
		if m := sectionRe.FindStringSubmatch(ln); m != nil {
			cur = m[1]
			continue
		}
		m := keyRe.FindStringSubmatch(ln)
		if m == nil || cur == "" {
			continue
		}
		e := inv[cur]
		switch m[1] {
		case "version":
			e.Version = m[2]
		case "note":
			e.Note = m[2]
		}
		inv[cur] = e
	}
	for id, e := range inv {
		if e.Version == "" {
			delete(inv, id)
		}
	}
	if len(inv) == 0 {
		return nil
	}
	return inv
}

// IDs in alphabetical order.
func (inv Inventory) IDs() []string {
	ids := make([]string, 0, len(inv))
	for id := range inv {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids
}

// Urgency of an assessment.
const (
	OK = iota
	Info
	Update
	Urgent
)

// Assessment is what follows from running a version.
type Assessment struct {
	ID, Version, Note string
	Branch            string
	Latest            *model.Release   // the newest release of the branch
	Behind            []*model.Release // releases of the branch newer than mine
	Mine              *model.Release   // my release in the feed, if it is there
	Affected          []*model.Vuln
	Unknown           []*model.Vuln // no version ranges (last year)
	Modules           []*model.Vuln // in a module of my major version, its version is not mine (last year or KEV)
	ModuleKEV         []*model.Vuln // the exploited ones of Modules
	KEV               []*model.Vuln
	Support           *eol.Branch
	Action            string // English format, translated with Args by the UI (urgent ones add → Target)
	Args              []any
	Urgency           int
}

// Target is the release to update to, "" when none is newer.
func (a *Assessment) Target() string {
	if a.Latest != nil && len(a.Behind) > 0 {
		return a.Latest.Tag
	}
	return ""
}

// ModuleKEVNote names the modules with exploited vulnerabilities when the
// action is about something else: "endpoint, ucp"; "" when none.
func (a *Assessment) ModuleKEVNote() string {
	if len(a.ModuleKEV) == 0 || a.Action == "check the modules: %d exploited (KEV)" {
		return ""
	}
	var names []string
	seen := map[string]bool{}
	for _, v := range a.ModuleKEV {
		if !seen[v.Module] {
			seen[v.Module] = true
			names = append(names, v.Module)
		}
	}
	return strings.Join(names, ", ")
}

// Serious are the affected high and critical vulnerabilities.
func (a *Assessment) Serious() []*model.Vuln {
	var out []*model.Vuln
	for _, v := range a.Affected {
		if v.Serious() {
			out = append(out, v)
		}
	}
	return out
}

// Context is what an assessment needs from the hub.
type Context struct {
	Vulns   []*model.Vuln
	KEV     func(*model.Vuln) bool
	Support []eol.Branch
}

// Assess the version of a product.
func Assess(st *model.ProductState, version, note string, c Context) *Assessment {
	v := versions.Parse(version)
	a := &Assessment{ID: st.Product.ID, Version: version, Note: note, Branch: "?"}
	if v != nil {
		a.Branch = (&model.Release{Product: st.Product.ID, Tag: version}).Branch()
	}
	for _, r := range st.Stable() {
		if r.Commit || r.Branch() != a.Branch || r.Version() == nil || v == nil {
			continue
		}
		if a.Latest == nil {
			a.Latest = r
		}
		switch model.CompareVersions(r.Version(), v) {
		case 1:
			a.Behind = append(a.Behind, r)
		case 0:
			if a.Mine == nil {
				a.Mine = r
			}
		}
	}
	for _, vu := range c.Vulns {
		affected, known := versions.Affects(vu, version)
		kev := c.KEV != nil && c.KEV(vu)
		switch {
		case vu.Module != "":
			// the module of another major version (FreePBX 17 modules on 16) is not here
			if len(vu.Ranges) > 0 && len(versions.InBranch(vu, version)) == 0 {
				continue
			}
			if kev || vu.AgeDays() <= 365 {
				a.Modules = append(a.Modules, vu)
			}
			if kev {
				a.ModuleKEV = append(a.ModuleKEV, vu)
			}
		case affected:
			a.Affected = append(a.Affected, vu)
			if kev {
				a.KEV = append(a.KEV, vu)
			}
		case !known && vu.AgeDays() <= 365:
			a.Unknown = append(a.Unknown, vu)
		}
	}
	for i := range c.Support {
		if c.Support[i].Branch == a.Branch {
			a.Support = &c.Support[i]
			break
		}
	}
	// policies by major (Asterisk 20) against a major.minor branch
	for i := range c.Support {
		if a.Support == nil && v != nil && c.Support[i].Branch == strconv.Itoa(v[0]) {
			a.Support = &c.Support[i]
		}
	}
	status := ""
	if a.Support != nil {
		status = a.Support.Status
	}
	target := a.Target()
	switch serious := a.Serious(); {
	case len(a.KEV) > 0:
		a.Urgency, a.Action, a.Args = Urgent, N("URGENT: %d exploited vulnerabilities (KEV)"), []any{len(a.KEV)}
	case len(serious) > 0:
		a.Urgency, a.Action, a.Args = Urgent, N("update: %d high/critical"), []any{len(serious)}
	case len(a.ModuleKEV) > 0:
		a.Urgency, a.Action, a.Args = Update, N("check the modules: %d exploited (KEV)"), []any{len(a.ModuleKEV)}
	case status == "eol" || status == "eol?":
		a.Urgency, a.Action = Update, N("unsupported branch - plan the migration")
	case len(a.Behind) > 0:
		a.Urgency = Info
		if len(a.Affected) > 0 {
			a.Urgency = Update
		}
		a.Action, a.Args = N("update available: %s (+%d)"), []any{target, len(a.Behind)}
	case status == "security":
		a.Urgency, a.Action = Info, N("branch is security-only")
	default:
		a.Urgency, a.Action = OK, N("up to date")
	}
	return a
}
