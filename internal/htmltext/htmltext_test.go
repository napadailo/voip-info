package htmltext

import (
	"strings"
	"testing"
)

func TestRender(t *testing.T) {
	out := Render(`<h2>Change Log</h2><p>Fix <code>res_pjsip</code> crash &amp; <b>leak</b> [red]</p>
<ul><li>one</li><li>two <a href="https://x">link</a></li></ul><pre>raw   spaces</pre><br>`)
	for _, want := range []string{"Change Log", "res_pjsip", "crash & ", "leak", "•[-] one", "link", "raw   spaces"} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in\n%s", want, out)
		}
	}
	// text from the notes is escaped: "[red]" must not become a color tag
	if !strings.Contains(out, "[red[]") {
		t.Errorf("not escaped:\n%s", out)
	}
}

func TestRenderBrokenHTML(t *testing.T) {
	if out := Render(`<p>unclosed <b>bold<li>item`); !strings.Contains(out, "item") {
		t.Errorf("got %q", out)
	}
}

func TestText(t *testing.T) {
	out := Text(`<h3>Upgrade Notes:</h3><ul><li><code>http.c</code>: [now] disabled</li></ul>`)
	if out != "Upgrade Notes:\n• http.c: [now] disabled" {
		t.Errorf("%q", out)
	}
}
