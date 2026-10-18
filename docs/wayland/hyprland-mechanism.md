# Hyprland hyprctl 机制：实测细节

> 2026-10 在 Debian 13 (trixie) / Hyprland **0.55.2** / Wayland
> （monitor scale **2.0** —— HiDPI 路径由此首次真机验证）上逐项验证。
> 本文件记录 [design.md](./design.md) §8 P3（wlroots 后端）Hyprland
> 侧的 spike 结论与全部踩坑细节。sway 侧对应见
> [sway-mechanism.md](./sway-mechanism.md)。

## 1. 机制总览：hyprctl IPC

与 sway（i3 协议 swaymsg）同类：Hyprland 自带 IPC（unix socket，
`$XDG_RUNTIME_DIR/hypr/$HYPRLAND_INSTANCE_SIGNATURE/.socket.sock`），
windowctl **shell-out `hyprctl`** 并解析其 JSON，无 godbus、无常驻
helper。

```
windowctl (exec)                          Hyprland
  ├─ hyprctl -j clients ─────────────────► 窗口列表 JSON
  ├─ hyprctl -j monitors / cursorpos ────► 输出 / 光标
  ├─ hyprctl dispatch <name> <args> ─────► 写：togglefloating/
  │                                          resizewindowpixel/
  │                                          movewindowpixel/
  │                                          focuswindow/movecursor
  └─ grim -g "x,y WxH" out.png ──────────► 截图（wlr-screencopy）
```

- 连接前提：`HYPRLAND_INSTANCE_SIGNATURE`（+ `XDG_RUNTIME_DIR`）。
  未设时 hyprctl 报 "HYPRLAND_INSTANCE_SIGNATURE not set!" 且 exit 2。
  实例 socket 目录 `/run/user/<uid>/hypr/<sig>/` —— **崩溃残留会留下
  旧目录**（实测有两个），"取最新目录" 才是正确的发现方式（smoke
  脚本 `ls -t | head -1`）。
- `hyprctl instances` 无需 HIS 即可列出全部实例（含 pid/wl socket），
  可作交叉校验。

## 2. 错误契约（重要差异，已验证）

**hyprctl dispatch 失败时 exit code = 0，错误写在 stdout** —— 与
swaymsg（exit 2 + JSON error 体）完全不同。实测：

| 场景 | stdout | exit |
|------|--------|------|
| 成功 | `ok` | 0 |
| criteria 无匹配（窗口已关/ID 失效） | `No such window found` | 0 |
| dispatcher 名不存在 | `Invalid dispatcher` | 0 |
| dispatcher 参数非法 | `moveWindow: no window` | 0 |

adapter 据此把输出文本当作 verdict：`ok` → 成功；含
"No such window found" → `core.ErrNoMatch`；其余 → 原样带上下文报错。

## 3. 坐标口径（已验证，scale=2 真机）

被测 monitor：mode 2336×1261，scale 2.0 → **逻辑桌面 1168×630**。

| 数据 | 坐标系 |
|------|--------|
| `hyprctl -j clients` 的 `at`/`size` | **逻辑坐标**（实测平铺 foot：at=[21,21]、size=[1126,589]，与 1168 宽的布局吻合） |
| `hyprctl -j monitors` 的 `width`/`height` | **输出 mode 的物理像素**（2336×1261）→ ListMonitors 必须**除以 scale**；`physicalWidth/Height` 字段在此虚拟显示上为 0，不可依赖 |
| `hyprctl cursorpos` / `movecursor` | 逻辑坐标 |
| `grim -g` 几何 | 逻辑坐标 |

## 4. 窗口枚举（已验证）

```sh
hyprctl -j clients
```

- 列表顺序 = **焦点历史序，最近聚焦在前**（`focusHistoryID` 0 是
  当前焦点）——恰好满足公共层 stampFocused 要求的
  "frontmost first" 顺序（见 windowctl.go stampFocused 注释）。
- 过滤规则：`mapped && !hidden && class != ""`。layer-shell 表面
  （waybar 等）**不出现在 clients 里**，无需专门过滤 panel。
