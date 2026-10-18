# Wayland 后端

windowctl 的 Linux Wayland 支持文档索引。

> [此文档目录](/docs/wayland) 随本 fork 一起提交（上游分支曾有意不纳管）。

## 文档

- [design.md](./design.md) — 架构、机制、能力矩阵、已验证事实、限制与路线图（主文档）
- [kwin-mechanism.md](./kwin-mechanism.md) — KWin D-Bus 脚本注入机制的实测细节与踩坑记录
- [gnome-mechanism.md](./gnome-mechanism.md) — GNOME Shell 扩展机制的实测细节与踩坑记录（P2 前置 spike）
- [sway-mechanism.md](./sway-mechanism.md) — sway IPC (swaymsg) 机制的实测细节与踩坑记录（P3 spike）
- [hyprland-mechanism.md](./hyprland-mechanism.md) — Hyprland hyprctl 机制的实测细节与踩坑记录（P3 spike，含 ydotool 绝对定位陷阱）

## 概述

Linux 后端在 `internal/adapter/linux/` 内做**运行时调度**：检测
`XDG_SESSION_TYPE` / `WAYLAND_DISPLAY` 判定会话类型，Wayland 会话再按
compositor 分派到具体后端（当前实现 KDE Plasma/KWin、GNOME/Mutter、
sway 与 Hyprland；其余 compositor 明确报错）。

| 会话 | 调度目标 | 外部工具依赖 |
|------|----------|--------------|
| X11（或未检测到 Wayland） | `x11/` 子包（原 wmctrl/xdotool 实现，行为不变） | wmctrl、xdotool、xrandr、import/scrot |
| Wayland + KDE | `wayland/` → `kde/` 后端 | spectacle、ydotool（可选，仅输入用） |
| Wayland + GNOME | `wayland/` → `gnome/` 后端（Shell 扩展自动安装，首次需注销一次） | ydotool（可选，仅输入用） |
| Wayland + sway | `wayland/` → `sway/` 后端（swaymsg IPC） | swaymsg（随 compositor）、grim、ydotool（可选，仅输入/按钮用） |
| Wayland + Hyprland | `wayland/` → `hyprland/` 后端（hyprctl IPC） | hyprctl（随 compositor）、grim、ydotool（键盘/按钮） |
| Wayland + 其他 compositor | 明确报错（当前不支持） | — |

**依赖变更**：新增纯 Go 依赖 `github.com/godbus/dbus/v5`（无 CGO、
无传递依赖）。KWin 窗口管理必须经 D-Bus 回调回传结果，shell-out
拿不到，这是 requirements §8.4“优先标准库”偏好的一次明确取舍
（先例：`x/sys`、`x/image`）。

## 快速事实（2026-10 在 KWin 6.7.4 / Plasma 6.7.4 实测）

- Wayland 核心协议**无法**移动/缩放他人窗口；KDE 侧唯一途径是
  KWin D-Bus 脚本注入 + `callDBus` 回调。
- 窗口稳定 ID = `String(window.internalId)`，形如 `{uuid}` 带花括号。
- 移动窗口的正确写法是**整体赋一个字面量对象**：
  `w.frameGeometry = { x, y, width, height }`（读回的快照对象改完再
  赋回无效；单字段连续写互相覆盖，只有最后一次生效）。
- 截图：KWin 6 已移除 `org.kde.KWin.Screenshot` 接口，改用
  `spectacle -b -n -f -o file.png` 全屏抓取 + Go 侧裁剪/降采样。
- 鼠标位置 `workspace.cursorPos` 在 KDE 可用（Sway 无此途径）。

详见 [design.md](./design.md)。

## GNOME 快速事实（2026-10 在 Debian 13 / GNOME Shell 48.7 实测）

- 常驻 Shell 扩展经 D-Bus 服务直连调用（无 KWin 式回调机制）；扩展
  随二进制 go:embed 分发，首次调用自动安装——**新装扩展需注销登录
  一次**才被 Shell 感知（无绕过路径，源码已证），此后每会话自动激活。
- 扩展声明 `session-modes: ["user", "unlock-dialog"]`：锁屏期间服务
  保持存活（否则锁屏会静默 disable 扩展，bus 名消失）。
- 窗口稳定 ID = `String(get_stable_sequence())`（数字）。
- 移动：`move_resize_frame(true, x, y, w, h)`（5 参，首参 user_op）；
  **同步生效**（与 KWin 的异步落定不同），但刚 map 完成的窗口会
  忽略几何修改，需等待落定。
- 截图：D-Bus 截图接口有白名单（`AccessDenied`），必须走扩展进程内
  `Shell.Screenshot`（GOutputStream、**设备像素坐标**）；Shell 已将
  其 promisify，只能 promise 式调用（callback 式 → unhandled
  rejection → Shell 直接禁用扩展）。

详见 [gnome-mechanism.md](./gnome-mechanism.md)。

## wlroots 快速事实（2026-10 在 sway 1.10.1 / Hyprland 0.55.2 实测）

- sway / Hyprland 都有**原生 IPC**，shell-out 即可（swaymsg /
  hyprctl），无需 KWin 式回调或 GNOME 式常驻扩展。
- 窗口稳定 ID：sway = tree 节点 `id`（criteria `[con_id=N]`）；
  Hyprland = `address` 字符串（criteria `address:0x…`）。
- **平铺窗口都不能绝对定位**（sway 报
  "Only floating containers can be moved..."；Hyprland 的 exact
  move 在平铺窗口上只重排布局）——Move 先转浮动（已决策）。
- 坐标口径易踩坑：sway 的 move/resize 命令**含 SSD 标题栏**而
  get_tree 的 `rect` 不含（deco_rect.height 补偿）；Hyprland 的
  exact 口径即 clients 报告值（零补偿），但 `resize` 以中心锚会
  移动位置（move 必须放最后）；hyprctl monitors 的 width/height
  是 mode **物理像素**（需 ÷scale）。
- 错误契约相反：swaymsg 失败 exit 2 + JSON error 体；hyprctl
  dispatch 失败 **exit 0**、错误在 stdout 文本（`ok` 才是成功）。
- 截图：`grim -g`（共享管线 `capture.GrimCapture`）；逻辑 ×
  scale 设备像素 → 降采样回点尺寸。**scale=2 真机验证**了
  HiDPI 归一化路径。
- **ydotool --absolute 经 libinput 指针加速失真**（默认配置下
  Hyprland 精确 ×2、与 scale 无关；sway 非单调不可确定）——两个
  wlroots 后端的 MouseMove 都走 compositor 自身的精确光标通道：
  Hyprland `dispatch movecursor`、sway `seat seat0 cursor set`
  （实测均像素级 1:1）。
- 光标查询：Hyprland 有（cursorpos）；**sway 没有**（明确
  not-implemented）。
- smoke 应用：`foot --app-id=wctl-*`（gnome-calculator 在 sway 上
  即崩；标题会被 shell OSC 覆盖，识别靠 app-id）。

详见 [sway-mechanism.md](./sway-mechanism.md) 与
[hyprland-mechanism.md](./hyprland-mechanism.md)。
