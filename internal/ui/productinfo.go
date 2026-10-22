package ui

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/rivo/tview"

	"github.com/napadailo/voip-tui/tui"
	"github.com/napadailo/voip-tui/tui/theme"

	"github.com/napadailo/voip-info/internal/dockerhub"
	"github.com/napadailo/voip-info/internal/news"
	"github.com/napadailo/voip-info/internal/upgrade"
)

// listPane is a table and a text pane beside it: the selected row shows in
// the text, enter opens it on the whole screen (full), b opens its link. The
// focus stays on the table: the pane is a preview.
func (v *ProductView) listPane(title string, headers []string, show, full func(*Row), link func(*Row) string) (*TableView, *tview.TextView) {
	t := tui.NewTableView(v.app.App, title, headers)
	text := tview.NewTextView().SetDynamicColors(true).SetWrap(true).SetWordWrap(true)
	text.SetBorder(true).SetBorderPadding(0, 0, 1, 1)
	open := func() {
		if r := t.Selected(); r != nil {
			v.app.OpenURL(link(r))
		}
	}
	t.OnSelect(show)
	t.OnEnter(full)
	t.Bind('b', open)
	t.SetInputCapture(v.chain(t.HandleKey))
	return t, text
}

// fillNews lists the news items mentioning the product.
func (v *ProductView) fillNews() {
	items := v.app.hub.ProductNews(v.st.Product.ID)
	rows := make([]Row, 0, len(items))
	for _, n := range items {
		rows = append(rows, Row{Cells: []string{n.Date.Format("2006-01-02"), tui.Trunc(n.Source, 22), tui.Trunc(n.Title, 90)},
			CellTones: map[int]tui.Tone{0: ageTone(n.AgeDays()), 1: tui.ToneDim}, Data: n, Key: n.URL})
	}
	v.newsT.SetRows(rows)
	switch {
	case len(rows) > 0:
		if r := v.newsT.Selected(); r != nil {
			v.showNewsItem(r)
		}
	case !v.app.hub.NewsLoaded:
		v.newsText.SetText(tui.Faint(T("loading…")))
	default:
		v.newsText.SetText(tui.Faint(T("No mentions in the followed sources.")))
	}
}

func (v *ProductView) showNewsItem(r *Row) {
	v.newsText.SetText(v.app.newsCard(r.Data.(*news.Item)))
	v.newsText.ScrollToBeginning()
}

// loadImages searches Docker Hub once, when the tab is opened.
func (v *ProductView) loadImages() {
	if v.imgLoaded || v.imgLoading {
		return
	}
	v.imgLoading = true
	v.fillImages()
	snap := upgrade.Snapshot(v.st)
	v.app.Async(func(ctx context.Context) func() {
		imgs, err := dockerhub.ProductImages(ctx, v.app.hub.Fetcher, snap, 8)
		return func() {
			v.images, v.imgLoading, v.imgLoaded = imgs, false, true
			if err != nil && len(imgs) == 0 {
				v.app.Flash().Errf("Docker Hub: %v", err)
			}
			v.fillImages()
		}
	})
}

func lagCell(img *dockerhub.Image) (string, tui.Tone) {
	switch {
	case img.Version == nil:
		return "?", tui.ToneDim
	case img.StaleBranch:
		return T("old"), tui.ToneErr
	case img.Lag == 0:
		return "ok", tui.ToneOK
	case img.Lag > 3:
		return fmt.Sprintf("-%d", img.Lag), tui.ToneErr
	}
	return fmt.Sprintf("-%d", img.Lag), tui.ToneWarn
}

func shortArchs(archs []string) string {
	var out []string
	for _, a := range first(archs, 3) {
		out = append(out, strings.NewReplacer("amd64", "x86", "arm64", "arm").Replace(a))
	}
	return strings.Join(out, ",")
}

