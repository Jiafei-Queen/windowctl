---
title: Agent skill
description: Drive windowctl in plain English — the skill ships inside the binary.
---

**You do not have to install it.** Every `windowctl` command (except
`install`, `uninstall` and `help`) makes sure the bundled skill is present and
current: if it is missing, or was written by an older binary, it is
(re)installed and one line goes to stderr — `agent skill updated at <path>`.
Otherwise it is silent, and a failure never fails your command. Upgrading
windowctl upgrades the skill.

```sh
export WINDOWCTL_NO_SKILL_AUTOINSTALL=1   # opt out of the automatic install
windowctl install --skills                # or install / refresh it explicitly
```

Auto-install also skips a symlinked skill directory (a developer's live link),
and stays off after you run `windowctl uninstall --skills` until the next
`install --skills`.

It writes the `window-ctl-skill` agent skill into `~/.claude/skills/`, and
into `~/.agents/skills/` when Codex is present. Restart your agent session to
pick it up.

No network. No package manager. No repository to clone — the skill's text is
embedded in the binary you already installed.

## What it does

It is the routing and recipe layer. You say:

> *"split chrome left, slack right"*
> *"send vscode to my external"*
> *"take a screenshot of the second monitor"*
> *"show me where the Allow button is"*
> *"click File in Notepad"*
> *"put everything back to work mode"*

and it resolves that to `windowctl` calls — after listing windows to confirm
exactly one thing matches, which is the step a human skips and regrets.

Every action is a single `windowctl` invocation you could have typed. The skill
never reaches into Quartz, Win32 or X11 itself.

## Managing it

```sh
windowctl install --skills --agents   # also ~/.agents/skills, even without codex on PATH
windowctl uninstall --skills [--agents]
```

`CLAUDE_SKILLS_DIR` / `AGENTS_SKILLS_DIR` override the target roots.

Two guards, because this writes and deletes under your home directory:

- A **symlinked** skill directory is a live working tree (that is how skill
  developers install). It is never followed and never replaced without
  `--force`.
- A directory is only deleted if its `SKILL.md` declares
  `name: window-ctl-skill`. Someone else's skill that happens to sit at that
  path is left alone, with an error explaining why.

## Why it is embedded

The skill documents the CLI's flags. When they lived in separate repositories,
nothing stopped the skill from confidently emitting a flag the CLI had dropped.
Same repo, same commit, same CI — that class of bug goes away.

## Find: name it, get its coordinates

The skill's `find` reference teaches the agent to locate things by name instead
of guessing pixels:

```sh
windowctl find --app TextEdit --text "Format" --json                # {Text, Label, Role, Source, Bounds, Center}
windowctl find --app Notepad --text "File" --role menuitem --json   # only menu items
windowctl find --title "Settings" --text "Allow" --exact --json     # whole label must equal it
windowctl find --app Preview --text "Total" --source ocr --json     # pixels only (PDFs, images, canvas)
```

Accessibility tree first (macOS AX including the app's menu bar, Windows UI
Automation), OCR second. Then the agent either clicks `Center` with
`windowctl mouse click` or points a human at it with
`windowctl draw --app <App> --find "<text>" --text "…"`. Not found exits 3 and
lists the closest labels it saw. See [Find on screen](/windowctl/find/).
