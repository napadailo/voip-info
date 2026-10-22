# voip-info

voip-info is a k9s-style terminal interface over the open-source VoIP/RTC ecosystem. It follows 66
products, from Asterisk, FreeSWITCH, Kamailio, OpenSIPS and rtpengine to WebRTC servers, SIP stacks,
voice AI frameworks and testing tools, and answers the questions of someone who runs them: what came out,
what is vulnerable and how badly, what my versions need, what the industry talks about.

The full documentation: [docs/voip-info.md](docs/voip-info.md), in Russian [docs/voip-info.ru.md](docs/voip-info.ru.md).

## What it shows

| View | What it shows |
|---|---|
| `:products` | the latest release and what is in it, pre-releases, cadence, stars, the commit trend, vulnerabilities of the product and of its dependencies, a health index |
| `:releases` | every release, newest first, with its change tags (security, breaking, deprecations, features, fixes) and the upgrade risk |
| `:security`, `:kev` | vulnerabilities from NVD and GitHub advisories ranked by real risk: CISA KEV, EPSS, CVSS; exploited VoIP vulnerabilities of vendors inside and outside the catalog |
| `:mine`, `:upgrade` | your versions in production: the vulnerabilities of that very version, newer releases, branch support, a recommendation; the upgrade path with the CVEs it closes and the breaking changes on the way |
| `:health`, `:eol`, `:deps` | a 0-100 health index by component, LTS and EOL dates of the branches, the dependency graph with how a vulnerability of a library reaches its consumers |
| `:news`, `:radar` | project blogs, industry press, Google News, Hacker News, mailing lists; 19 topics counted over the news, release notes and IETF drafts |
| `:ietf`, `:events`, `:market` | IETF drafts and RFCs of the VoIP and RTC working groups, conferences and IETF meetings, vendors' quotes and news |
| `:digest` | what changed over a period on one page, saved as Markdown |

A product card has eight tabs: releases with their notes, issues and pull requests, roadmap and branch
support, health and statistics, dependencies, vulnerabilities, news and Docker images.

## Install

Download a binary for Linux or macOS from [Releases](https://github.com/napadailo/voip-info/releases), or
install with Go 1.22+:

```sh
go install github.com/napadailo/voip-info/cmd/voip-info@latest
```

Or build from a checkout:

```sh
make build      # bin/voip-info
make install    # into $GOPATH/bin
make test
```

## Configuration

voip-info works without a config. `~/.config/voip-info/config.yaml` adds tokens and defaults;
`voip-info -sample-config` prints a commented example. Tokens are never kept in the config: it says where
each one is, an environment variable, a file or a command such as a secret manager or the macOS keychain,
and `voip-info secrets` tells where each was found.

```yaml
secrets:
  github: {command: gh auth token}
  nvd:    {file: ~/.config/voip-info/nvd.key}
```

A GitHub token raises the API limit from 60 to 5000 requests an hour; an NVD key loads vulnerabilities 10
times faster. Both are free.

## Without the interface

```sh
voip-info digest -days 7 -mine -out digest.md   # for cron or a systemd timer
voip-info digest -days 1                         # to stdout
```

## Keys

Vim keys, `enter` opens, `esc` goes back, `/` filters, `:` takes commands with completion, `o` sorts, `b`
opens the row in the browser, `0`-`9` pick a category, `w` watches a row and `W` keeps only my stack, `?`
lists the keys and commands. `:theme` and `:lang` switch the theme and the
language (English, Russian).

## Data

Releases come from GitHub Atom feeds, metrics from the GitHub API, vulnerabilities from NVD, GitHub Security
Advisories, CISA KEV and FIRST EPSS, standards from the IETF datatracker, package use from npm, PyPI and
Docker Hub. Everything is cached locally in SQLite under `~/.local/share/voip-info`.

voip-info is unofficial and not affiliated with the projects and vendors it follows; their names belong to
their owners.

## Built on

[voip-tui](https://github.com/napadailo/voip-tui), the framework of rtpengine-tui, opensips-tui and
homer-tui: the same keys, themes and languages.

## License

MIT, see [LICENSE](LICENSE).