// fillImages lists the Docker images found.
func (v *ProductView) fillImages() {
	rows := make([]Row, 0, len(v.images))
	for _, img := range v.images {
		name := img.Name
		tones := map[int]tui.Tone{4: tui.ToneAccent, 6: tui.ToneDim}
		if img.Official {
			name += " ✓"
		}
		if !img.Catalog {
			tones[0] = tui.ToneDim
		}
		updated, ver := "-", "?"
		if !img.Updated.IsZero() {
			d := int(time.Since(img.Updated).Hours() / 24)
			updated, tones[3] = age(d), ageTone(d)
		}
		if img.Version != nil {
			ver = joinInts(img.Version)
		} else {
			tones[4] = tui.ToneDim
		}
		lag, lt := lagCell(img)
		tones[5] = lt
		rows = append(rows, Row{Cells: []string{name, num(img.Pulls), fmt.Sprint(img.Stars), updated, ver, lag, shortArchs(img.Archs())},
			CellTones: tones, Data: img, Key: img.Name})
	}
	v.imgT.SetRows(rows)
	switch {
	case v.imgLoading:
		v.imgText.SetText(tui.Faint(T("searching Docker Hub…")))
	case !v.imgLoaded:
		v.imgText.SetText(tui.Faint(T("opens with the tab")))
	case len(rows) == 0:
		v.imgText.SetText(tui.Faint(T("No images found on Docker Hub.")))
	default:
		if r := v.imgT.Selected(); r != nil {
			v.showImage(r.Data.(*dockerhub.Image))
		}
	}
}

// imageCard is the markup of a Docker image: freshness, archs, the latest tags.
func (v *ProductView) imageCard(img *dockerhub.Image) string {
	title := img.Name
	if img.Official {
		title += " ✓ official"
	}
	c := tui.NewCard(title)
	src := T("listed in the catalog")
	if !img.Catalog {
		src = T("found by the Docker Hub search (community)")
	}
	updated := ""
	if !img.Updated.IsZero() {
		updated = Tf("updated %s", img.Updated.Format("2006-01-02"))
	}
	c.Sub(src, Tf("pulls %s", num(img.Pulls)), Tf("stars %d", img.Stars), updated)
	c.Text(tui.Trunc(img.Description, 300))
	ver := img.VersionTag
	if ver == "" {
		ver = T("not found")
	}
	c.KV(N("product version"), tui.Strong(ver))
	if img.Note != "" {
		_, lt := lagCell(img)
		c.KV(N("freshness"), tui.Paint(lt.Color(), tr(img.Note, img.NoteArgs)))
	}
	if archs := img.Archs(); len(archs) > 0 {
		c.KV(N("architectures"), esc(strings.Join(archs, ", ")))
	}
	if !img.Catalog {
		c.Line(tui.Paint(theme.Warn, T("A community image: check its Dockerfile and build source before production use.")))
	}
	c.Line(tui.Faint(img.URL()))
	if len(img.Tags) > 0 {
		c.Section(N("Latest tags"))
		var rows [][]string
		for _, t := range first(img.Tags, 20) {
			size, upd := "-", "-"
			if t.Size > 0 {
				size = fmt.Sprintf("%.0f MB", float64(t.Size)/1e6)
			}
			if !t.Updated.IsZero() {
				upd = t.Updated.Format("2006-01-02")
			}
			rows = append(rows, []string{tui.Paint(theme.Accent, t.Name), upd, size, esc(strings.Join(t.Archs, ", "))})
		}
		c.Line(strings.ReplaceAll(strings.TrimRight(textTable([]string{N("TAG"), N("UPDATED"), N("SIZE"), N("ARCH")}, rows), "\n"), "\n", "\n    "))
	}
	return c.String()
}

// showImage previews an image beside the list.
func (v *ProductView) showImage(img *dockerhub.Image) {
	v.imgText.SetText(v.imageCard(img))
	v.imgText.ScrollToBeginning()
}

func joinInts(v []int) string {
	s := make([]string, len(v))
	for i, n := range v {
		s[i] = fmt.Sprint(n)
	}
	return strings.Join(s, ".")
}
