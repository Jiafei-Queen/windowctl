# sway IPC 机制：实测细节

> 2026-10 在 Debian 13 (trixie) / sway **1.10.1** / Wayland 上逐项验证。
> 本文件记录 [design.md](./design.md) §8 P3（wlroots 后端）sway 侧的
> spike 结论与全部踩坑细节，实现时的"为什么这么写"都源于这些实验。

## 1. 机制总览：swaymsg IPC（i3 协议）

与 KWin（脚本注入 + D-Bus 回调）、GNOME（常驻扩展 D-Bus 服务）不同，
sway 提供**原生 IPC**：`swaymsg` 直接连 sway 的 unix socket，支持
查询（`-t get_tree` 等）与命令（`[criteria] command`）。shell-out 到
`swaymsg` 即可拿到结果 JSON，无需 godbus、无需常驻进程。

```
windowctl (exec)                    sway 进程
  ├─ swaymsg -t get_tree ──────────► 读：窗口树 JSON
  ├─ swaymsg [con_id=N] cmd ──────► 写：floating/resize/move/focus
  └─ grim -g "x,y WxH" out.png ────► 截图（wlr-screencopy）
```

- `swaymsg` 连接的 socket 取自 `SWAYSOCK` 环境变量。
- **命令失败时**（如对平铺窗口 move），swaymsg 打印结构化 JSON 错误
  （`{"success": false, "error": "..."}`）且 **exit code = 2**（成功
  为 0）——Go 侧以 exit code 判定，错误体留作上下文。

## 2. 环境与检测（对 detect.go 的输入）

实测被测机器（sway 自身进程 environ 与子进程 environ 对比）：

- sway 子进程（swaybar、经 sway exec 启动的应用）持有完整会话环境：
  `XDG_SESSION_TYPE=wayland`、`WAYLAND_DISPLAY=wayland-1`、
  `XDG_CURRENT_DESKTOP=sway`、`SWAYSOCK=/run/user/<uid>/sway-ipc.<uid>.<pid>.sock`。
- sway **自身**进程 environ 里没有 `SWAYSOCK`（由启动包装设置给子进程）。
- `systemctl --user show-environment` 同样可见全部变量 —— SSH 会话
  注入环境的可靠来源。

两个对实现的直接影响：

1. **detect.go 必须补 `XDG_CURRENT_DESKTOP=sway` 兜底**：不能只认
   `SWAYSOCK`（虽然正常图形会话的子进程都有它，但 nested/exec 场景
   可能缺失；而 `XDG_CURRENT_DESKTOP=sway` 由会话保证）。
2. **SSH 会话必须显式注入 `SWAYSOCK`**：sway 1.10 的 swaymsg **不做**
   `XDG_RUNTIME_DIR` 通配自动发现（实测只设 `XDG_RUNTIME_DIR` 报
   "Unable to retrieve socket path"）。且 socket 路径**含 sway 的
   PID**（`sway-ipc.1000.6259.sock`），不是固定名 —— smoke 脚本需用
   glob `sway-ipc.*.sock` 或 `systemctl --user show-environment` 发现。

## 3. 窗口枚举（已验证）

```sh
swaymsg -t get_tree
```

树结构：`root → output → workspace → con/floating_con`。窗口判定：

- 节点 `type` 为 `con`（平铺）或 `floating_con`（浮动）且
  - `app_id` 非空（native Wayland），或
  - `window_properties.class` 非空（XWayland：`shell=="x11"`，
    `app_id` 为 null）
  
  即窗口语义等价于 KWin 的 `normalWindow` 过滤。容器拆分节点、
  `output`/`workspace` 节点天然被此规则排除。
- **swaybar 不出现在 get_tree 里**（独立进程，不是树节点）——无需
  像 KDE 那样过滤 panel。
- 每个 leaf 上可读：`name`（title）、`app_id`、`pid`、`rect`、
  `focused`（bool，全树恰好至多一个 true）、`floating`、`visible`。

## 4. 稳定 ID（已验证）

- 窗口稳定 ID = 节点 `id` 字段（JSON number，Go 侧转 string 契约）。
- 实测跨多次 get_tree、跨 `floating enable/disable` 切换保持不变
  （id=16 从平铺 `con` 变为浮动 `floating_con` 期间 id 恒定）。
- 特殊节点 id 占用 int32 边界值（`2147483645..2147483647` 是
  scratchpad/伪节点），真实窗口是小整数，Go 侧 `strconv` 即可。
- `move`/`focus` 用 criteria `[con_id=N]` 精确匹配该 id。

## 5. 平铺 vs 浮动（Move 的前置语义，已验证）

对平铺（`type=="con"`，`floating` 为 `auto_off`/`user_off`）窗口：

