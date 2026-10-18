# 设计：Linux Wayland 后端

> 配套文档：[kwin-mechanism.md](./kwin-mechanism.md)（实测细节）、
> [gnome-mechanism.md](./gnome-mechanism.md)（P2 前置 spike 实测）、
> 规格切片 [17-wayland-adapter.md](../specs/17-wayland-adapter.md)。

## 1. 背景与可行性结论

windowctl 的 Linux 支持此前基于 `wmctrl` / `xdotool`（X11 工具），
在 Wayland 会话下完全不可用。评估结论（2026-10）：

**可行，但“Wayland 后端”不是单一后端，而是按 compositor 分派的
一组后端。** Wayland 核心协议禁止客户端管理他人窗口，任何跨
compositor 的统一窗口管理协议都不存在——`ext-foreign-toplevel-list-v1`
只能列窗口 + activate，不能 move/resize，且仅 wlroots 系实现。

本设计覆盖 **KDE Plasma (KWin)**（P1 已实现）、**GNOME (Mutter)**
（P2 已实现）与 **wlroots 系 sway/Hyprland**（P3 已实现，维护者的
VM 测试环境）。远期：labwc/river 等，见 §8 路线图。

## 2. 架构：运行时调度

```
internal/adapter/linux/
├── adapter.go          # 包入口：New() 检测会话类型并分派
├── detect.go           # isWaylandSession() / 纯函数，可单测
├── detect_test.go
├── x11/                # 现有 wmctrl/xdotool 实现原样搬入（零行为变化）
│   ├── adapter.go
│   ├── automation.go
│   └── overlay.go
└── wayland/
    ├── wayland.go      # core.Adapter 实现：按 compositor 分派 backend
    ├── detect.go       # detectCompositor()：SWAYSOCK / HYPRLAND_INSTANCE_SIGNATURE / XDG_CURRENT_DESKTOP
    ├── detect_test.go
    ├── input.go        # 共享：ydotool 鼠标/键盘（全 compositor 通用）
    ├── keys.go         # 共享：windowctl key 名 → evdev keycode 映射表
    ├── keys_test.go
    ├── capture/        # 共享：CaptureRect 的 PNG 裁剪/降采样/写出（KDE/GNOME/wlroots 共用）
    │   ├── capture.go
    │   └── grim.go     # 共享：grim -g 区域抓取管线（sway/Hyprland 复用）
    ├── kde/
    │   ├── kde.go      # godbus 连接 + 脚本注入器 + ListWindows/Move/Focus/ListMonitors/CursorPosition
    │   ├── screenshot.go  # spectacle 全屏抓取 + 裁剪 + 点归一化降采样
    │   └── script_test.go # JS 模板渲染（注入转义）单测
    ├── gnome/
    │   ├── gnome.go    # godbus 客户端：直连常驻扩展（无回调机制）
    │   ├── install.go  # go:embed 扩展源码 + 自动安装 + gsettings 预置
    │   ├── operations.go  # ListWindows/Move/Focus/ListMonitors/CursorPosition 报文解析
    │   ├── screenshot.go  # 扩展内区域抓取 → 点归一化
    │   ├── gnome_test.go  # 报文解析单测
    │   └── extension/  # GNOME Shell 扩展源码（ESM，随二进制分发）
    │       ├── metadata.json
    │       └── extension.js
    ├── sway/           # P3：swaymsg IPC（i3 协议），无 D-Bus、无常驻 helper
    │   ├── sway.go     # Backend + swaymsg exec 通道
    │   ├── operations.go  # get_tree 枚举 / floating 补偿 Move / focus / monitors
    │   ├── screenshot.go  # grim（capture.GrimCapture）
    │   └── sway_test.go   # tree/outputs JSON fixture 解析单测
    └── hyprland/       # P3：hyprctl IPC；错误 verdict 在 stdout 文本
        ├── hyprland.go    # Backend + hyprctl exec 通道
        ├── operations.go  # clients/monitors 枚举 / exact 几何 / movecursor
        ├── screenshot.go  # grim（capture.GrimCapture）
        └── hyprland_test.go  # clients/monitors/cursorpos fixture 单测

（smoke 见 scripts/smoke-wayland*.sh：KDE/GNOME/sway/Hyprland 各一，
均不进 CI）
```

检测优先级：

1. `XDG_SESSION_TYPE=wayland` → Wayland
2. 未设 `XDG_SESSION_TYPE` 且 `WAYLAND_DISPLAY` 非空 → Wayland
3. 其余（含 `XDG_SESSION_TYPE=x11`）→ X11 路径，**行为零变化**

