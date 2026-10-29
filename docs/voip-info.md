# voip-info

A k9s-style terminal interface over the open-source VoIP/RTC ecosystem: releases and branches of 66
products (Asterisk, FreeSWITCH, Kamailio, OpenSIPS, rtpengine, WebRTC servers, SIP stacks, voice AI,
testing tools), project activity on GitHub, vulnerabilities ranked by real risk, the versions you run,
news, the market, IETF standards and conferences. A digest of a period goes to the screen or to Markdown.

Russian version: [voip-info.ru.md](voip-info.ru.md).

```
Tokens:        GitHub yes · NVD yes        <0> all           <5> SIP Stack     <enter> card
API:           4972/5000 reset 17:30       <1> PBX           <6> Monitoring    <b> browser
Category:      PBX · 8 products            <2> SIP Proxy     <7> CPaaS/AI      <o/O> sort
New releases:  52 · news 24h: 31           <3> Media/RTP     <8> Codec/DSP     <w/W> watch/my stack
CVE/30d:       59 (high+ 31) · KEV 3       <4> WebRTC        <9> Testing
Updated:       16:48:31 · en
╔══════════════════════════════════════ Products[8] PBX ═══════════════════════════════════════╗
║ NAME         LATEST   CHANGES    PRE         AGE      STARS     COMMITS          CVE 1Y  HEALTH   ║
║ ◆ Asterisk   23.5.0   SEC FIX    24.0.0-rc3  55d      3.6k +8   ▇▆█▅▅▃▇▃ +42%    28      80       ║
║ FreeSWITCH   1.10.12  FIX                    4mo      4.1k      ▃▄▅▃▃▄▂▃ -12%    6       71       ║
║ ◆ FreePBX    17.0.33  SEC                    9d       1.3k      ▂▃▂▁▃▂▄▃         77      70       ║
╚══════════════════════════════════════════════════════════════════════════════════════════════╝
```

## Install