```sh
swaymsg "[con_id=16] move position 700 px 300 px"
# {"success": false, "parse_error": false,
#  "error": "Only floating containers can be moved to an absolute position"}
# exit code 2
```

**平铺窗口不可绝对定位**（i3/sway 布局语义）。adapter 的策略
（已决策）：Move 时对平铺窗口先 `floating enable` 再定位 —— 保证
Move 契约对所有窗口可用，代价是改变用户的平铺布局（写入文档如实
标注）。`floating` 字段值：`auto_off`/`user_off`=平铺，`auto_on`/
`user_on`=浮动。floating enable 后 `type` 变 `floating_con`、节点
id 不变。

## 6. 几何命令：两个坐标口径（核心踩坑，已验证）

sway 的 move 命令定位的是**容器左上角（含 SSD 标题栏）**，而
get_tree 的 `rect`（windowctl 的 bounds 语义）是**客户区 + 边框、
不含标题栏**。三个字段实测关系：

| 字段 | 语义 |
|------|------|
| `rect` | 客户区 + 边框（不含标题栏），**屏幕绝对坐标** |
| `deco_rect` | 标题栏矩形，工作区相对坐标；`border none` 时全 0 |
| `window_rect` | 客户区相对 rect 的内嵌偏移（边框厚度） |

实测（swaybar 高 20px，标题栏 22px，边框 2px）：

```sh
swaymsg "[con_id=16] move absolute position 300 px 300 px"
# → rect.y = 322 = 300 + 22（标题栏）
swaymsg "[con_id=16] move position 400 px 400 px"
# → rect.y = 442 = 400 + 20（bar，工作区 y 原点） + 22（标题栏）
```

结论：

| 命令 | 坐标系 |
|------|--------|
| `move absolute position X px Y px` | **屏幕坐标**，容器含标题栏左上角 |
| `move position X px Y px` | **工作区坐标**（原点 = workspace.rect.x/y，即 bar 下方） |

**adapter 采用 `move absolute position`**（少一层工作区原点换算），
落位公式：

```text
deco_h = 该窗口当前 deco_rect.height（border none 时 = 0）
move absolute position X px (Y - deco_h) px   → rect.x = X, rect.y = Y ✓
resize set width W px height (H + deco_h) px  → rect.w ≈ W, rect.h ≈ H
```

实测两组公式（带标题栏 / border none 各一组）：

- `border none` 时**像素级精确**：(200,200,500,300) → rect 精确。
- 带标题栏时位置精确，尺寸差 1–2px（客户端 min-size/步进协商，
  如 foot 请求 600×400 实得 599×398）——smoke 容差内。

> `resize set` 的高度口径同样**含标题栏**：请求 400px 实得
> rect.h=378 = 400 − 22（标题栏）。宽度含边框：请求 600 → rect.w=600
> 精确（客户区 596 = 600 − 2×2 边框）。

顺序注意：**先 resize 后 move**（resize 可能因协商微调位置，move
放最后保证落点）。

## 7. 聚焦（已验证）

```sh
swaymsg "[con_id=16] focus"
# 之后该节点 focused == true（全树唯一）
```

## 8. 显示器（已验证）

```sh
swaymsg -t get_outputs
```

- 输出对象含 `rect`（**屏幕绝对逻辑坐标**）、`scale`（float）、
  `name`、`current_workspace`、`active`、`focused`、**`primary`**
  （真实字段 —— KWin 只有 activeScreen 代理，sway 直接可用）。
- 单输出实测：Virtual-1，rect={0,0,2336,1261}，scale=1.0，
  primary=false（该 VM 无主屏声明，ListMonitors 的 Primary 需容忍
  全 false —— 与 KWin 的 activeScreen 语义并列兜底）。

## 9. 截图：grim（已验证）

```sh
grim -g "100,50 400x300" /tmp/out.png   # 注意 WxH 用 'x' 不是空格
```

- `-g` 几何是**逻辑坐标**（与 get_tree rect 同口径）；输出 PNG 尺寸
  = 逻辑尺寸 × scale（scale=1.0 实测 400×300 请求 → 400×300 输出）。
- scale>1 时输出为设备像素 → 按 design.md §5 降采样到逻辑尺寸
  （复用 `wayland/capture/`）。**方向与 KDE 相同**（设备像素 → 逻辑
  点），spec 17 草稿里"归一化方向与 KDE 相反"的说法不成立，以本
  实测为准。
- **越界矩形不裁剪**：`-g "2000,1000 500x400"`（屏幕仅 2336×1261）
  仍输出完整 500×400，出屏区域为空内容。windowctl 的 bounds 都来
  自窗口 rect（天然在屏内），无需特殊处理，但不得对 bounds 做
  "看起来更聪明"的夹取（grim 会自己给出与请求同尺寸的结果）。