> 被测机器 `DISPLAY=:1`（XWayland）与 `WAYLAND_DISPLAY` 并存，
> 故 `XDG_SESSION_TYPE` 必须优先。

Compositor 识别：`SWAYSOCK` → sway；`HYPRLAND_INSTANCE_SIGNATURE`
→ hyprland；`XDG_CURRENT_DESKTOP` 含 KDE/GNOME/sway/Hyprland →
对应后端（实测 sway 会话的 XDG_CURRENT_DESKTOP 就是 `sway`，
Hyprland 是 `Hyprland`——为 socket 变量缺失的场景兜底）；其余 →
明确报错 "no windowctl backend for Wayland compositor %q"。

## 3. KDE 后端（KWin）机制

**窗口管理唯一途径**：把 JS 脚本经 `org.kde.KWin /Scripting` 注入
KWin，脚本在 compositor 进程内执行（`workspace.stackingOrder`、
`frameGeometry` 赋值、`workspace.activeWindow` 赋值），结果经脚本内
`callDBus` 回调到**本进程拥有的 D-Bus 服务**。

- 依赖 `github.com/godbus/dbus/v5`（纯 Go、无 CGO、无传递依赖）
- bus 名每进程唯一：`org.muthuishere.windowctl.kde.<pid>`
- 回调带调用 tag，丢弃迟到回调；5s 超时报明确错误
- `loadScript` 同名返回 -1 → unload 重试

各操作的 JS 写法（实测，细节见 kwin-mechanism.md）：

| 操作 | JS |
|------|-----|
| 列窗口 | `stackingOrder` + `normalWindow` 过滤；`String(internalId)` 作 ID；`frameGeometry` 作 bounds |
| 移动 | `w.frameGeometry = { x, y, width, height }`（**必须整体字面量**） |
| 聚焦 | `workspace.activeWindow = w` |
| 显示器 | `workspace.screens`（`geometry`+`devicePixelRatio`）；Primary=`activeScreen`；Active=`cursorPos` 所在屏；Focused=`activeWindow` 质心所在屏 |
| 光标位置 | `workspace.cursorPos` |

**几何异步性**：Wayland 几何变更要等客户端响应 configure 事件，
`Move` 后立即读回可能是旧值。adapter 不做校验（与其他平台一致），
smoke 测试等待落定后断言。

**权限语义**：KDE 无 macOS 式权限门。`RequestAccessibility` /
`CheckAccessibility` / `CheckScreenCapture` / `RequestScreenCapture`
全部 no-op（true/nil），与现有 Linux 语义一致。

## 4. 能力矩阵

| Adapter 方法 | KDE (KWin) | GNOME (Mutter) | sway | Hyprland | 备注 |
|---|---|---|---|---|---|
| ListWindows | ✅ 脚本 | ✅ 扩展（type=NORMAL 过滤） | ✅ get_tree（app_id/WM_CLASS 过滤） | ✅ hyprctl clients（mapped&&!hidden 过滤） | normalWindow/Meta.WindowType.NORMAL 过滤不含对话框 |
| ListMonitors | ✅ 脚本（含 dpr） | ✅ 扩展（含 scale） | ✅ get_outputs（真实 primary 字段） | ✅ hyprctl monitors（mode 像素 ÷scale；无 primary → focused 兜底） | |
| Move | ✅ 脚本 | ✅ 扩展（同步生效） | ✅ floating enable + resize/move absolute（含标题栏补偿） | ✅ togglefloating + exact resize/move（零补偿，像素级精确） | 平铺窗口先转浮动（sway/Hyprland 已决策） |
| Focus | ✅ 脚本 | ✅ 扩展 | ✅ [con_id] focus | ✅ focuswindow address | |
| CursorPosition | ✅ cursorPos | ✅ get_pointer | ❌ sway 无途径（明确报错） | ✅ hyprctl cursorpos | |
| CaptureRect | ✅ spectacle + 裁剪 | ✅ 扩展内区域抓取 | ✅ grim（共享管线） | ✅ grim（scale=2 HiDPI 路径真机验证） | 见 §5 点归一化 |
| MouseMove/Click | ✅ ydotool | ✅ ydotool | ✅ seat cursor set 定位 + ydotool 按钮 | ✅ hyprctl movecursor 定位 + ydotool 按钮 | 需 udev 配置 |
| TypeText | ✅ ydotool type | ✅ ydotool type | ✅ ydotool type | ✅ ydotool type | 布局相关 |
| PressChord | ✅ ydotool key（evdev keycode） | 同左 | 同左 | 同左 | 映射表 keys.go |
| Launch | ✅ exec（不变） | ✅ exec（不变） | 同左 | 同左 | |
| ShowOverlay | ❌ ErrNotImplemented | ❌ ErrNotImplemented | wlr-layer-shell（远期） | 同左 | KDE/GNOME 均不支持 layer-shell |
| AccessibleTexts | ❌ ErrNotImplemented | ❌ ErrNotImplemented | AT-SPI（远期） | 同左 | |
| RecognizeText | ❌ ErrNotImplemented | ❌ ErrNotImplemented | tesseract（远期） | 同左 | |

