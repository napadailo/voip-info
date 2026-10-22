package deps

import (
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/napadailo/voip-info/internal/htmltext"
	"github.com/napadailo/voip-info/internal/model"
)

// Fix statuses of an inherited vulnerability.
const (
	Pending  = "pending"  // no release of the consumer since the publication
	Released = "released" // a release came out, its notes do not mention the dependency
	Fixed    = "fixed"    // a release mentions the vulnerability or the dependency: probably fixed
	Separate = "separate" // the dependency is a separate service: update it on its own
)

// Aliases are the words a dependency goes by in release notes.
var Aliases = map[string][]string{
	"pjsip":     {"pjproject", "pjsip", "pjlib"},
	"sofia-sip": {"sofia-sip", "sofia"},
	"libre":     {"libre", "baresip/re"},
	"opus":      {"opus"},
	"spandsp":   {"spandsp"},
	"linphone":  {"liblinphone", "linphone", "belle-sip", "mediastreamer"},
	"pion":      {"pion"},
}

// Inherited is a vulnerability of a dependency as seen from a consumer.
type Inherited struct {
	Vuln     *model.Vuln
	First    Edge   // the first hop from the consumer
	Upstream string // the vulnerable dependency
	Depth    int
	Status   string
	By       string // the product whose releases decided the status
	Tag      string // the release mentioning it (fixed) or the first one after (released)
}

// Source gives the releases (newest first) and the vulnerabilities of a product.
type Source interface {
	Releases(id string) []*model.Release
	Vulns(id string) []*model.Vuln
}

var (
	textCache sync.Map
	aliasRes  sync.Map
)

func notesText(html string) string {
	if v, ok := textCache.Load(html); ok {
		return v.(string)
	}
	t := strings.ToLower(htmltext.Text(html))
	textCache.Store(html, t)
	return t
}

func aliasRe(up string) *regexp.Regexp {
	if v, ok := aliasRes.Load(up); ok {
		return v.(*regexp.Regexp)
	}
	words := Aliases[up]
	if words == nil {
		words = []string{up}
	}
	q := make([]string, len(words))
	for i, w := range words {
		q[i] = regexp.QuoteMeta(w)
	}
	re := regexp.MustCompile(`\b(` + strings.Join(q, "|") + `)\b`)
	aliasRes.Store(up, re)
	return re
}

func mentions(r *model.Release, v *model.Vuln, up string) bool {
	text := notesText(r.NotesHTML)
	if r.Commit {
		text = strings.ToLower(r.Title)
	}
	for _, id := range v.IDs() {
		if strings.Contains(text, strings.ToLower(id)) {
			return true
		}
	}
	return aliasRe(up).MatchString(text)
}

func notesStatus(src Source, id string, v *model.Vuln, up string) (status, tag string) {
	var after []*model.Release // newest first
	for _, r := range src.Releases(id) {
		if !r.Date.Before(v.Published) {
			after = append(after, r)
		}
	}
	if len(after) == 0 {
		return Pending, ""
	}
	for i := len(after) - 1; i >= 0; i-- {
		if mentions(after[i], v, up) {
			return Fixed, after[i].Tag
		}
	}
	return Released, after[len(after)-1].Tag
}

func (g *Graph) reaches(a, b string) bool {
	for _, h := range g.Upstream(a) {
		if h.Edge.Dst == b {
			return true
		}
	}
	return false
}

// StatusFor tells how a fix of a vulnerability of up reaches id: bundled and
// linked dependencies are fixed by a release of id itself (its notes are
// checked), platform and runtime ones are not shipped by id, so the status
// is the dependency's own.
func (g *Graph) StatusFor(src Source, id string, v *model.Vuln, up string) (status, by, tag string, first Edge, ok bool) {
	for _, e := range g.up[id] {
		if e.Dst == up || g.reaches(e.Dst, up) {
			first, ok = e, true
			break
		}
	}
	if !ok {
		return
	}
	if first.Kind == Platform || first.Kind == Runtime {
		if first.Dst == up {
			return Separate, up, "", first, true
		}
		status, by, tag, _, _ = g.StatusFor(src, first.Dst, v, up)
		return status, by, tag, first, true
	}
	status, tag = notesStatus(src, id, v, up)
	return status, id, tag, first, true
}

// InheritedVulns are the vulnerabilities of the dependencies of id published
// within days, of severity rank up to maxRank (model.SeverityRank), newest first.
func (g *Graph) InheritedVulns(src Source, id string, days, maxRank int) []Inherited {
	cutoff := time.Now().AddDate(0, 0, -days)
	var out []Inherited
	for _, h := range g.Upstream(id) {
		// weak links (runtime, optional) do not carry further than the first level
		if h.Depth > 1 && !Strong(h.Edge.Kind) {
			continue
		}
		for _, v := range src.Vulns(h.Edge.Dst) {
			if v.Published.Before(cutoff) || model.SeverityRank(v.Severity) > maxRank {
				continue
			}
			status, by, tag, first, ok := g.StatusFor(src, id, v, h.Edge.Dst)
			// nor through a weak first hop
			if !ok || h.Depth > 1 && !Strong(first.Kind) {
				continue
			}
			out = append(out, Inherited{Vuln: v, First: first, Upstream: h.Edge.Dst, Depth: h.Depth, Status: status, By: by, Tag: tag})
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Vuln.Published.After(out[j].Vuln.Published) })
	return out
}

// Summary counts the inherited vulnerabilities by status.
func Summary(inh []Inherited) map[string]int {
	out := map[string]int{}
	for _, i := range inh {
		out[i.Status]++
	}
	return out
}
