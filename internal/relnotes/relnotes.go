// Package relnotes classifies release notes: what is inside a release and
// how risky the upgrade is. It reads the notes already loaded; Asterisk
// marks changes with UpgradeNote / UserNote fields, which count on their own.
package relnotes

import (
	"regexp"
	"sort"
	"strings"
	"sync"

	"github.com/napadailo/voip-info/internal/htmltext"
	"github.com/napadailo/voip-info/internal/model"
)

// Tags in display order: security fixes, breaking changes, deprecations, features, fixes.
var Tags = []string{"SEC", "BRK", "DEP", "FEAT", "FIX"}

var (
	cveRe = regexp.MustCompile(`(?i)\b(CVE-\d{4}-\d{4,}|GHSA-\w{4}-\w{4}-\w{4})\b`)
	rules = map[string]*regexp.Regexp{
		"SEC": regexp.MustCompile(`(?i)\b(security|vulnerab|exploit|cve-\d|ghsa-|advisory|xss|injection|overflow|` +
			`use[- ]after[- ]free|denial of service|\bdos\b|auth(entication)? bypass)`),
		"BRK": regexp.MustCompile(`(?i)(breaking change|\bbreaking\b|backwards?[- ]incompatib|incompatible change|` +
			`upgradenote|upgrade note|\bremoved?\b (the )?(support|option|module|api|parameter)|no longer (support|available)|` +
			`must now|migration (is )?required|drop(ped)? support)`),
		"DEP": regexp.MustCompile(`(?i)\bdeprecat`),
		"FEAT": regexp.MustCompile(`(?i)(\bfeat(ure)?s?\b|\badd(ed|s)? (new |support|the ability|a new)|new (module|option|api|` +
			`feature|command|function)|\bintroduc|usernote|user note|\bimplement(ed|s)?\b)`),
		"FIX": regexp.MustCompile(`(?i)(\bfix(e[sd])?\b|\bbug\b|\bcrash|\bleak|\bregression|\bsegfault|\bdeadlock|` +
			`(minor|maintenance|bug ?fix|patch|point) release)`),
	}
	// template section titles (often empty) are not a signal by themselves
	anyHead  = regexp.MustCompile(`(?i)^\s*#*\s*(upgrade|user|developer)\s+notes?\s*:?\s*$|^\s*#*\s*breaking changes?\s*:?\s*$`)
	zeroStat = regexp.MustCompile(`(?i)(advisories|issues) resolved:\s*0\b`)
	head     = regexp.MustCompile(`(?i)^\s*#*\s*(upgrade notes?|breaking changes?|developer notes?)\s*:?\s*$`)
	inline   = regexp.MustCompile(`(?i)\b(upgradenote|breaking change)\s*:\s*(.+)`)
	nonWord  = regexp.MustCompile(`\W+`)
)

// Info is what the notes of a release say.
type Info struct {
	Tags    []string
	Counts  map[string]int
	CVEs    []string
	Upgrade []string // Upgrade Notes, breaking changes, developer notes ("(dev) " prefix)
	Empty   bool
}

// Risk of the upgrade: high (breaking), medium (features, deprecations), low (fixes only), ? (cannot tell).
func (i Info) Risk() string {
	switch {
	case i.Empty || len(i.Tags) == 0:
		return "?"
	case i.Has("BRK"):
		return "high"
	case i.Has("FEAT") || i.Has("DEP"):
		return "medium"
	}
	return "low"
}

// Has reports a tag.
func (i Info) Has(tag string) bool {
	for _, t := range i.Tags {
		if t == tag {
			return true
		}
	}
	return false
}

// Badge is the tags joined: "SEC BRK FIX".
func (i Info) Badge() string { return strings.Join(i.Tags, " ") }

// DevPrefix marks the developer notes among Upgrade.
const DevPrefix = "(dev) "

var cache sync.Map

// Classify the notes of a release (cached by content).
func Classify(r *model.Release) Info {
	key := r.NotesHTML
	if r.Commit {
		key = r.Title
	}
	if v, ok := cache.Load(key); ok {
		return v.(Info)
	}
	text := r.Title
	if !r.Commit {
		text = htmltext.Text(r.NotesHTML)
	}
	info := classify(text)
	cache.Store(key, info)
	return info
}