Binaries for Linux and macOS are on [Releases](https://github.com/napadailo/voip-info/releases). Or with
Go 1.22+:

```sh
go install github.com/napadailo/voip-info/cmd/voip-info@latest
```

From a checkout:

```sh
make build      # bin/voip-info
make install    # into $GOPATH/bin
make test
```

voip-info is one file: the catalog of products, feeds and vendors is built in, the database is SQLite in
pure Go, no CGO.

## First run

```sh
voip-info                    # the interface, products first
voip-info -lang ru           # the interface language
voip-info -c releases        # the view or card to start with: -c asterisk
voip-info -theme nord        # the theme
voip-info -sample-config     # an example config
voip-info -version
```

On start voip-info loads in the background:

- the releases of every product from GitHub `releases.atom` (no rate limit);
- GitHub API metrics: stars, commits, issues, pull requests, milestones, security advisories (with a token);
- vulnerabilities from NVD, the CISA KEV catalog and EPSS scores;
- news, vendor quotes, IETF drafts and RFCs, IETF meetings, conference news;
- npm, PyPI and Docker Hub package stats.

Everything is cached in the database; repeated GitHub requests carry an ETag and do not use the rate
limit. Without an NVD key the first load of vulnerabilities takes about 3 minutes (NVD allows 5 requests in
30 seconds), with a key about 20 seconds; the header shows the progress (`NVD 12/28`). `r` reloads
everything from the network.

### The header

| Line | What it shows |
|---|---|
| Tokens | whether there is a GitHub token (60 requests an hour without one) and an NVD key |
| API | what is left of the GitHub API limit and when it resets |
| Category | the category and its number of products |
| New releases | unseen releases and the news of 24 hours |
| CVE/30d | new vulnerabilities of 30 days, the high/critical among them, the catalog's vulnerabilities in CISA KEV; the NVD progress |
| Updated | the time of the last load and the interface language |

## Configuration

The config is `~/.config/voip-info/config.yaml` (`$XDG_CONFIG_HOME/voip-info/config.yaml`; `$VOIP_INFO_CONFIG`
or `-config` changes the path). It may not exist: everything works without it, but without tokens.

```yaml
theme: nord                # :theme lists them, custom ones below
language: en               # en, ru; default: from LANG; :lang switches
dataDir: ~/.local/share/voip-info   # cache and state
digest:
  days: 7                  # default period of :digest and voip-info digest
  mine: true               # only "my stack" by default
secrets:                   # where the tokens are; a secret without settings is not read
  github:   {command: gh auth token}
  nvd:      {command: security find-generic-password -s voipinfo-nvd -w}
themes:
  noc: {base: nord, accent: "#ffaf00"}
```

The theme and the language chosen with `:theme` and `:lang` are remembered. Precedence: the flag, the
config, the last choice, the default.

### Secrets

Tokens are never kept in the config or in the code: the config only says where to take them. Each secret
has three sources, tried in order; the first that is not empty wins:

| Source | What it is | Where it fits |
|---|---|---|
| `env` | an environment variable | a one-off run, CI |
| `file` | a file holding the secret (`~/` is the home directory) | a Linux server (a file of mode `0600`), Docker and Kubernetes secrets (`/run/secrets/…`), systemd `LoadCredential` |
| `command` | a command printing the secret (run by `sh -c`) | any secret manager: `pass show …`, `op read …`, `secret-tool lookup …`, `vault kv get …`; the macOS keychain |

A secret without settings does not exist: no default variable names, no implicit `gh auth token`.

| Secret | What for |
|---|---|
| `github` | GitHub API metrics: 5000 requests an hour instead of 60. A fine-grained token with "Public repositories (read-only)" is enough; the token of `gh` is `command: gh auth token` |
| `nvd` | an NVD API key: vulnerabilities load 10 times faster. Free: https://nvd.nist.gov/developers/request-an-api-key |

The macOS keychain:

```sh
security add-generic-password -U -a "$USER" -s voipinfo-github -w     # asks for the token; -U replaces it
```

```yaml
secrets:
  github: {command: security find-generic-password -s voipinfo-github -w}
```

While the Mac is unlocked the keychain is open, and cron jobs see the secrets too.

`voip-info secrets` tells where each secret was found, or why not, without printing the values:

```
github    found by the command
nvd       found in the file /etc/voip-info/nvd.key
```

### Data

The data directory is `~/.local/share/voip-info` (`$XDG_DATA_HOME/voip-info`, `dataDir` of the config or
`$VOIP_INFO_DATA`):

| File | What it holds |
|---|---|
| `voip-info.db` | the HTTP cache, the releases seen, daily snapshots of metrics (stars, the health index, pulls), the theme and the language chosen |
| `inventory.yaml` | my versions (`:have`), editable by hand |
| `watch.yaml` | the "my stack" subscription (`w`, `:watch`) |
| `exports/` | saved digests (`:export`, `e` in the digest) |

On the first run my versions and the subscription are imported from `~/.voipinfo` (or `$VOIPINFO_HOME`),
the data of the former Python version.

## Keys

| Key | Action |
|---|---|
| `j` `k` `g` `G` `ctrl-d` `ctrl-u` | move |
| `enter` | open: a product card, a vulnerability, a news item, a release |
| `esc` / `q` | back (`esc` first clears the filter) |
| `/` | filter rows (regex, `!` inverts); search in text views, then `n` / `N` |
| `:` | command prompt with completion and history |
| `o` / `O` | sort column / order; empty cells always last |
| `h` / `l` | scroll wide tables sideways |
| `b` | open the page of the row in the browser: the release, the vulnerability, the news item, the repository |
| `0`-`9` | the category (`0` all); in `:news` the kind of feeds |
| `w` | watch the product, topic, vendor or working group of the row (again: unwatch) |
| `W` | only my stack in every view |
| `r` | reload everything from the network |
| `n` | mark all releases seen |
| `R` | in `:security`, sort by real risk |
| `?` | help: keys and commands |
| `:theme`, `:lang` | theme with live preview, interface language |
| `:q` | quit |

Each view lists its own keys in the header. Categories: `1` PBX, `2` SIP Proxy, `3` Media/RTP, `4` WebRTC,
`5` SIP Stack, `6` Monitoring, `7` CPaaS/AI, `8` Codec/DSP, `9` Testing, `0` all; also as commands:
`:pbx`, `:sip`, `:media`, `:webrtc`, `:stack`, `:mon`, `:ai`, `:codec`, `:test`, `:all`. `:<id>` opens a
product card, for example `:asterisk`.

## Commands

| Command | Aliases | What it does |
|---|---|---|
| `:products` | `p`, `prod` | products |
| `:releases` | `rel` | the release feed |
| `:health` | `hl` | the health index by component |
| `:eol` | `lts`, `support` | branch support |
| `:deps` | `graph` | the dependency graph |
| `:security` | `sec`, `cve`, `vuln` | vulnerabilities |
| `:kev` | `exploited` | exploited VoIP vulnerabilities from CISA KEV |
| `:mine` | `my`, `inventory`, `inv` | my versions |
| `:have <id> <version> [note]` | | record a version; `:have <id> -` removes it |
| `:upgrade <id> [from] [to]` | `up` | the upgrade path within a branch |
| `:news [kind]` | `ns` | news; kind: project, industry, market, regional, community, lists |
| `:radar` | `topics`, `trends` | the topic radar |
| `:ietf` | `rfc`, `standards` | IETF drafts and RFCs |
| `:events` | `conf`, `calendar` | conferences and IETF meetings |
| `:market` | `vendors`, `biz` | vendors: quotes and news |
| `:watch [q]` | | my stack; with an argument, subscribe |
| `:unwatch <q>` | | drop a subscription |
| `:digest [days] [mine\|all]` | | the digest of a period |
| `:export [days] [mine]` | | save the digest as Markdown |
| `:reload` | | reload everything from the network |
| `:seen` | | mark all releases seen |
| `:theme [name]`, `:lang [en\|ru]` | | the theme, the language |

## Views

### `:products`

A row per product, the newest releases first.

| Column | What it is |
|---|---|
| NAME | `◆` in my versions, `◉` watched or a dependency of a watched product; green: unseen releases; `…` loading, `!` the feed failed |
| CATEGORY | the category of the catalog |
| LATEST | the latest stable release; for projects without releases the latest commit, `⎇ sha` |
| CHANGES | the release note tags of that release, `SEC` `BRK` `DEP` `FEAT` `FIX`, each its own color; `-` no notes |
| PRE | a pre-release (rc, beta) newer than the stable one |
| AGE | time since the latest stable release: green up to 30 days, plain and yellow up to a year, red beyond |
| NEW | unseen releases (the first run counts the releases of 14 days as new; `n` marks all seen) |
| CADENCE,D | the median interval between stable releases, in days |
| STARS | stars and their growth over a week (`17k +18`), from daily snapshots |
| COMMITS | commits of 8 weeks as a sparkline and the trend of 12 weeks against the 12 before (none when those had fewer than 10 commits) |
| CVE 1Y | own vulnerabilities of a year, colored by the worst; `0` a source and none; empty, no source |
| CVE DEPS | high/critical vulnerabilities of the dependencies of 180 days without a fix: `ok`, `3 ✗` waiting for a release of the product, `2 ↻` update the dependency on its own |
| HEALTH | the health index, 0-100 |
| STATUS | by the latest stable release: hot up to 30 days, active up to 4 months, slow up to a year, stale beyond |

### `:releases`

Every release of every product (about the last 10 of each), the newest first: date, age, product, tag,
branch, stable/pre, the change tags and the upgrade risk. `●` marks the unseen. `enter` opens the product
card on that release. `/SEC` keeps the releases with security fixes.

### `:health`

The index and its six components per product as bars, the change over a week, the share of the weight
computed from data. See [The health index](#the-health-index).

### `:eol`

Per branch: kind (LTS / standard), the latest release, the x.0 date, the end of full support, EOL, days to
EOL, the status and the next change. See [Branch support](#branch-support).

### `:deps`

The links "product → dependency" with the kind, the dependency's latest release and the impact radius: how
many products inherit a vulnerability of the dependency over strong links. See
[The dependency graph](#the-dependency-graph).

### `:security`

The vulnerabilities of every product from NVD and GitHub Security Advisories, merged by CVE: date,
severity, CVSS, EPSS, KEV, product, how many products inherit it, ID, summary, source. The newest first;
`R` sorts by real risk: exploited (KEV) first, then the probability of exploitation (EPSS), then CVSS.
`enter` opens the vulnerability: the threat, the vulnerable version ranges and where fixed, who inherits
it, whether it affects your version, the description.

### `:kev`

VoIP and UC entries of the [CISA KEV](https://www.cisa.gov/known-exploited-vulnerabilities-catalog)
catalog (Asterisk, FreePBX, Mitel, Avaya, 3CX, Cisco UC, SBCs…), vendors outside the catalog included:
the date added, CVE, vendor, product, ransomware, EPSS and the product of the catalog the CVE belongs to.

### `:mine`, `:have`, `:upgrade`

`:have asterisk 20.18.0 lab` records a version, `:have asterisk -` removes it; the list is
`inventory.yaml` in the data directory.

| Column | What it is |
|---|---|
| BRANCH, SUPPORT | the branch of your version and its status from `:eol` |
| LATEST IN BRANCH, BEHIND | where to update without changing the branch, how many releases came after yours |
| CVE, HIGH+, KEV | the vulnerabilities that affect your very version (by the version ranges of GitHub advisories and NVD CPEs) |
| UPDATE FIXES | how many of them the latest of the branch closes |
| NO DATA 1Y | vulnerabilities of a year without version ranges or in FreePBX modules: worth a check by hand |
| CVE DEPS | vulnerabilities of the dependencies of a year without a fix |
| ACTION | the recommendation |

The recommendation, by the first rule that holds: a vulnerability in KEV, "URGENT"; high/critical,
"update"; an unsupported branch, "plan the migration"; newer releases, "update available"; a
security-only branch, a warning; else "up to date". Colors: red, yellow, blue, green.

`enter` shows the summary of the version over its vulnerabilities, each with why it is there ("fixed in
3.6.9", "no version data", "PJSIP ✗ waiting for a release"); `enter` on one opens it. `u` opens the upgrade
path, `c` the product card on its Security tab.

`:upgrade <id> [from] [to]` (or `u` in `:mine`): every release of the branch between the versions (older
history comes from `tags.atom` when the feed falls short), the risk, the releases with security fixes and
with breaking changes, the CVEs it closes and those that remain, every upgrade note and breaking change by
release. For final Asterisk releases the full changelog comes from their "Full ChangeLog" link.

### `:news`

24 feeds: project blogs, industry press, Google News (market, voice AI, security), Hacker News, the
Kamailio and OpenSIPS lists (messages grouped into threads), the Asterisk forum, Russian-language sources
and Kazakhstan. News are tagged with the products they name. `1`-`6` pick the kind of feeds: project,
industry, market, regional, community, lists; `0` all of them. `enter` shows an item whole.

### `:radar`

19 topics (voice AI, STIR/SHAKEN, fraud, Media over QUIC, WebRTC, encryption, post-quantum cryptography,
codecs, IMS/5G, RCS, Teams, Kubernetes, the copper switch-off, regulation, contact centers, emergency calls,
STT/TTS, vulnerabilities, deals): mentions in 7 and 30 days, the trend (the last week against the average
week of the month), a sparkline of 12 weeks, the sources (news, release notes, IETF, lists), the products a
topic comes up with. Topics are matched by keywords in English and Russian. `enter` opens a topic: its
summary over its mentions, `b` opens one.

### `:ietf`

Drafts (of a year) and RFCs (of two years, with their publication date) of the sipcore, stir, mmusic,
avtcore, moq, wish, ecrit and webtrans working groups from the datatracker: date, kind, group, document with
its revision, title, pages, when a draft expires. `enter` shows the abstract, `b` the document on the
datatracker.

### `:events`

IETF meetings with their dates and conferences (FOSDEM, Enterprise Connect, Kamailio World, OpenSIPS Summit,
ClueCon, AstriCon, RTC.ON) with their usual month and the latest mention in the news. `enter` on a
conference: its summary over its mentions.

### `:market`

15 vendors (Sangoma, Twilio, RingCentral, 8x8, Bandwidth, Ooma, Five9, Zoom, Vonage, 3CX, Avaya, Mitel,
Sinch, Vapi, Retell AI): the share price, its change over a month and 3 months, a sparkline, the news of 30
days (reprints folded), the latest headline. `enter` shows the quotes over the news.

### `:watch`

What my stack holds and why each is there: "subscription", "my versions", "dependency of Asterisk". See
[My stack](#my-stack).

## The product card

`enter` on a product, or `:<id>`. The header: name, category, repository, homepage, description; the latest
release and pre-release, cadence, releases in 90 days, live branches, status; stars, forks, open issues and
pull requests, commits of 12 weeks with the trend, language, license, the health index. `1`-`8` or `tab`
switch the tabs:

| Tab | What it shows |
|---|---|
| 1 Releases | releases with their change tags, the notes of the selected one beside; `enter` opens a release on the whole screen (search, `b`) |
| 2 Activity | issues and pull requests of 30 days: opened, closed, the median time to close, merged and rejected, authors, the share of the top author; `enter` opens an issue or a pull request with its description and labels |
| 3 Roadmap | the pre-release, milestones with progress, the support of the branches |
| 4 Stats | the health index by component with its basis, the index by day, the "Usage" line (npm, PyPI, Docker Hub), the repository metrics, commits of 52 weeks |
| 5 Deps | the tree of dependencies and consumers, fresh releases of the dependencies, the impact radius |
| 6 Security | a summary over the vulnerabilities: own ones and inherited ones with the state of their fix; `enter` opens one |
| 7 News | the product in the news |
| 8 Docker | images of the catalog and of the Docker Hub search: pulls, stars, updated, the product version in the image, the lag, architectures, the latest tags |

## My stack

My stack is what concerns you: the products of my versions, the subscription (`w` on a row of any view
or `:watch <id|topic|vendor|wg>`) and all the dependencies of those products. A product, a radar topic, a
vendor and an IETF working group can be watched; `:watch voice` finds a topic by a part of its name.

`W` keeps only my stack in every view: its products in the product lists, the news about them and about
the watched topics in `:news`, the watched topics, vendors and groups in `:radar`, `:market`, `:ietf`. The
mode is remembered. The digest gives my stack its first section; `:digest mine` is that section alone.

## The digest

`:digest [days] [mine|all]` is the whole digest of a period on one page: my stack, my versions, KEV,
vulnerabilities by risk, vulnerabilities flowing through dependencies, significant releases (new majors and
branches), pre-releases, patch releases, branch support deadlines, project signals (commits speeding up and
slowing down, the health index leaders and those at risk, usage, archived projects), the market, standards,
the radar, IETF meetings soon, news by kind without duplicates. In the digest `e` saves it as `.md`.

`:export [days] [mine]` saves the whole digest in `exports/` of the data directory.

## Without the interface

```sh
voip-info digest [-days N] [-mine | -all] [-out FILE]
voip-info secrets
```

`voip-info digest` loads everything and prints the whole digest to stdout, `-out` writes it to a file.
`-days` defaults to `digest.days` of the config, else 7; `-mine` keeps to my stack, `-all` gives everything
even when the config has `digest.mine: true`.

cron, Mondays at 9:00 (`crontab -e`, the path from `which voip-info`):

```
0 9 * * 1 /home/user/go/bin/voip-info digest -days 7 -mine -out /home/user/digest.md
```

With the secrets in the macOS keychain the Mac must be on and unlocked at that time. On Linux a systemd
timer and secrets in files fit better:

```ini
# ~/.config/systemd/user/voip-info-digest.service
[Service]
Type=oneshot
ExecStart=%h/go/bin/voip-info digest -days 7 -mine -out %h/digest.md

# ~/.config/systemd/user/voip-info-digest.timer
[Timer]
OnCalendar=Mon 09:00
Persistent=true
[Install]
WantedBy=timers.target
```

```sh
systemctl --user enable --now voip-info-digest.timer
```

## How it is computed

### The health index

0-100 from six components:

| Component | Weight | Scale |
|---|---|---|
| Release freshness | 25 | days since the latest stable release: ≤30 → 100, ≤90 → 85, ≤180 → 60, ≤365 → 30, beyond → 5 |
| Release cadence | 10 | the median interval: ≤30 → 100, ≤60 → 85, ≤120 → 60, ≤240 → 35, beyond → 10 (needs 3 releases) |
| Development pace | 20 | commits of 12 weeks on a log scale: 120+ → 100, 10 → about 50; a fall of more than half → ×0.75 |
| Responsiveness | 15 | issues of 30 days: half the share closed, half the median time to close (≤7 days → 100, ≤30 → 70, ≤90 → 40) |
| Community | 10 | authors of merged pull requests of 30 days: 1 → 15, 2 → 40, ≤4 → 60, ≤9 → 85, more → 100; one author over 60% → ×0.7 |
| Security | 20 | high/critical of a year: 0 → 100, ≤2 → 75, ≤5 → 55, ≤10 → 35, more → 15; −10 per critical (up to −30), −15 for inherited ones without a fix, −25 for KEV |

A component without data does not count; the result is pulled toward a neutral 50 in proportion to the
missing weight: `score = raw × coverage + 50 × (1 − coverage)`. Under 60% coverage the score is dimmed and
marked `~`. An archived repository gets 10 at most. Grades: A ≥ 80, B ≥ 65, C ≥ 50, D ≥ 35, F below. The
value is kept once a day, so its change over a week shows.

### Branch support

- **Asterisk**, the project's policy: even majors are LTS (4 years of full support and 1 of security
  fixes), odd ones standard (1 + 1), counted from x.0.0. A major with release candidates only is
  "upcoming".
- **Kamailio**: the two latest branches are maintained; an older one is security-only while it gets
  releases and EOL after half a year without.
- **Others**: a heuristic by the latest release of the branch: up to half a year "active?", up to a year
  "rare fixes?", beyond "probably EOL".

### The dependency graph

Links live in the catalog: `deps: ['pjsip:bundled', ...]`. The kind of a link tells how a vulnerability
reaches the consumer:

| Kind | Meaning | How the fix arrives |
|---|---|---|
| `bundled` | a copy of the sources inside (Asterisk ← pjproject) | only with a release of the consumer |
| `linked` | a library | a rebuild or a release of the consumer |
| `optional` | an optional module | the same, when the module is used |
| `platform` | the product is built on the other (FreePBX ← Asterisk) | by updating the platform |
| `runtime` | a separate service alongside (Kamailio + rtpengine) | by updating the service on its own |

The state of an inherited vulnerability is a heuristic over the consumer's release notes after the CVE was
published: `✗` waiting for a release (none came), `?` a release came without a mention of the CVE or the
dependency, `✓` probably fixed (mentioned), `↻` update on its own. Through `platform` and `runtime` the
state is the intermediate product's; weak links (`runtime`, `optional`) do not carry further than one hop.
The impact radius is how many products inherit a vulnerability over strong links (`bundled`, `linked`,
`platform`).

### Release note tags and the upgrade risk

`SEC` security fixes, `BRK` breaking changes and upgrade notes, `DEP` deprecations, `FEAT` new features,
`FIX` fixes. The upgrade risk: high (BRK), medium (FEAT or DEP), low (fixes only), `?` the notes do not
tell.

### Vulnerabilities

The sources are NVD (a keyword search per product with filters against false matches) and the GitHub
Security Advisories of the repository; entries of one CVE are merged, GitHub preferred (it names the fixed
versions). Whether a vulnerability affects a version comes from version ranges: GitHub advisories and NVD
CPEs. Without ranges a vulnerability counts as neither affecting nor safe: it goes to "no data".
[KEV](https://www.cisa.gov/known-exploited-vulnerabilities-catalog) is confirmed exploitation in attacks,
[EPSS](https://www.first.org/epss/) the probability of exploitation in the next 30 days.

FreePBX is made of modules (framework, core, endpoint, api, backup, cdr and more), each with its own version.
NVD often labels a vulnerability of a module with the CPE of FreePBX itself and the versions of the module:
CVE-2025-57819 is fixed in endpoint 16.0.89, not in FreePBX 16.0.89. So the module is taken from the CPE and
the description ("the endpoint module", "patched in endpoint versions"), and such a vulnerability is not
compared with your FreePBX version: its card names the module, `:mine` says "module endpoint: check its
version" (Admin → Module Admin). When NVD names no module, the vulnerability is compared as usual.

### Docker images

The product version in an image comes from its newest tag that looks like a version of the product (or
from its description). `ok` current in its branch, `-N` N releases of the branch behind, `old` the branch
gets no releases any more. A floating tag such as `4.0` counts as current for branch 4.0.

## Sources and limits

| Data | Source | Limit, cache |
|---|---|---|
| Releases, release notes | GitHub `releases.atom`, `tags.atom`, release pages | none; 30 minutes |
| Metrics, activity, milestones, advisories | GitHub REST API | 60 an hour without a token, 5000 with one; ETag |
| Vulnerabilities | NVD API 2.0 | 5 requests in 30 seconds, 50 with a key; a day |
| Exploited vulnerabilities | CISA KEV | 12 hours |
| Probability of exploitation | FIRST EPSS | a day |
| News, lists | RSS/Atom, Google News, HN Algolia, pipermail archives | 3 hours |
| Usage, Docker images | npm, pypistats.org, Docker Hub | a day; pypistats limits the rate strictly |
| Quotes | Nasdaq API | 6 hours |
| Standards, meetings | IETF datatracker | 12 hours, meetings a week |

## The catalog

The catalog is built in: `internal/catalog/data/`.

| File | What it holds |
|---|---|
| `catalog.yaml` | 66 products: `repo`, `category`, `desc`, `homepage`, `tag_filter` (which tags are releases), `nvd` / `nvd_filter` / `nvd_exclude` (the CVE search and its filters), `deps`, `track: commits` (projects without releases), `news_match`, `npm` / `pypi` / `docker` |
| `feeds.yaml` | news feeds: `kind`, `type` (rss, hn, pipermail), `product`, `filter` |
| `vendors.yaml` | vendors: `ticker`, the news `query`, `desc` |
| `events.yaml` | conferences: `where`, the usual `month`, `query` |

Descriptions (`desc`, `where`) come in the interface languages: `desc: {en: ..., ru: ...}`; the current
language is shown, English when it is missing.

## Themes and languages

`:theme` picks a theme with a live preview; custom themes go to the `themes` section of the config
(`noc: {base: nord, accent: "#ffaf00"}`). `:lang en|ru` switches the interface, the digest and the catalog
descriptions; the default language comes from `LANG`.

## Development

```sh
make test       # tests without the network: UI tests against stubs of every source
make vet        # go vet and gofmt
make live       # screens over the real data (needs the network)
make snapshot   # release archives into dist/ (goreleaser)
```

| Package | What it holds |
|---|---|
| `cmd/voip-info` | entry point, flags, commands without the interface |
| `internal/hub` | data and analytics without the interface: loading the sources, vulnerabilities, health, my versions, my stack |
| `internal/ui` | views, the product card, commands, the Russian catalog |
| `internal/digest`, `internal/export` | the Markdown digest, its rendering and saving |
| `internal/sources`, `news`, `ietf`, `market`, `adoption`, `dockerhub` | source clients |
| `internal/relnotes`, `deps`, `eol`, `health`, `radar`, `upgrade`, `inventory`, `watch` | analytics |
| `internal/config`, `internal/secret`, `internal/store`, `internal/fetch` | config, secrets, database, HTTP with a cache |

The interface is built on the [voip-tui](https://github.com/napadailo/voip-tui) framework. Every visible
string of the interface is English in the code with a translation in `internal/ui/locale_ru.go`; the catalog
test fails when a translation is missing or unused.

A release: a tag `vX.Y.Z` runs the `release` workflow; goreleaser builds archives for Linux and macOS
(amd64, arm64) and attaches them to the GitHub release.
