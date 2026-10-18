# ADR 0012 — the product site (winctl.deemwar.com)

- Status: Accepted (spike S9 PASS) — the site was removed from this fork
- Date: 2026-07-26

## Context
`windowctl` has a README and a specs folder and nothing a human lands on. The
product now spans two surfaces (window control + device streaming) that a README
cannot show: zones are visual, and "drive a real app's mic, camera, screen and
keyboard from a socket" needs to be *demonstrated*, not listed.

Owner constraints, taken from how the existing sites were built and reviewed:
- **Style = `toolnexus`** (`nexus-workspace/toolnexus/site`) — rated "extreme
  good" and already reused for `cite-nexus` and `ctx-optimize`. That is Astro +
  Starlight, Inter Variable + JetBrains Mono, the deemwar token system in one
  `deemwar.css`, dark-first charcoal, single accent.
- **Data over adjectives.** "like steve jobs — more data then obscured verbose";
  "subtle, to the point, honest, nothing marketing, no bluff."
- **Hero + Quickstart are mandatory** — a previous site was rejected for
  shipping without them.
- **Responsive is not optional** — a hard-coded `max-width` on a hero once broke
  a launch page.
- **Cloudflare**, not GitHub Pages (toolnexus is GH Pages; `aiceo.deemwar.com`
  and `stocks.deemwar.com` are the Cloudflare precedent to follow).

## Decision
**Astro + Starlight, in-repo at `site/`, deployed to Cloudflare Pages as
`winctl-deemwar`, served at `winctl.deemwar.com`.**

- **Framework: Starlight.** Same as toolnexus, so the layout, sidebar, search
  (pagefind), and `llms.txt` behavior are identical by construction rather than
  by imitation. A splash-template `index.mdx` carries the marketing hero; the
  rest is real documentation.
- **In-repo at `site/`,** mirroring toolnexus's layout. The site's claims are
  then editable in the same commit as the code that makes them true.
- **`base: '/'`**, because this is an apex-style custom domain, not a project
  path — the one config that must differ from toolnexus, and the one most likely
  to be copied wrong (every internal link breaks).
- **Design tokens forked, not copied.** Same font stack, same charcoal
  neutrals, same structure — but the accent moves off toolnexus blue
  (`#3478F6`) to a **tiling teal (`#0FB9A8`)**, so the two products are
  recognisably siblings and not the same page. The site's signature motif is a
  **CSS-grid zone diagram** that shows what `--zone 2A` actually means: the
  product's own subject matter used as its visual language, which is why the
  colour is not arbitrary.
- **Deploy: `wrangler pages deploy`** against the existing account, custom
  domain attached by API, DNS a proxied `CNAME` — the same shape as the other
  live deemwar Pages sites.
- **No client-side JS beyond what Starlight ships.** The zone diagram, the
  monitor mock, and the terminal blocks are static HTML/CSS. A page about a
  native binary should not need a framework to render a rectangle.

## Alternatives rejected
- **A hand-rolled single-page site.** Faster to write, but loses search, the
  sidebar, `llms.txt`, and the shared look — and would drift from the house
  style the owner has repeatedly asked to reuse.
- **GitHub Pages** (as toolnexus does) — the ask is explicitly Cloudflare, and
  Pages gives us the deemwar domain, instant rollback, and preview deploys.
- **Docs-only, no marketing hero** — rejected by prior review; the landing page
  is the point.
- **Marketing-only, no docs** — the audience is developers who will immediately
  want flags. The README's content is the site's spine.

## Consequences
- Two places now describe the CLI surface (README + site). The site is generated
  from the same facts but is not literally the README — accept the duplication,
  and treat a flag change as a three-file change (code, README, site).
- `site/node_modules` and `site/dist` must be gitignored; the Go build is
  untouched (`go build ./...` never sees `site/`).
- Publishing is a separate pipeline from `task release` — the site can ship
  without a version bump, and a version bump does not force a site deploy.

## Spike S9 — Cloudflare path
See `spikes/s9_site_deploy.sh`: verify the API token, resolve the `deemwar.com`
zone, confirm `winctl.deemwar.com` is unclaimed, confirm `wrangler` is present,
and build the site. PASS = all green before any live resource is created.
