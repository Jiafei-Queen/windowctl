---
title: Agent skill
description: Drive windowctl in plain English — the skill ships inside the binary.
---

```sh
windowctl install --skills
```

That writes the `window-ctl-skill` agent skill into `~/.claude/skills/`, and
into `~/.agents/skills/` when Codex is present. Restart your agent session to
pick it up.

No network. No package manager. No repository to clone — the skill's text is
embedded in the binary you already installed.

## What it does

It is the routing and recipe layer. You say:

> *"split chrome left, slack right"*
> *"send vscode to my external"*
> *"take a screenshot of the second monitor"*
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
