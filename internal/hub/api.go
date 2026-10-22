package hub

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/napadailo/voip-info/internal/analytics"
	"github.com/napadailo/voip-info/internal/eol"
	"github.com/napadailo/voip-info/internal/fetch"
	"github.com/napadailo/voip-info/internal/ghapi"
	"github.com/napadailo/voip-info/internal/health"
	"github.com/napadailo/voip-info/internal/model"
	"github.com/napadailo/voip-info/internal/sources/github"
)

type apiResult struct {
	repo       *ghapi.Repo
	weekly     []int
	activity   []ghapi.Issue
	milestones []ghapi.Milestone
	advisories []ghapi.Advisory
	errs       []string
}

func noteErr(errs *[]string, err error) {
	if err != nil && !errors.Is(err, fetch.ErrNotCached) && err.Error() != "computing" {
		*errs = append(*errs, err.Error())
	}
}

// fetchAPI reads what the GitHub API knows about p: everything from the
// network, or only what is cached (cacheOnly, the table without a token).
func (h *Hub) fetchAPI(ctx context.Context, st *model.ProductState, force, cacheOnly bool) apiResult {
	p, f := st.Product, h.Fetcher
	var r apiResult
	var err error
	r.repo, err = github.FetchRepo(ctx, f, p, force, cacheOnly)
	noteErr(&r.errs, err)
	r.weekly, err = github.FetchParticipation(ctx, f, p, force, cacheOnly)
	noteErr(&r.errs, err)
	r.activity, err = github.FetchActivity(ctx, f, p, time.Now().Add(-30*24*time.Hour), force, cacheOnly)
	noteErr(&r.errs, err)
	r.milestones, err = github.FetchMilestones(ctx, f, p, force, cacheOnly)
	noteErr(&r.errs, err)
	r.advisories, err = github.FetchAdvisories(ctx, f, p, force, cacheOnly)
	noteErr(&r.errs, err)
	return r
}

// apply stores an API result; on the UI goroutine.
func (h *Hub) apply(st *model.ProductState, r apiResult, full bool) {
	g := &st.GH
	if r.repo != nil {
		g.Repo = r.repo
		_ = h.Store.Snapshot(st.Product.ID, today(), map[string]float64{
			"stars": float64(r.repo.Stars), "forks": float64(r.repo.Forks), "open_issues": float64(r.repo.OpenIssues),
		})
	}
	if r.weekly != nil {
		g.Weekly = r.weekly
	}
	if r.activity != nil {
		g.Activity = r.activity
	}
	if r.milestones != nil {
		g.Milestones = r.milestones
	}
	if r.advisories != nil {
		g.Advisories = r.advisories
		delete(h.vulns, st.Product.ID)
	}
	g.Errs = r.errs
	g.Full = g.Full || full
	g.Loading = false
}

func today() string { return time.Now().Format(time.DateOnly) }

// LoadTableMetrics fills the metrics of every product: from the network with
// a token (5000 requests per hour), from the cache without one.
func (h *Hub) LoadTableMetrics(ctx context.Context, force bool) {
	cacheOnly := !h.Fetcher.HasToken()
	h.Apply(func() { h.metricsDone = 0 })
	var wg sync.WaitGroup
	sem := make(chan struct{}, 6)
	for _, p := range h.Products {
		st := h.States[p.ID]
		wg.Add(1)
		go func() {
			defer wg.Done()
			sem <- struct{}{}
			r := h.fetchAPI(ctx, st, force, cacheOnly)
			<-sem
			h.Apply(func() {
				h.apply(st, r, !cacheOnly)
				// redraw as the metrics arrive, not only at the end: with a token this takes a while
				if h.metricsDone++; h.metricsDone%6 == 0 {
					h.OnChange("api")
				}
			})
		}()
	}
	wg.Wait()
	h.Apply(func() { h.SnapshotHealth() })
	h.changed("api")
}

// LoadAPI loads everything the GitHub API has on one product (the card).
func (h *Hub) LoadAPI(ctx context.Context, st *model.ProductState, force bool) {
	h.Apply(func() { st.GH.Loading = true; h.OnChange("api") })
	r := h.fetchAPI(ctx, st, force, false)
	h.Apply(func() { h.apply(st, r, true) })
	h.changed("api")
}

// LoadSupportDates finds when the Asterisk majors came out (its LTS policy counts from x.0.0).
func (h *Hub) LoadSupportDates(ctx context.Context) {
	var majors []int
	h.Apply(func() {
		if st := h.States["asterisk"]; st != nil {
			top := 0
			for _, r := range st.Stable() {
				if v := r.Version(); len(v) > 0 {
					top = max(top, v[0])
				}
			}
			for m := 18; m <= top; m++ {
				majors = append(majors, m)
			}
		}
	})
	if len(majors) == 0 {
		return
	}
	dates := eol.AsteriskMajorDates(ctx, h.Fetcher, majors, github.API, github.Base)
	h.Apply(func() { h.X0 = dates })
	h.changed("support")
}

// Support is the support matrix of a product's branches.
func (h *Hub) Support(st *model.ProductState) []eol.Branch { return eol.Matrix(st, h.X0, time.Now()) }

// Health is the health index of a product.
func (h *Hub) Health(st *model.ProductState) health.Health { return health.Compute(st, h.security(st)) }

// SnapshotHealth stores today's index of every product.
func (h *Hub) SnapshotHealth() {
	for _, st := range h.States {
		hl := h.Health(st)
		_ = h.Store.Snapshot(st.Product.ID, today(), map[string]float64{"health": float64(hl.Score), "health_cov": hl.Coverage})
	}
}

// Delta is the growth of a stored metric over about days.
func (h *Hub) Delta(id, metric string, days int) (float64, bool) {
	return analytics.Delta(h.Store.History(id, metric), days)
}