**ydotool 绝对定位陷阱（sway/Hyprland spike 定论）**：ydotool 的
`--absolute` 是 "INT32_MIN 相对 slam + 偏移" 的 hack，经 libinput
指针加速后落点失真且不可确定（Hyprland 默认配置精确 ×2、与
monitor scale 无关；sway 默认配置非单调——同一请求分别落进目标
窗口/空隙/越过远缘）。因此有 compositor 光标通道的后端一律走它
（`pointerMover` 接口）：Hyprland 用 `dispatch movecursor`，sway
用 `seat seat0 cursor set`（两者实测像素级 1:1）。KDE/GNOME 无
等价通道、继续 ydotool（被测机 dpr=1，HiDPI 落点未验证，见
sway-mechanism.md §11、hyprland-mechanism.md §7）。键盘不受影响
（EV_KEY 不经过加速）。

## 5. 点归一化（自动化闭环不变量）

Core 契约：**1 图像像素 == 1 坐标点**（这样 agent 从截图读出的
像素坐标可以直接喂给 `MouseClick`）。

- KDE 截图链：`spectacle -b -n -f -o tmp.png` 抓**虚拟桌面全屏**
  （设备像素）→ Go 侧按 `bounds × devicePixelRatio` 裁剪 →
  若 dpr≠1 则降采样到 `bounds.W × bounds.H` → 写出 PNG。
- dpr=1（含被测机器）时退化为纯裁剪。
- 裁剪用 `image/draw`，降采样用 `golang.org/x/image/draw`
  （已在依赖树中，纯 Go）。
- wlroots（sway/Hyprland）链：`grim -g "x,y WxH"` **直接区域抓取**
  （无全屏 + 裁剪步骤），几何是逻辑坐标，输出 = 逻辑 × scale 设备
  像素 → scale≠1 时降采样回点尺寸（`capture.GrimCapture` 共享；
  Hyprland scale=2 真机验证）。

## 6. 外部工具依赖

| 工具 | 用途 | 缺失时行为 |
|------|------|-----------|
| spectacle | KDE 截图 | `exec.LookPath` 失败 → 明确错误 + 安装指引 |
| swaymsg | sway 窗口管理/枚举（随 compositor 安装） | 同上 |
| hyprctl | Hyprland 窗口管理/枚举/光标（随 compositor 安装） | 同上 |
| grim | sway/Hyprland 截图 | 同上 |
| ydotool（+ ydotoold） | 鼠标/键盘注入 | 同上，错误含 udev 规则与 input 组配置步骤 |
| godbus（Go 库） | KWin D-Bus 回调 | 编译期依赖，无需安装 |

策略（已决策）：**检测缺失即明确报错 + 安装指引**，不做静默降级。

## 7. 测试策略

- **进 CI（ubuntu/mac/windows 全绿）**：`detect_test.go`（会话检测
  纯函数，表驱动 env 组合）、`kde/script_test.go`（JS 模板注入转义）、
  `keys_test.go`（evdev keycode 映射表完整性）、`gnome/gnome_test.go`
  （扩展报文解析/字符串解析）。
- **不进 CI**（runner 无 compositor，spec 10 禁 Docker）：
  `task smoke-wayland`（本机 KDE 会话）、`task smoke-wayland-gnome`
  （本机 GNOME 会话）、`task smoke-wayland-sway` / 
  `task smoke-wayland-hyprland`（本机对应会话，或
  `WCTL_SMOKE_SSH=user@host` 推送到 VM 目标执行）——launch →
  poll `windows list` → `move`（目标坐标按显示器尺寸自适应，避免
  推到屏外）→ 等落定 → 断言 bounds（容差 `WCTL_WAYLAND_TOLERANCE`，
  默认 50px）→ `focus` → `monitors` → `screenshot`；Hyprland smoke
  额外做 `mouse move`/`mouse position` 的 movecursor/cursorpos
  精确往返断言。

