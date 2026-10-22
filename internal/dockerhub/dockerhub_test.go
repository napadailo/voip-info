package dockerhub

import (
	"testing"
	"time"

	"github.com/napadailo/voip-info/internal/catalog"
	"github.com/napadailo/voip-info/internal/model"
)

func TestFreshness(t *testing.T) {
	now := time.Now()
	rel := func(tag string, d int) *model.Release {
		return &model.Release{Product: "opensips", Tag: tag, Date: now.AddDate(0, 0, -d)}
	}
	st := &model.ProductState{Product: &catalog.Product{ID: "opensips"}, Releases: []*model.Release{
		rel("4.0.2", 5), rel("4.0.1", 40), rel("3.6.9", 42), rel("3.6.8", 60), rel("3.0.1", 2000)}}
	img := func(tags ...string) *Image {
		i := &Image{}
		for _, tg := range tags {
			i.Tags = append(i.Tags, Tag{Name: tg})
		}
		i.freshness(st)
		return i
	}
	if i := img("latest", "4.0", "3.6", "20240101"); i.VersionTag != "4.0" || i.Lag != 0 || i.Note != "floating tag %s follows branch %s" {
		t.Errorf("floating: %+v", i)
	}
	if i := img("3.6.8"); i.Lag != 1 || i.NoteArgs[2] != "3.6.9" {
		t.Errorf("behind: %+v", i)
	}
	if i := img("3.0.1"); !i.StaleBranch {
		t.Errorf("stale: %+v", i)
	}
	if i := img("latest"); i.Version != nil || i.Note == "" {
		t.Errorf("no version: %+v", i)
	}
}
