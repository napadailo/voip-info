package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/napadailo/voip-info/internal/config"
	"github.com/napadailo/voip-info/internal/digest"
	"github.com/napadailo/voip-info/internal/hub"
	"github.com/napadailo/voip-info/internal/secret"
)

const cliUsage = `Commands without the interface (cron, systemd timers):

  voip-info digest [-days N] [-mine | -all] [-out FILE]
      the digest of N days (digest.days of the config, else 7) to stdout or a file
  voip-info secrets
      where each secret was found (secrets: of the config), not the values

The tokens are where secrets: of the config says: a variable, a file, a command
(voip-info -sample-config).
`

// runCLI runs a subcommand; it returns the exit code.
func runCLI(args []string, cfg *config.Config, h *hub.Hub) int {
	switch args[0] {
	case "digest":
		return runDigest(args[1:], cfg, h)
	case "secrets":
		return runSecrets(cfg)
	case "help":
		fmt.Print(cliUsage)
		return 0
	}
	fmt.Fprintf(os.Stderr, "voip-info: unknown command %q\n\n%s", args[0], cliUsage)
	return 2
}

func runDigest(args []string, cfg *config.Config, h *hub.Hub) int {
	fs := flag.NewFlagSet("digest", flag.ContinueOnError)
	days := fs.Int("days", cfg.Digest.Days, "period in days")
	mine := fs.Bool("mine", false, `only "my stack"`)
	all := fs.Bool("all", false, "the full digest even when digest.mine of the config is set")
	out := fs.String("out", "", "write the Markdown to a file")
	if fs.Parse(args) != nil {
		return 2
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
	defer cancel()
	h.LoadAll(ctx, false)
	md := digest.Build(h, digest.Options{Since: time.Now().AddDate(0, 0, -*days), Label: digest.Label(*days),
		Mine: *mine || cfg.Digest.Mine && !*all})
	if *out == "" {
		fmt.Println(md)
		return 0
	}
	if err := os.WriteFile(*out, []byte(md), 0o644); err != nil {
		fmt.Fprintln(os.Stderr, "voip-info:", err)
		return 1
	}
	return 0
}

// runSecrets tells where each secret was found, never the value.
func runSecrets(cfg *config.Config) int {
	for _, name := range secret.Names {
		s, f := cfg.Secrets.Of(name), secret.Lookup(name)
		var where string
		switch f.From {
		case "env":
			where = "found in $" + s.Env
		case "file":
			where = "found in the file " + s.File
		case "command":
			where = "found by the command"
		default:
			where = "not found"
			if f.Err != nil {
				where += ": " + strings.ReplaceAll(f.Err.Error(), "\n", "; ")
			}
		}
		fmt.Printf("%-9s %s\n", name, where)
		fmt.Printf("%-9s env %q, file %q, command %q\n", "", s.Env, s.File, s.Command)
	}
	return 0
}
