---
title: Remote control
description: Share a monitor to any browser and drive it — click, type, keys — with one command.
---

```sh
windowctl remote                    # local URL, focused monitor
windowctl remote --monitor 2 --fps 15
windowctl remote --tunnel           # also a public https URL via cloudflared
```

It prints a URL. Open it in a browser — on your phone, on another machine —
and you see the screen live and can click, type and press keys on it. Ctrl-C
stops it.

## How it works

A small HTTP server streams JPEG frames of `screenshot` captures and forwards input to
the same `mouse` / `type` / `key` calls the CLI exposes. Because captures are
point-normalized, a click on the image maps straight to a screen point.

## Security

- Every request is gated by an access key. By default it is a **fresh random
  token per run**, embedded in the printed URL — the URL *is* the credential.
- `--key <k>` (or `$WINDOWCTL_REMOTE_KEY`) sets a stable key, for when the URL
  itself is stable (a named tunnel on your own domain).
- Without `--tunnel` it is reachable on localhost and your LAN only. With
  `--tunnel` it spawns `cloudflared` for a public `*.trycloudflare.com` URL —
  the key still gates it, but treat that URL like a password.

## Flags

| Flag | Default |
|---|---|
| `--monitor N` | the focused monitor |
| `--port N` | a free port |
| `--fps N` | 10 (1–30) |
| `--tunnel` | off |
| `--key K` | random per run |

From Go: `windowctl.Remote(RemoteOptions{…})` returns a handle you can stop.
