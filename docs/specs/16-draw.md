# Spec 16 — draw

- **Status:** Implemented (macOS, Windows); Linux: `--preview` only
- **Source:** FR-DRW-01, requirements §8, §11
- **Summary:** `windowctl draw` (library: `Draw` / `RenderDraw`) paints a big arrow with a text sign — optionally a ring or box around the target — over every window, so an agent can *show* a human where to click instead of describing it. Click-through, never takes focus, ends by itself. Look and vocabulary ported from bigarrow (github.com/franzenzenhofer/big-arrow-on-the-screen, MIT, macOS-only Swift); windowctl's version is one Go binary on macOS and Windows.

## Requirement: Pure-Go pixels, thin adapters

Placement, arrow geometry (flared root, bend/straight/zigzag/spiral shaft, notched head), the sign (text wrap ≤ 3 lines, font shrink to 24pt), borders and the drop shadow are computed and rasterized in the public package into one premultiplied RGBA image the size of the target monitor. Adapters only implement two primitives: `OverlayScale(bounds)` and `ShowOverlay(bounds, img, done)`.

### Scenario: identical rendering everywhere

- **Given** the same options and monitor geometry
- **When** `RenderDraw` runs on any OS (including Linux CI)
- **Then** it produces the same pixels `Draw` would show.

## Requirement: Never in the way, no privileges

The overlay window is transparent and click-through, never becomes key/active, stays out of the Dock/taskbar/Alt-Tab, and needs no admin rights and no macOS permission.

- **macOS:** borderless non-activating `NSPanel`, accessory activation policy, `kCGScreenSaverWindowLevel`, collection behaviour canJoinAllSpaces | stationary | ignoresCycle | fullScreenAuxiliary, `ignoresMouseEvents`. Pure C via `objc_msgSend`. Must run on the main thread (the darwin adapter locks the main goroutine to it in `init`).
- **Windows:** `WS_EX_LAYERED | WS_EX_TRANSPARENT | WS_EX_TOPMOST | WS_EX_TOOLWINDOW | WS_EX_NOACTIVATE` popup filled by `UpdateLayeredWindow` (per-pixel alpha), shown `SW_SHOWNOACTIVATE`, topmost re-asserted every 500 ms. Cannot appear on the UAC secure desktop or over exclusive-fullscreen games (OS limits).

### Scenario: focus is preserved

- **Given** a terminal has keyboard focus
- **When** `windowctl draw --at X,Y --text T` runs
- **Then** the arrow appears and the terminal keeps focus; clicks on the arrow reach the window below.

## Requirement: Accuracy — screenshot coordinates in, exact tip out

Targets use the same coordinate model as `screenshot`/`mouse`: global points (macOS) / physical pixels (Windows), monitor-relative with `--monitor`. A point target is hit exactly by the arrow tip; boxes at the edge midpoint nearest the sign; rings at the point facing the sign. Size metrics scale by DPI/96 on Windows (so a stroke is the same physical size at 100% and 200%); the image is rendered at the backing scale on macOS (crisp on Retina).

### Scenario: tip at the point

- **Given** `--at 700,500`
- **Then** the JSON result's `Tip` is `{700,500}` and the sign never covers the target or leaves the monitor.

## Requirement: Targets

Exactly one of `--at X,Y`, `--rect X,Y,W,H`, `--title`/`--app` (the window's bounds, boxed; first match wins), `--mouse`. `--monitor N` makes `--at`/`--rect` relative. Zero or several targets, or a missing `--text`, exit 2 with an actionable message; no matching window is `ErrNoMatch`.

## Requirement: Find (`windowctl find`, `draw --find`)

Locate text inside a matched window and return its exact global bounds: first the accessibility tree (macOS AX incl. the app's menu bar; Windows UI Automation `FindAll` over descendants), then OCR (macOS Vision `VNRecognizeTextRequest` at native resolution with per-word `boundingBoxForRange:`; Windows `Windows.Media.Ocr` over a 1:1 capture). Both are in-box, on-device, no admin. Ranking: exact label > whole word > prefix > substring; then interactive roles (button, menuitem, link, tab, checkbox, radio) > textfield > text; then top-to-bottom. A partial match inside a large element (text area, web view) is narrowed to the exact words by OCR of that element. `--role` filters accessibility matches; `--source auto|accessibility|ocr`; `--nth N`. Not found → exit 3 with the closest labels seen. `draw --find` resolves once (the detached child draws that rect) and boxes the element tightly with 8pt padding plus a short arrow (bigarrow style); `--style ring` draws an ellipse hugging the rect. Linux: not implemented.

## Requirement: Look

`--style arrow|ring|box`, `--shape bend|straight|zigzag|spiral`, `--size S|M|L`, `--border shadow|white-black|black`, `--corners round|sharp`, `--from auto|<8 sides>` (a preference — falls back to the roomiest side if the sign would not fit), `--color` preset or hex, `--text-color`, `--border-color`, `--edge-color`, `--font` (path or system font name; `windowctl draw fonts` lists them), `--font-scale`.

Palette is tuned so sign text reaches WCAG AA (≥ 4.5:1) where the hue allows (≥ 3:1 for every preset; text is ≥ 24pt heavy): red `E5261A` (default), orange `D93A00`, amber `FF8A00`, yellow `FFD60A`, green `1A7F37`, teal `00B5AD`, blue `0A6CFF`, purple `8E44FF`, pink `D81B60`, black `1C1C1E`, white `FFFFFF`. Text colour is whichever of white / ink has the higher contrast. Default font: Arial Rounded Bold (macOS), Segoe UI Black (Windows), embedded Go Bold as last resort.

## Requirement: User defaults (`~/.config/windowctl/fontconfig.json`)

Same path on macOS and Windows (`$WINDOWCTL_CONFIG` overrides the file, `$XDG_CONFIG_HOME` the directory). Keys: `font`, `fontScale`, `color`, `textColor`, `borderColor`, `edgeColor`, `size`, `style`, `shape`, `border`, `corners`, `from`. Precedence: flag / `DrawOptions` field > config > built-in. `windowctl draw config` prints the effective config; `--init [--force]` writes the default. `--no-config` / `DrawOptions.NoConfig` skips it. An explicitly named font that cannot be found is an error, never silently replaced.

## Requirement: Lifetime

Every drawing ends by itself: foreground `draw` blocks for `--duration` (default 8 s; Ctrl-C ends it). `--detach` re-runs the binary as a detached child (own session / `DETACHED_PROCESS`), waits until it has registered, prints where it landed, and returns; default 300 s. Live drawings are recorded in `<UserCacheDir>/windowctl/draw/<pid>.json`; `windowctl draw stop` (alias `clear`) ends them all; a new drawing places its sign clear of live ones. `--duration 0` = until stopped.

## Requirement: Verify before showing

`--preview out.png` renders the exact pixels without showing anything; `--json` prints `{Monitor, Sign, Tip, From, Surface, PixelScale}` in global coordinates, so an agent can check placement (or overlay the PNG on a `screenshot`) before the human sees it.

## Tasks

| Task | Verifies |
|---|---|
| `task test` | `draw_test.go`: colour parsing + preset contrast, enum validation, target rules, exact tip, sign inside monitor and clear of target, `--monitor` relativity, window box, forced side + avoidance, every shape × style × border renders, config precedence, duration/stop |
| `task build` | macOS (cgo) adapter; `GOOS=windows go vet ./...` the Windows adapter |

Gap: no CI smoke for the on-screen overlay yet; verified manually on macOS 26 (3 monitors) and Windows 11 arm64 (live screenshot shows both drawings; `draw stop` removes them).
