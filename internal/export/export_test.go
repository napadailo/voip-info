package export

import (
	"os"
	"strings"
	"testing"
)

// plain is a format of visible marks, to check the parsing.
var plain = Format{
	Esc:     func(s string) string { return s },
	Code:    func(s string) string { return "`" + s + "`" },
	Bold:    func(s string) string { return "*" + s + "*" },
	Ita:     func(s string) string { return "_" + s + "_" },
	Link:    func(t, u string) string { return t + " <" + u + ">" },
	Quote:   func(s string) string { return "| " + s },
	PreLine: func(s string) string { return s },
}

func TestConvert(t *testing.T) {
	md := "# Digest\n\n_Period: a - b_\n\n| | Product | ID |\n|---|---|---|\n| 🟥 | Asterisk | [CVE-1](https://x/1) |\n\n" +
		"- **Kamailio** `6.0.3` snake_case_name\n  - [a \\| b](https://y)\n\n---\n> quote"
	got := strings.Join(Convert(md, plain), "\n")
	want := "*Digest*\n\n_Period: a - b_\n\n• 🟥 · Asterisk · CVE-1 <https://x/1>\n\n" +
		"• *Kamailio* `6.0.3` snake_case_name\n   • a | b <https://y>\n\n──────────\n| quote"
	if got != want {
		t.Errorf("got\n%s\nwant\n%s", got, want)
	}
}

func TestSaveMarkdown(t *testing.T) {
	dir := t.TempDir()
	p, err := SaveMarkdown(dir, "# x", "digest")
	if err != nil || !strings.HasPrefix(p, dir+"/exports/digest-") || !strings.HasSuffix(p, ".md") {
		t.Fatal(p, err)
	}
	if b, _ := os.ReadFile(p); string(b) != "# x" {
		t.Errorf("%q", b)
	}
}
