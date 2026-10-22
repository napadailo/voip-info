// Package radar counts what the news, release notes, mailing lists and IETF
// documents talk about. Topics are regular expressions (English and
// Russian); the trend is the last week against the average week of the month.
package radar

import (
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/napadailo/voip-info/internal/htmltext"
	"github.com/napadailo/voip-info/internal/ietf"
	"github.com/napadailo/voip-info/internal/model"
	"github.com/napadailo/voip-info/internal/news"
)

// N marks a message for the catalog; the UI translates it.
func N(s string) string { return s }

// Topic is a name (English, translated by the UI) and its pattern.
type Topic struct {
	Name string
	Re   *regexp.Regexp
}

// t compiles a topic. RE2 classes are ASCII: \w becomes a letter of any
// alphabet, or the Russian word forms would not match.
func t(name, re string) Topic {
	return Topic{name, regexp.MustCompile(`(?i)` + strings.ReplaceAll(re, `\w`, `\pL`))}
}

// Topics followed.
var Topics = []Topic{
	t(N("Voice AI / LLM agents"), `voice ?ai|voice agent|ai agent|\bllm|speech-to-speech|realtime api|conversational ai|`+
		`голосов\w* (ии|ai|агент|робот|бот)|ии-агент|ии-ассистент|нейросет`),
	t(N("STIR/SHAKEN, caller ID"), `\bstir\b|shaken|robocall|caller id|spoof|подмен\w* номер|антифрод|определител\w* номер`),
	t(N("Fraud, toll fraud"), `toll fraud|\bfraud|vishing|\bscam|мошенни|спам-звонк|телефонн\w* спам`),
	t(N("Media over QUIC"), `\bmoq\b|media over quic|webtransport`),
	t(N("WebRTC, WHIP/WHEP"), `webrtc|\bwhip\b|\bwhep\b`),
	t(N("TLS / SRTP / encryption"), `\btls\b|\bsrtp|\bdtls|encrypt|шифров`),
	t(N("Post-quantum cryptography"), `post-?quantum|\bpqc\b|ml-kem|kyber|постквант`),
	t(N("Codecs (Opus, AV1, H.265)"), `\bopus\b|\bcodec|\bdred\b|\bav1\b|h\.?265|hevc|кодек`),
	t(N("IMS / VoLTE / 5G"), `\bims\b|volte|vonr|\b5g\b|p-cscf|s-cscf`),
	t(N("RCS and messaging"), `\brcs\b|rich communication`),
	t(N("Microsoft Teams, Direct Routing"), `microsoft teams|direct routing|operator connect|teams phone`),
	t(N("Kubernetes / cloud-native"), `kubernetes|\bk8s\b|\bhelm\b|cloud-native|контейнер`),
	t(N("Copper / PSTN switch-off"), `copper|pstn switch|digital landline|\bpots\b|switch-?off|медн\w* (сет|лини)`),
	t(N("Regulation"), `\bfcc\b|ofcom|regulat|регулир|законопроект|минцифр|роскомнадзор`),
	t(N("Contact centers / CCaaS"), `contact cent|\bccaas|call cent|колл-?центр|контакт-?центр`),
	t(N("Emergency calls"), `\be911\b|\b911\b|\b112\b|emergency call|\becrit\b|экстренн`),
	t(N("STT / TTS / transcription"), `transcri|speech[- ]to[- ]text|\bstt\b|\btts\b|text[- ]to[- ]speech|`+
		`распознавани\w* речи|синтез\w* речи|транскриб`),
	t(N("Vulnerabilities and attacks"), `vulnerab|\bcve-\d|exploit|\bransomware|уязвим|хакер|атак`),
	t(N("Deals and investments"), `acqui|merger|\bipo\b|funding|raises \$|raised \$|series [a-d]\b|поглощ|приобрет\w* (компан|бизнес)|`+
		`инвестиц|(^|[^\pL])сделк`),
}

// Sources of mentions.
const (
	News    = "news"
	Release = "release"
	IETF    = "ietf"
	Lists   = "lists"
)

// Mention of a topic.
type Mention struct {
	Date    time.Time
	Source  string
	Title   string
	URL     string
	Product string // product name, "" when none
}

// Stat of a topic.
type Stat struct {
	Name     string
	Mentions []Mention // newest first
}

