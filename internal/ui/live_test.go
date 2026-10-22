package ui

import (
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/gdamore/tcell/v2"

	"github.com/napadailo/voip-tui/i18n"

	"github.com/napadailo/voip-info/internal/config"
	"github.com/napadailo/voip-info/internal/hub"
	"github.com/napadailo/voip-info/internal/store"
)

// TestLiveScreens prints screens over the real feeds: VOIP_INFO_LIVE=1 go test ./internal/ui -run Live -v
func waitAPI() int {
	if s := os.Getenv("VOIP_INFO_LIVE_WAIT"); s != "" {
		var n int
		fmt.Sscan(s, &n)
		return n
	}
	return 20
}

func TestLiveScreens(t *testing.T) {
	if os.Getenv("VOIP_INFO_LIVE") == "" {
		t.Skip("set VOIP_INFO_LIVE=1")
	}
	_ = i18n.Set("en")
	cfg, _ := config.Load(config.DefaultPath()) // the real secrets: the screens over the real feeds
	cfg.Apply()
	dir := os.Getenv("VOIP_INFO_DATA")
	if dir == "" {
		dir = t.TempDir()
	}
	st, err := store.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	h, _ := hub.New(st)
	h.DataDir = dir
	_ = h.LoadInventory() // imports ~/.voipinfo/inventory.toml the first time
	app := New(cfg, h)
	screen := tcell.NewSimulationScreen("UTF-8")
	_ = screen.Init()
	screen.SetSize(200, 50)
	app.SetScreen(screen)
	go func() { _ = app.Start("") }()
	defer app.Stop()
	time.Sleep(50 * time.Millisecond)
	app.QueueUpdateDraw(func() { screen.SetSize(200, 50) })
	hs := &harness{t: t, app: app, screen: screen}
	hs.waitRe(`Updated:\s+\d\d:\d\d:\d\d`)
	time.Sleep(time.Duration(waitAPI()) * time.Second) // the API metrics come after the releases
	shot := func(title string) { fmt.Printf("===== %s\n%s\n", title, hs.text()) }
	cmd := func(c string) { hs.keys(":" + c); hs.key(tcell.KeyEnter); time.Sleep(400 * time.Millisecond) }
	shot("products")
	cmd("health")
	shot("health")
	cmd("eol")
	shot("eol")
	cmd("asterisk")
	hs.waitRe(`<asterisk>`)
	time.Sleep(4 * time.Second) // the card loads the full API data
	for _, tab := range "23456" {
		hs.keys(string(tab))
		time.Sleep(300 * time.Millisecond)
		shot("asterisk tab " + string(tab))
	}
	// NVD without a key: a request per 6.5 seconds, cached for a day
	for deadline := time.Now().Add(5 * time.Minute); time.Now().Before(deadline); time.Sleep(2 * time.Second) {
		done := false
		app.QueueUpdate(func() { done = h.NVDTotal > 0 && h.NVDDone == h.NVDTotal && h.KEV != nil })
		if done {
			break
		}
	}
	time.Sleep(3 * time.Second) // EPSS
	hs.key(tcell.KeyEscape)
	cmd("security")
	shot("security")
	hs.keys("R")
	time.Sleep(300 * time.Millisecond)
	shot("security by risk")
	cmd("kev")
	shot("kev")
	cmd("mine")
	shot("mine")
	cmd("asterisk")
	hs.keys("6")
	time.Sleep(300 * time.Millisecond)
	shot("asterisk security")
	if _, ok := h.Inventory["asterisk"]; ok {
		cmd("upgrade asterisk")
		time.Sleep(15 * time.Second)
		shot("upgrade asterisk")
	}
	for deadline := time.Now().Add(4 * time.Minute); time.Now().Before(deadline); time.Sleep(2 * time.Second) {
		done := false
		app.QueueUpdate(func() { done = h.NewsLoaded && h.MarketDone && h.StdDone && h.AdoptionDone })
		if done {
			break
		}
	}
	time.Sleep(2 * time.Second) // the radar
	for _, c := range []string{"news", "radar", "ietf", "events", "market"} {
		cmd(c)
		shot(c)
	}
	cmd("opensips")
	hs.keys("7")
	time.Sleep(300 * time.Millisecond)
	shot("opensips news")
	hs.keys("8")
	time.Sleep(10 * time.Second)
	shot("opensips docker")
	app.QueueUpdate(func() {
		for _, e := range h.NewsErrs {
			fmt.Println("news error:", e)
		}
	})
	for _, st := range h.Ordered() {
		if st.NVDErr != nil {
			fmt.Println("nvd error:", st.Product.ID, st.NVDErr)
		}
	}
	for _, st := range h.Ordered() {
		if len(st.GH.Errs) > 0 {
			fmt.Println("api error:", st.Product.ID, st.GH.Errs)
			break
		}
	}
}
