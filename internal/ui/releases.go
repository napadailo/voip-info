package ui

import (
	"sort"

	"github.com/napadailo/voip-tui/tui"

	"github.com/napadailo/voip-info/internal/model"
)

// ReleasesView is the feed of all releases.
type ReleasesView struct {
	*TableView
	app *App
}

type releaseRef struct {
	st *model.ProductState
	r  *model.Release
}

func newReleasesView(a *App) *ReleasesView {
	v := &ReleasesView{app: a, TableView: tui.NewTableView(a.App, N("Releases"), []string{
		N("DATE"), N("AGE"), N("PRODUCT"), N("CATEGORY"), N("TAG"), N("BRANCH"), N("TYPE"), N("CHANGES"), N("UPGRADE RISK"),
	})}
	v.OnEnter(func(r *Row) {
		ref := r.Data.(releaseRef)
		a.Push(newProductView(a, ref.st, ref.r.Tag, ""))
	})
	a.bindBrowser(v.TableView)
	v.Bind('r', a.Reload)
	v.Bind('n', a.MarkAllSeen)
	a.bindCategories(v.TableView)
	a.bindStack(v.TableView)
	v.Rebuild()
	return v
}

func (v *ReleasesView) Name() string      { return "releases" }
func (v *ReleasesView) Table() *TableView { return v.TableView }
func (v *ReleasesView) Close()            {}
func (v *ReleasesView) Refresh()          { v.Rebuild() }

func (v *ReleasesView) Hints() []Hint {
	return append(categoryHints(), Hint{Key: "enter", Desc: N("card")}, Hint{Key: "b", Desc: N("browser")}, stackHints[0])
}

// Rebuild fills the feed: releases of the visible products, commits excluded.
func (v *ReleasesView) Rebuild() {
	var refs []releaseRef
	for _, st := range v.app.visible() {
		for _, r := range st.Releases {
			if !r.Commit {
				refs = append(refs, releaseRef{st, r})
			}
		}
	}
	sort.SliceStable(refs, func(i, j int) bool { return refs[i].r.Date.After(refs[j].r.Date) })
	rows := make([]Row, 0, len(refs))
	for _, ref := range refs {
		r := ref.r
		typ, tagTone := T("stable"), tui.ToneFg
		if r.Prerelease() {
			typ, tagTone = T("pre"), tui.ToneAccent
		}
		name := ref.st.Product.Name
		tones := map[int]tui.Tone{1: ageTone(r.AgeDays()), 3: tui.ToneDim, 4: tagTone, 6: tagTone}
		if ref.st.IsUnseen(r) {
			name = "● " + name
			tones[2] = tui.ToneOK
		}
		var changes, risk string
		changes, tones[7] = badge(r)
		risk, tones[8] = riskCell(r)
		rows = append(rows, Row{
			Cells:     []string{r.Date.Format("2006-01-02"), age(r.AgeDays()), name, ref.st.Product.Category, r.Tag, r.Branch(), typ, changes, risk},
			CellTones: tones, Markup: map[int]bool{7: true}, Sort: map[int]float64{1: float64(r.AgeDays())},
			Data: ref, Key: ref.st.Product.ID + "\t" + r.Tag,
		})
	}
	v.SetRows(rows)
	v.SetTitleInfo(v.app.titleInfo())
}