// Count of mentions within days.
func (s *Stat) Count(days int, now time.Time) int {
	cut := now.AddDate(0, 0, -days)
	n := 0
	for _, m := range s.Mentions {
		if !m.Date.Before(cut) {
			n++
		}
	}
	return n
}

// Trend in percent: the last week against the average week of 30 days.
func (s *Stat) Trend(now time.Time) (float64, bool) {
	base := float64(s.Count(30, now)) / 4.3
	if base < 1 {
		return 0, false
	}
	return (float64(s.Count(7, now)) - base) / base * 100, true
}

// Weekly counts, oldest first.
func (s *Stat) Weekly(weeks int, now time.Time) []int {
	out := make([]int, weeks)
	for _, m := range s.Mentions {
		w := int(now.Sub(m.Date).Hours() / 24 / 7)
		if w >= 0 && w < weeks {
			out[weeks-1-w]++
		}
	}
	return out
}

// BySource counts the mentions of days by source.
func (s *Stat) BySource(days int, now time.Time) map[string]int {
	cut := now.AddDate(0, 0, -days)
	out := map[string]int{}
	for _, m := range s.Mentions {
		if !m.Date.Before(cut) {
			out[m.Source]++
		}
	}
	return out
}

// ProductCount is a product and its mentions.
type ProductCount struct {
	Name  string
	Count int
}

// TopProducts are the products mentioned with the topic most often within days.
func (s *Stat) TopProducts(days, n int, now time.Time) []ProductCount {
	cut := now.AddDate(0, 0, -days)
	cnt := map[string]int{}
	for _, m := range s.Mentions {
		if !m.Date.Before(cut) && m.Product != "" {
			cnt[m.Product]++
		}
	}
	var out []ProductCount
	for p, c := range cnt {
		out = append(out, ProductCount{p, c})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Count != out[j].Count {
			return out[i].Count > out[j].Count
		}
		return out[i].Name < out[j].Name
	})
	if len(out) > n {
		out = out[:n]
	}
	return out
}

// Input is everything loaded.
type Input struct {
	News   []*news.Item
	States []*model.ProductState
	Docs   []*ietf.Doc
	Names  map[string]string // product id -> name
	Days   int               // how far back, 84 by default
	Now    time.Time
}

// Build counts the topics over the input.
func Build(in Input) []*Stat {
	if in.Days == 0 {
		in.Days = 84
	}
	if in.Now.IsZero() {
		in.Now = time.Now()
	}
	cut := in.Now.AddDate(0, 0, -in.Days)
	stats := make([]*Stat, len(Topics))
	for i, tp := range Topics {
		stats[i] = &Stat{Name: tp.Name}
	}
	scan := func(text string, m Mention) {
		for i, tp := range Topics {
			if tp.Re.MatchString(text) {
				stats[i].Mentions = append(stats[i].Mentions, m)
			}
		}
	}
	seen := map[string]bool{}
	for _, n := range in.News {
		if n.Date.Before(cut) || seen[n.URL] {
			continue
		}
		seen[n.URL] = true
		src := News
		if n.Kind == "lists" {
			src = Lists
		}
		prod := ""
		if len(n.Products) > 0 {
			prod = in.Names[n.Products[0]]
		}
		sum := []rune(n.Summary)
		scan(n.Title+" "+string(sum[:min(len(sum), 500)]), Mention{n.Date, src, n.Title, n.URL, prod})
	}
	for _, st := range in.States {
		for _, r := range st.Releases {
			if r.Date.Before(cut) || r.Commit {
				continue
			}
			text := []rune(htmltext.Text(r.NotesHTML))
			scan(string(text[:min(len(text), 4000)]), Mention{r.Date, Release, st.Product.Name + " " + r.Tag, r.URL, st.Product.Name})
		}
	}
	for _, d := range in.Docs {
		if !d.Time.Before(cut) {
			scan(d.Title+" "+d.Abstract, Mention{d.Time, IETF, d.Label() + ": " + d.Title, d.URL(), ""})
		}
	}
	for _, s := range stats {
		sort.SliceStable(s.Mentions, func(i, j int) bool { return s.Mentions[i].Date.After(s.Mentions[j].Date) })
	}
	return stats
}
