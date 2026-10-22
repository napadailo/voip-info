// voip-info is a terminal UI over the releases, security and news of the
// open-source VoIP ecosystem.
package main

import (
	"flag"
	"fmt"
	"os"
	"runtime/debug"

	"github.com/napadailo/voip-tui/i18n"
	"github.com/napadailo/voip-tui/tui/theme"

	"github.com/napadailo/voip-info/internal/config"
	"github.com/napadailo/voip-info/internal/hub"
	"github.com/napadailo/voip-info/internal/store"
	"github.com/napadailo/voip-info/internal/ui"
)

var version = "dev"

// go install module@version does not pass -ldflags; the module version recorded in the binary stands in for it.
func init() {
	if bi, ok := debug.ReadBuildInfo(); ok && version == "dev" && bi.Main.Version != "" && bi.Main.Version != "(devel)" {
		version = bi.Main.Version
	}
}

func main() {
	var (
		cfgPath   = flag.String("config", config.DefaultPath(), "config file")
		command   = flag.String("c", "", "initial command, e.g. -c releases or -c asterisk")
		themeName = flag.String("theme", "", "color theme")
		lang      = flag.String("lang", "", "interface language (en, ru), overrides the config and LANG")
		showVer   = flag.Bool("version", false, "print version")
		sample    = flag.Bool("sample-config", false, "print a sample config file")
	)
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "voip-info %s - releases, security and news of the VoIP ecosystem\n\nUsage: voip-info [flags] [command]\n\n", version)
		flag.PrintDefaults()
		fmt.Fprintf(os.Stderr, "\nConfig: %s (voip-info -sample-config prints an example)\n\n%s", config.DefaultPath(), cliUsage)
	}
	flag.Parse()
	if *showVer {
		fmt.Println(version)
		return
	}
	if *sample {
		fmt.Print(config.Sample)
		return
	}
	_ = i18n.Set(i18n.FromEnv())
	cfg, err := config.Load(*cfgPath)
	if err != nil {
		fatal(err)
	}
	cfg.Apply() // the secrets before the hub: its fetcher takes the GitHub token
	dir, err := cfg.Data()
	if err != nil {
		fatal(err)
	}
	st, err := store.Open(dir)
	if err != nil {
		fatal(err)
	}
	defer st.Close()
	h, err := hub.New(st)
	if err != nil {
		fatal(err)
	}
	h.DataDir = dir
	if err := h.LoadInventory(); err != nil {
		fmt.Fprintln(os.Stderr, "inventory:", err)
	}
	if err := h.LoadWatch(); err != nil {
		fmt.Fprintln(os.Stderr, "watch:", err)
	}
	if err := theme.Register(cfg.Themes); err != nil {
		fatal(err)
	}
	// -flag, then the config, then what :theme / :lang chose last time
	themeSel := pick(*themeName, cfg.Theme, setting(st, "theme"), theme.Default)
	langSel := pick(*lang, cfg.Language, setting(st, "language"), i18n.FromEnv())
	if err := i18n.Set(langSel); err != nil {
		fatal(err)
	}
	if flag.NArg() > 0 {
		code := runCLI(flag.Args(), cfg, h)
		st.Close()
		os.Exit(code)
	}
	ui.Version = version
	app := ui.New(cfg, h)
	if err := app.SetTheme(themeSel); err != nil {
		_ = app.SetTheme(theme.Default)
	}
	if err := app.Start(*command); err != nil {
		fatal(err)
	}
}

func setting(s *store.Store, key string) string {
	v, _ := s.Setting(key)
	return v
}

func pick(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, "voip-info:", err)
	os.Exit(1)
}