- 每项可读：`address`（稳定 ID）、`class`、`title`、`pid`、
  `at`、`size`、`floating`、`fullscreen`（int）、`workspace`
  （对象：id/name）、`monitor`（输出下标）。
- XWayland 窗口同构（`xwayland: true`，class=WM_CLASS）。

## 5. 稳定 ID（已验证）

- 窗口稳定 ID = `address` 字符串（如 `0x5631bd3f6d30`）。
- 跨 `togglefloating` 实测不变；同会话内稳定。
- criteria 写法：`address:0x5631bd3f6d30`。

## 6. 几何命令（已验证）

| 命令 | 语义 |
|------|------|
| `dispatch movewindowpixel "exact X Y,address:0x…"` | floating 窗口左上角 → **(X,Y) 精确落位**（默认 decoration 无标题栏，无补偿） |
| `dispatch resizewindowpixel "exact W H,address:0x…"` | size → **(W,H) 精确**；但**以窗口中心为锚**，会移动位置 |
| `dispatch togglefloating address:0x…` | 平铺⇄浮动切换；floating 的 size 会变为 Hyprland 的浮动默认尺寸（与平铺尺寸不同），后续 resize 会覆盖 |
| `dispatch focuswindow address:0x…` | 激活窗口（activewindow 立即可读） |
| `dispatch movecursor X Y` | 光标绝对定位，**精确**（见 §7） |

实测的落位序列（Move 实现）：

1. 平铺（`floating=false`）→ `togglefloating`（同 sway：平铺窗口的
   exact move 不做绝对定位，实测只会重排布局）。
2. `resizewindowpixel exact W H` —— 尺寸精确，位置被重排（以中心锚）。
3. `movewindowpixel exact X Y` —— **放最后**，位置精确。

> 与 sway 的差异：sway 命令口径是"含 SSD 标题栏的容器"，需要
> deco_rect.height 补偿；Hyprland 默认无边框标题，exact 口径即
> clients 报告的 at/size 本身，**零补偿、像素级精确**
> （scale=2 下实测 334,50 500x530 → at/size 逐值相等）。
> 若用户配置启用了标题栏类 decoration，语义会回到"含边框"口径，
> 容差吸收。

## 7. 输入：ydotool 的绝对定位陷阱（本次 spike 最重要的发现）

### 现象

```sh
ydotool mousemove --absolute -x 100 -y 100
hyprctl cursorpos   # → (200, 200)  —— 精确 2×，且与 monitor scale 无关
```

- ×2 **精确、可重复、与 scale 无关**（临时把 monitor scale 改 1 后
  仍是 ×2 —— 排除 scale 假设）。
- 普通 relative move 则**不可确定**：+10,+20 → +18,+36（≈1.8×），
  连续运动后 +100,+100 → +98,+98（≈1×）—— libinput adaptive
  加速的速度依赖曲线。
- 唯独"slam 之后的第一个 offset"稳定 ×2.0。

### 机制（源码考证）

- `ydotoold` 是**纯转发器**（Daemon/ydotoold.c：socket 收
  `input_event` 原样写 uinput）——绝对定位的换算在**客户端**。
- `Client/tool_mousemove.c` 的 `--absolute` 是个 hack：先发
  `REL_X/REL_Y = INT32_MIN` 的巨量负 delta 把光标钳到原点，再发
  `+pos` 的相对 delta。其 --help 原话：
  *"You need to disable mouse speed acceleration for correct
  absolute movement."*
- 巨量 delta 经 libinput 加速后落点失真（Hyprland 上表现为
  精确 ×2 的 clamp 残差）；把光标坐标除以任何 compositor 的 scale
  都修不了它 —— **根因是加速曲线，不是坐标系**。
- 佐证：`hyprctl keyword input:force_no_accel 1`（或
  `accel_profile flat`）后，ydotool --absolute 立即恢复 **1:1
  精确**（实测 300,200 → 300,200）。

### 结论（adapter 采用）

