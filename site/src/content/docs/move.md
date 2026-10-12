---
title: Move, resize, focus
description: The three write operations, and what happens when the OS says no.
---

```sh
windowctl move --app "Google Chrome" --monitor 2 --zone 2B
windowctl resize --app "Google Chrome" --w 900 --h 700
windowctl focus --title "jira"
```

- **`move`** places a window: by [zone](/windowctl/zones/), by split, or by coordinates.
- **`resize`** keeps the window's current X/Y and changes only its size.
- **`focus`** raises the window and activates its application.

## When the OS clamps the move

Applications get a say. Chrome refuses widths below roughly 500px; some windows
resist leaving the menu-bar inset. windowctl does not pretend that worked — it
exits non-zero and reports the bounds the window actually settled at:

```
windowctl: requested 512x640 at (3840,30), OS clamped to 576x615 at (3840,55)
           (likely a minimum-window-size constraint)
```

The bounds are read **after** the window settles, not mid-animation, so the
number you get is the number that stuck.

## macOS specifics

Move and focus go through the Accessibility API: position and size are set as a
pair (a sandwich, so cross-display moves land correctly), and focus is a raise
action plus an application activate. The bridge from a window ID to its
Accessibility element is a fresh lookup under the owning process, disambiguated
by title and bounds — no caching that can go stale, no private API.

Without permission, these exit with a clear `Accessibility permission denied`
error. See [Permissions](/windowctl/permissions/).
