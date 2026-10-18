---
title: Quickstart
description: Install windowctl, grant permission once on macOS, and move your first window.
---

## Install

```sh
npm install -g @muthuishere/windowctl
```

npm resolves one platform sub-package (`@muthuishere/windowctl-darwin-arm64` and
friends) and execs the native binary. There is no postinstall script and no
download step at install time — the binary is in the package.

Other routes:

```sh
go install github.com/Jiafei-Queen/windowctl/cmd/windowctl@latest   # Go
# or grab a binary from GitHub Releases
```

## macOS: grant Accessibility once

Listing windows needs nothing. Moving, resizing or focusing them needs the
Accessibility permission, because that is the API doing the work.

```sh
windowctl permissions            # opens the system prompt
windowctl permissions --status   # script-friendly; never prompts
```

Grant it to the app that *runs* windowctl — your terminal, or your agent's
host process. If it is denied you get a descriptive error, not a silent no-op.

## Move something

```sh
windowctl windows list                          # what is open
windowctl move --app "Google Chrome" --zone 1A  # left half
windowctl move --title "Terminal" --zone 1B     # right half
```

`--app` is a case-insensitive **exact** match on the OS-reported name (VS Code
reports `Code`, Chrome reports `Google Chrome`). `--title` is a case-insensitive
**substring**. When several windows match, the first is used — so list before
you move if you are not sure.

## Then keep going

- [Zones & splits](/windowctl/zones/) — the rectangle vocabulary
- [Layouts in one call](/windowctl/batch/) — apply a whole desk arrangement
- [Agent skill](/windowctl/agent-skill/) — drive it in plain English