- **能用 compositor 自身通道定位的，绝不用 ydotool --absolute**：
  Hyprland 有 `dispatch movecursor X Y`（逻辑坐标、精确、与用户
  accel 配置无关）；sway 会话复验时发现 sway 也有等价通道
  `seat seat0 cursor set X Y`（同样精确 1:1，实测）。两者都以
  `pointerMover` 接口接入共享 input.go，MouseMove/MouseClick 的
  定位全部走 compositor 通道，ydotool 只承担按钮与键盘。
- sway 默认配置下 ydotool --absolute 的落点已实测为**非单调、
  不可确定**（同一请求分别落进目标窗口/空隙/越过远缘）——比
  Hyprland 的稳定 ×2 更糟，坐实了上述决策（见
  sway-mechanism.md §11）。
- 键盘（type/key）不受影响：EV_KEY 事件不经过 pointer accel。
- ydotoold 的 xinput 加速关闭只在 `DISPLAY` 存在时执行
  （ydotoold.c main）；从 SSH 启动的 ydotoold 不会关 —— 别把
  "ydotoold 自己会处理" 当假设。

## 8. 显示器（已验证）

- `hyprctl -j monitors`：`name`、`x`/`y`（逻辑布局坐标）、
  `width`/`height`（**mode 物理像素**，需 ÷scale）、`scale`、
  `focused`（输入焦点所在输出 —— 真实字段）、`disabled`、
  `availableModes`。
- **没有 `primary` 字段**（0.55.2）→ ListMonitors 的 Primary 用
  focused 输出兜底（与 sway 无 primary 时相同策略）。
- `hyprctl cursorpos`："x, y" 文本（无 -j JSON），解析
  `strconv`。

## 9. 截图：grim（scale=2 真机验证）

```sh
grim -g "334,50 500x530" /tmp/out.png   # 逻辑坐标
```

- 输出 PNG = **逻辑 × scale 设备像素**：请求 500x530 → 1000×1060
  （scale 2）；100x100 → 200x200。Go 侧降采样回点尺寸后
  "1 图像像素 == 1 坐标点" 成立 —— **HiDPI 归一化路径首次真机
  验证**（此前 KDE/GNOME 的被测机都是 scale 1）。
- smoke（scale 2）已覆盖：断言输出 PNG 恰为请求的逻辑尺寸。

## 10. 测试应用备注

- `foot --app-id=wctl-*`：class 跟随 app-id（hyprctl clients 的
  `class` 直接可见），识别稳定；标题仍会被 shell OSC 覆盖。
- ydotoold 用户级启动即可（input 组）；跨会话（sway→Hyprland 切换）
  daemon 无需重启（uinput 与 compositor 无关，实测存活）。

## 11. 对 design.md 的实现输入（P3 Hyprland 定稿要点）

| 项 | 结论 |
|----|------|
| 通道 | shell-out `hyprctl`（Hyprland IPC），无 godbus 依赖 |
| 依赖 | `hyprctl`、`grim`；ydotool 共享（键盘 + 按钮） |
| 窗口 ID | clients 的 `address` 字符串；criteria `address:0x…` |
| 错误语义 | **stdout 文本 verdict + exit 0**（与 swaymsg 的 exit 2 相反） |
| 平铺窗口 Move | 先 `togglefloating`（已决策，改变布局如实标注） |
| Move 落位 | resizewindowpixel exact(W,H) → movewindowpixel exact(X,Y)；resize 以中心锚会移动位置，move 必须放最后 |
| ListMonitors | width/height ÷ scale = 逻辑；无 primary → focused 兜底；Active=cursorpos 真实信号 |
| 光标 | `hyprctl cursorpos` + `dispatch movecursor`（精确）；**MouseMove 禁用 ydotool --absolute**（libinput 加速失真，源码+实测双重定论） |
| 截图 | grim 逻辑坐标；scale=2 实测 逻辑×2 设备像素 → 降采样归一化（HiDPI 路径真机验证） |
| detect | `HYPRLAND_INSTANCE_SIGNATURE`（已有）+ `XDG_CURRENT_DESKTOP=Hyprland` 兜底（detect.go 已补） |
| 实例发现 | `/run/user/<uid>/hypr/` 最新目录（崩溃残留旧目录，`ls -t`） |