## 8. 路线图

- **P1（已完成）**：结构重构 + KDE 后端全能力（窗口管理 + 截图 +
  输入）+ 单测 + smoke + 文档。
- **P2（已完成，2026-10）**：GNOME (Mutter) 后端 —— 常驻 GNOME Shell
  扩展（`wayland/gnome/extension/`，随二进制 go:embed 分发，首次调用
  自动安装到 `~/.local/share/gnome-shell/extensions/` + gsettings
  预置 enable + 提示注销一次），扩展以 D-Bus 服务
  （`org.muthuishere.windowctl.gnome`）暴露 ListWindows
  （`get_window_actors()`）、Move（`move_resize_frame`）、Focus、
  ListMonitors、CursorPosition、Screenshot（进程内 `Shell.Screenshot`
  + GOutputStream，设备像素坐标）；dpr 处理沿用 §5 点归一化；
  输入 ydotool 直接复用。机制细节与生命周期实测见
  [gnome-mechanism.md](./gnome-mechanism.md)。
- **P3（已完成，2026-10）**：wlroots 后端 —— sway（shell-out
  `swaymsg`：get_tree 枚举（node `id` 作稳定 ID）/ 平铺先
  `floating enable` / `resize set` + `move absolute position`
  （口径含 SSD 标题栏，用 deco_rect.height 补偿）/ get_outputs
  （真实 primary）/ grim 截图；**CursorPosition 不可用**：sway IPC
  无光标查询）与 Hyprland（shell-out `hyprctl -j`：clients
  （address 作稳定 ID）/ `togglefloating` + `resizewindowpixel
  exact` + `movewindowpixel exact`（exact 口径零补偿、像素级
  精确；resize 以中心锚故 move 放最后）/ monitors（mode 像素
  ÷scale）/ cursorpos）；截图 grim 共享管线（`wayland/capture`
  新增 `GrimCapture`），**scale=2 真机验证点归一化**；输入 ydotool
  复用，但 MouseMove 改走 `pointerMover` 通道 —— Hyprland 用
  `dispatch movecursor`（ydotool --absolute 经 libinput 加速失真
  的定论见 hyprland-mechanism.md §7）。机制细节见
  [sway-mechanism.md](./sway-mechanism.md) 与
  [hyprland-mechanism.md](./hyprland-mechanism.md)。
- **远期**：wlr-layer-shell overlay（go-wayland 原生绑定）、AT-SPI
  AccessibleTexts（GNOME 生态最成熟，P2 落地后优先评估）、tesseract OCR、
  KDE/GNOME 的 HiDPI ydotool 落点验证（无 compositor 光标通道，
  如失真需 no-accel 依赖标注）。

## 9. 风险

| 风险 | 缓解 |
|------|------|
| KWin 脚本 API 随版本漂移 | 只用 KWin 6.0 文档 API；实测锁定 6.7.4 |
| 回调超时/迟到 | tag 过滤 + 5s 超时 + 明确错误 |
| ydotool 安装门槛（udev/root） | 明确报错含完整步骤；文档单列小节 |
| KDE HiDPI 截图慢（KDE bug 433776） | 文档标注；不影响正确性 |
| 多显示器 spectacle 行为未验证 | 文档标注假设 |
| godbus 引入传递依赖影响发布体积 | godbus 零传递依赖，二进制 +100-200KB |
| GNOME 扩展安装后需注销才生效（Wayland 无法重启 Shell） | **已实测确认**（spike，见 gnome-mechanism.md §2）：无绕过路径；安装流程文档如实标注一次性注销 |
| GNOME Shell 扩展 API 跨版本漂移快于 KWin | 扩展 metadata 锁定 shell-version 起步；逐版本适配 |
| ydotool --absolute 经 libinput 加速失真且不可确定 | 有 compositor 光标通道的后端用之：Hyprland `movecursor`、sway `seat cursor set`（均实测 1:1）；KDE/GNOME 无通道，HiDPI 落点验证待做（hyprland-mechanism.md §7） |
| sway/Hyprland 平铺窗口无法绝对定位 → Move 自动转浮动 | **已决策**（改变用户布局，机制文档如实标注） |
| Hyprland monitors width/height 是 mode 物理像素（physical 字段为 0） | ListMonitors ÷scale 折算逻辑，实测锁定 0.55.2 |
| wlroots compositor 版本漂移（exact 语义/decoration） | spike 锁定 sway 1.10.1 / Hyprland 0.55.2；机制文档逐项记录 |
