# window-control

```bash
windowctl install --skills
```

That installs the bundled `window-ctl-skill` into `~/.claude/skills/window-ctl-skill`, and also into `~/.agents/skills/window-ctl-skill` when `codex` is on PATH.

If you want the registry-hosted copy instead:

```bash
npx skills add muthuishere-agent-skills/window-control
```

**An agent skill that moves your windows.**

Move them. Swap them between monitors. Arrange a whole layout in one shot.

```
"split chrome left, slack right"
"send vscode to my external monitor"
"snap this window to the right half"
"save this layout as work-mode"
"restore work-mode"
"pull every window back to the laptop"
```

## Requires

The CLI it drives:

```bash
npm install -g @muthuishere/windowctl
```

macOS: run `windowctl permissions` once and approve in **System Settings → Privacy & Security → Accessibility** for the terminal / IDE you launch the agent from.

## Two repos

- [**windowctl**](https://github.com/Jiafei-Queen/windowctl) — the cross-platform CLI (macOS / Windows / Linux). Where the actual window operations happen.
- [**window-control**](https://github.com/muthuishere-agent-skills/window-control) — this skill. Routes natural language into `windowctl` calls.

## License

MIT.