func classify(text string) Info {
	if len(strings.TrimSpace(text)) < 20 {
		return Info{Empty: true, Counts: map[string]int{}}
	}
	info := Info{Counts: map[string]int{}}
	for _, ln := range strings.Split(text, "\n") {
		ln = strings.TrimSpace(ln)
		if ln == "" || anyHead.MatchString(ln) || zeroStat.MatchString(ln) {
			continue
		}
		for tag, re := range rules {
			if re.MatchString(ln) {
				info.Counts[tag]++
			}
		}
	}
	seen := map[string]bool{}
	for _, m := range cveRe.FindAllStringSubmatch(text, -1) {
		id := strings.ToUpper(m[1])
		if !seen[id] {
			seen[id] = true
			info.CVEs = append(info.CVEs, id)
		}
	}
	sort.Strings(info.CVEs)
	if len(info.CVEs) > info.Counts["SEC"] {
		info.Counts["SEC"] = len(info.CVEs)
	}
	info.Upgrade = UpgradeNotes(text)
	brk := 0
	for _, u := range info.Upgrade {
		if !strings.HasPrefix(u, DevPrefix) {
			brk++
		}
	}
	info.Counts["BRK"] = max(info.Counts["BRK"], brk)
	for _, t := range Tags {
		if info.Counts[t] > 0 {
			info.Tags = append(info.Tags, t)
		}
	}
	return info
}

// UpgradeNotes extracts the items of "Upgrade Notes" / "Breaking changes" /
// "Developer Notes" sections and the paragraphs after "UpgradeNote:".
func UpgradeNotes(text string) []string {
	lines := strings.Split(text, "\n")
	var items []string
	for i := 0; i < len(lines); {
		ln := strings.TrimSpace(lines[i])
		if head.MatchString(ln) {
			prefix := ""
			if strings.Contains(strings.ToLower(ln), "developer") {
				prefix = DevPrefix
			}
			i++
			var cur []string
			for ; i < len(lines); i++ {
				raw := strings.TrimSpace(lines[i])
				if isHeading(raw) {
					break
				}
				if b, ok := bullet(raw); ok {
					if len(cur) > 0 {
						items = append(items, prefix+strings.Join(cur, " "))
					}
					cur = []string{}
					if b != "" {
						cur = append(cur, b)
					}
				} else if raw != "" && cur != nil {
					cur = append(cur, strings.TrimLeft(raw, "# "))
				}
			}
			if len(cur) > 0 {
				items = append(items, prefix+strings.Join(cur, " "))
			}
			continue
		}
		if m := inline.FindStringSubmatch(ln); m != nil {
			para := []string{strings.TrimSpace(m[2])}
			j := i + 1
			for ; j < len(lines); j++ {
				nx := strings.TrimSpace(lines[j])
				if nx == "" || inline.MatchString(nx) || strings.HasPrefix(nx, "#") || strings.HasPrefix(nx, "- ") || strings.HasPrefix(nx, "• ") {
					break
				}
				para = append(para, nx)
			}
			items = append(items, strings.Join(para, " "))
			i = j
			continue
		}
		i++
	}
	// the same text often is both in the section and in the commit's UpgradeNote
	norm := func(s string) string { return strings.TrimSpace(nonWord.ReplaceAllString(strings.ToLower(s), " ")) }
	var out []string
	for _, x := range items {
		nx := norm(x)
		if len([]rune(nx)) < 15 {
			continue
		}
		dup := false
		for _, y := range out {
			ny := norm(y)
			if strings.Contains(ny, first(nx, 60)) || strings.Contains(nx, first(ny, 60)) {
				dup = true
				break
			}
		}
		if !dup {
			out = append(out, x)
		}
	}
	if len(out) > 8 {
		out = out[:8]
	}
	return out
}

// isHeading: a markdown heading or a short title line ending with a colon
// ("Commit List:"), which is how the plain text renders <h3>.
func isHeading(s string) bool {
	if head.MatchString(s) || strings.HasPrefix(s, "#") && !strings.HasPrefix(s, "####") {
		return true
	}
	if _, ok := bullet(s); ok {
		return false
	}
	return strings.HasSuffix(s, ":") && len([]rune(s)) <= 40 && !strings.ContainsAny(strings.TrimSuffix(s, ":"), ".:")
}

// bullet returns the text of a list item line ("- x", "* x", "• x").
func bullet(s string) (string, bool) {
	if s == "•" || s == "-" || s == "*" {
		return "", true
	}
	for _, p := range []string{"- ", "* ", "• "} {
		if strings.HasPrefix(s, p) {
			return strings.TrimSpace(strings.TrimLeft(strings.TrimSpace(s[len(p):]), "# ")), true
		}
	}
	return "", false
}

func first(s string, n int) string {
	r := []rune(s)
	if len(r) > n {
		return string(r[:n])
	}
	return s
}