- grim 是 Wayland 客户端：需要 `WAYLAND_DISPLAY` + `XDG_RUNTIME_DIR`
  （SSH 注入时与 SWAYSOCK 一起给）。

## 10. 光标位置（不可用，已确认）

sway IPC 无光标位置查询：`get_tree`/`get_outputs`/`get_seats` 均
无 cursor 字段（design.md §4 矩阵 ❌ 的实测确认）。`CursorPosition`
在 sway 后端返回明确的 ErrNotImplemented（错误信息说明 sway 无此
途径）。ydotool 只能写不能读，uinput 不回读光标位置。

## 11. 输入：ydotool（已验证）

- **ydotoold 默认不运行**（KDE VM 上是开机服务，sway VM 需自行启
  动；用户级 `ydotoold &` 即可，用户已在 `input` 组，`/dev/uinput`
  可写）。
- `ydotool mousemove --absolute` / `type` / `key 36:1 36:0` 全部
  exit 0，实测正常。与 KDE/GNOME 共用 `wayland/input.go`（uinput
  与 compositor 无关，符合预期）。
- **✅ 已验证（2026-10，sway 会话复验）：--absolute 绝对落点不可靠**。
  用"点击后焦点落位"实验（两个浮动 foot 放在已知位置，ydotool
  move + click 后读 swaymsg get_tree 的 focused 节点）：同一请求
  （1200,250）落进了目标窗口（→1:1 的一次），而（990,250）落空、
  （1350,250）也落空——落点**非单调、每次不同**。根因与 Hyprland
  侧定论一致（hyprland-mechanism.md §7）：--absolute 是
  "INT32_MIN 相对 slam + 偏移"的 hack，slam 残差经 libinput
  adaptive 加速后每次不同（ydotool --help 原话即要求关闭加速）。
- **解法（已实现）：sway 有精确光标通道** ——
  `swaymsg "seat seat0 cursor set X Y"`（绝对布局坐标，精度实测
  1:1：10px 空隙落空、窗口边缘 ±3% 内全中）。sway Backend 实现
  `pointerMover.MoveCursor`（与 Hyprland 的 `dispatch movecursor`
  同构），共享 input.go 的 MouseMove/MouseClick 定位优先走它；
  ydotool 只承担按钮（click）与键盘。即 sway 无需用户关加速。
- seat 名：默认 seat `seat0`（单 seat 环境；多 seat 时如有需求再
  按 get_seats 解析）。
- 合并发现的**列表序 bug**：get_tree 的 `floating_nodes` 列表序是
  创建序**不是** z 序（focused 浮动窗口不在首位），而公共层
  stampFocused 依赖"frontmost first"顺序——曾把点击聚焦的第二个
  窗口盖错。已修复：walkWindows 改按每个容器的 `focus` 数组
  （子节点焦点新近序）遍历，输出即 frontmost-first。

## 12. 测试应用备注（smoke 用）

- `gnome-calculator` 在 sway 上启动即崩（GTK3 协议错误
  `Error 71 dispatching to Wayland display`）——KDE/GNOME smoke 的
  老角色不能复用。
- `foot` 原生 Wayland 稳定；标题会被 shell 的 OSC 转义覆盖成
  `user@host:~`，**识别窗口用 `--app-id=wctl-*`**（app_id 稳定且
  不会被 shell 覆盖）。

## 13. 对 design.md 的实现输入（P3 sway 定稿要点）

| 项 | 结论 |
|----|------|
| 通道 | shell-out `swaymsg`（i3 IPC），无 godbus 依赖 |
| 依赖 | `swaymsg`、`grim`（LookPath 明确报错）；ydotool 共享 |
| 窗口 ID | get_tree 节点 `id` 的字符串形式；criteria `[con_id=N]` |
| 平铺窗口 Move | 先 `floating enable`（已决策，改变布局如实标注） |
| Move 落位 | resize set(W, H+deco_h) → move absolute position(X, Y−deco_h)；先 resize 后 move |
| ListMonitors | get_outputs：rect/scale/primary 真实字段 |
| CursorPosition | ❌ 明确 not-implemented（sway 无途径） |
| 截图 | grim -g 逻辑坐标 → scale>1 降采样（方向与 KDE 相同） |
| detect | `XDG_CURRENT_DESKTOP` 补 sway 兜底；SSH 注入必须显式 SWAYSOCK |
| 错误语义 | swaymsg exit 2 + JSON error 体 → core.ErrNoMatch 等映射 |
| 输入 | ydotool 共享（按钮/键盘）；**光标定位走 `seat cursor set`（精确 1:1，已验证），ydotool --absolute 在默认加速配置下落点不确定**（见 §11） |
| 列表序 | walkWindows 按容器 `focus` 数组（焦点新近序）遍历 = frontmost-first；floating_nodes 列表序是创建序、非 z 序（已修复的盖章 bug） |
