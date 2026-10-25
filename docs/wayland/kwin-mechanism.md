# KWin 脚本注入机制：实测细节

> 2026-10 在 KWin 6.7.4 / Plasma 6.7.4 上逐项验证。本文件记录
> [design.md](./design.md) 之外的全部踩坑细节，实现时的“为什么
> 这么写”都源于这些实验。

## 1. 为什么不能纯 shell-out

KWin 的窗口管理没有公开的一次性 D-Bus 方法。`org.kde.KWin` 提供的
脚本接口（`/Scripting`）只能**注入并运行 JS 脚本**：

```
method int  org.kde.kwin.Scripting.loadScript(QString filePath, QString pluginName)
method void org.kde.kwin.Script.run()      # 路径 /Scripting/Script<id>
method void org.kde.kwin.Script.stop()
method bool org.kde.kwin.Scripting.unloadScript(QString pluginName)
```

脚本对象的 introspection 只有 `run()` / `stop()` 两个方法，**没有
信号**——`print()` 的输出（“Prints ... as a D-Bus signal”）无法从脚本
对象本身捕获。结果回传的唯一途径是脚本内的 `callDBus(service, path,
interface, method, arg, callback)` 调用**我们自己拥有的 D-Bus 服务**。

`gdbus` / `dbus-send` / `qdbus` 只能发起调用、不能拥有 bus 名，因此
纯 shell-out 拿不到执行结果 → 必须用 `godbus/dbus`。

## 2. 回调通道（已验证）

```
Go 进程                                    KWin
  ├─ ConnectSessionBus()
  ├─ Export(reportObject, path, iface)      ├─ loadScript(tmp.js, name) → scriptID
  ├─ RequestName(unique-bus-name)           ├─ /Scripting/Script<id>.run()
  ├─ loadScript(...) ─────────────────────► │     JS: 操作完成后
  ├─ run(...) ────────────────────────────► │     callDBus(我们的 bus 名,
  │                                         │             ..., 'Report', json)
  │  ◄──────── Report(json) ────────────────┤
  ├─ stop() + unloadScript(name)
```

- bus 名必须**每进程唯一**：`org.muthuishere.windowctl.kde.<pid>`。
- `loadScript` 若同名脚本已加载返回 `-1` → 先 `unloadScript` 再重试。
- 回调带**调用序号 tag**，runner 丢弃 tag 不匹配的迟到回调
  （超时调用可能留下迟到回调）。
- 超时 5s：回调未到 → 明确错误“KWin script did not report back”。

## 3. 窗口枚举（已验证）

```js
// 仅演示 stackingOrder 的遍历方式（自底向上）——适配器实际用的
// 是倒序 + 活动窗口置首，见 §3.1。
for (var i = 0; i < workspace.stackingOrder.length; i++) {
    var w = workspace.stackingOrder[i];
    if (!w.normalWindow || w.deleted || w.outline) continue;
    var g = w.frameGeometry;
    // w.caption → title；w.resourceClass → app；w.pid → pid
    // String(w.internalId) → "{uuid}" 带花括号的稳定 ID
}
```

- `stackingOrder` 即 z-order（后覆盖前），`active` 字段可判定聚焦。
- `normalWindow` 过滤排除 panel/dock/OSD 等，与 darwin 的 layer-0
  过滤语义对齐；注意 type=DIALOG 的窗口不在 `normalWindow` 内
  （有意为之：windowctl 面向应用窗口）。
- `frameGeometry` 是**含边框**的几何，与 wmctrl/darwin/windows 的
  frame 语义一致。
- 属性可以是小数（实测 `x=17.7`），Go 侧 `Math.round` 取整。

### 3.1 列表序：必须是 frontmost-first（2026-10 实测并修复）

公共层的 `stampFocused` 取"落在 focused 屏上的**第一个**窗口"盖
`Focused`，所以适配器必须返回 **frontmost-first**。而
`stackingOrder` 是**自底向上**的，直接照抄它会让 `Focused` 永远落在
**最底下**那个窗口上——KDE 上曾因此把 `type`/`key` 的焦点校验打成
2s 超时（`input NOT sent`），整条"安全输入路径"不可用。

修法两步（`listWindowsBody`）：

1. **倒序**遍历 `stackingOrder`（`for (var i = so.length - 1; i >= 0; i--)`）
   → 得到 frontmost-first；
2. **把 `workspace.activeWindow` 提到列表首位**。

第 2 步不能省：**KWin 的活动窗口不一定是最顶层的那个**。实测采样到
"konsole 是活动窗口、而 Vmware 压在它上面"的状态；只倒序的话
`Focused` 仍然盖错窗口。

顺带验证的事实：

- `workspace.activeWindow = w`（`Focus` 用的写法）**会** raise 窗口
  ——所以刚 focus 完 active == topmost，但一般情况下不成立。
- 每个窗口还有 `w.active` 布尔字段，与 `workspace.activeWindow`
  一致，可作为交叉校验。

其他后端（darwin `CGWindowListCopyWindowInfo`、windows
`EnumWindows`、sway 容器 `focus` 数组、hyprland `focusHistoryID`）
本来就是 frontmost-first —— 这条约定是**跨适配器**的，sway 当初也
踩过同一个坑（见 `sway-mechanism.md` §11）。

## 4. 移动窗口：三个失败写法 + 一个正确写法（全部实测）

| 写法 | 结果 |
|------|------|
| `var g = w.frameGeometry; g.x = 700; w.frameGeometry = g;` | ❌ 无变化（读回的是快照，赋回不被接受） |
| `w.frameGeometry.x = 700; w.frameGeometry.y = 500; w.frameGeometry.width = 600; w.frameGeometry.height = 400;` | ⚠️ **只有最后一次写入生效**（每次是“读-改-写整个矩形”，而同 tick 内读到的都是过期快照，连续写互相覆盖） |
| `w.frameGeometry = { x: 900, y: 300, width: 500, height: 350 };` | ✅ **精确落位** |

根因：QJSEngine 读 `frameGeometry` 返回**快照对象**（非活引用），
而 Wayland 几何变更是异步的（要等客户端响应 configure 事件），同
一脚本 tick 内的读取互相过期。整体赋一个字面量对象则被
QJSEngine 正确转换为 QRectF 并**一次性**应用。

**正确写法**（adapter 采用）：

```js
w.frameGeometry = { x: X, y: Y, width: W, height: H };
```

补充：`Qt` 全局对象在 KWin 脚本中**不可用**（`typeof Qt ===
"undefined"`），不能写 `Qt.rect(...)`。

## 5. 聚焦（已验证）

```js
workspace.activeWindow = w;   // KWin 6 合法写入
```

实测 `active: false → true`。

## 6. 显示器（已验证）

```js
workspace.screens           // KWin::Output 列表
  s.geometry                // QRect，逻辑坐标（已含 scale 折算）
  s.devicePixelRatio        // HiDPI 缩放
workspace.activeScreen      // 有活动窗口的屏幕（Primary 的代理）
workspace.cursorPos         // 光标位置（CursorPosition 数据源）
```

- 没有多显示器机器可测；`spectacle -f` 抓取虚拟桌面（所有显示器
  合并）的假设**未经验证**，文档中标注。
- KWin 脚本 API 不暴露“primary output”概念，用 `activeScreen` 代理。

## 7. 截图（已验证）

KWin 6 上 `org.kde.KWin` 下**没有** Screenshot 节点（递归 introspect
确认；该接口在 Plasma 5 时代存在，6.x 已移除）。可用途径：

- `spectacle -b -n -f -o out.png` —— 全屏（虚拟桌面）后台抓取，
  实测 0.6s、2560×1440（= 逻辑分辨率 × dpr=1）、RGBA PNG。
  默认**不**写剪贴板（需显式 `-c`）。tesseract 警告是 spectacle
  OCR 功能缺失，无害。`-b` 后台模式确认不弹 GUI 窗口。
- 区域抓取：门户 `org.freedesktop.portal.Screenshot` 只支持整屏 +
  交互式，无法程序化指定矩形 → 不采用。

点归一化：spectacle 出设备像素（HiDPI 下为逻辑分辨率 × dpr），
Go 侧按 `bounds × dpr` 裁剪后**降采样到 bounds 尺寸**，维持
“1 图像像素 == 1 坐标点”。

## 7.5 org.kde.KWin 的 /KWin 查询接口（坑）

- `queryWindowInfo()` —— **不是查询**：它打开交互式点选对话框，
  劫持光标，返回值是用户点选的窗口；用户取消时返回
  `org.kde.KWin.Error.UserCancel`。脚本里绝对不能用。
- `getWindowInfo(uuid)` —— 普通调用，返回该窗口的属性
  （caption/geometry/pid/...），可可靠用于单窗口查询。
  注意**没有 active 字段**，无法用它验证聚焦。

（实测：queryWindowInfo 有时立即返回活动窗口信息——行为取决于
内部状态，不可依赖。）

## 8. 输入：ydotool（本机未装，CLI 语法经官方 README 核实）

- 必须配合 `ydotoold` 守护进程（持久的 uinput 虚拟设备）。
- `/dev/uinput` 默认 root-only → 需 udev 规则 + input 组 + 服务。

```
ydotool key 29:1 56:1 59:1 59:0 56:0 29:0   # keycode:状态(1=按下,0=抬起)
ydotool type 'text'                          # 打字（布局相关）
ydotool mousemove --absolute -x 100 -y 100
ydotool click 0xC0                           # 左键；0xC1 右；0xC2 中
ydotool click --repeat 5 --next-delay 25 0xC0
```

- 键盘是 **keycode 制**（不是 keysym），与窗口布局相关 → 需要
  evdev keycode 映射表（见 `internal/adapter/linux/wayland/keys.go`）。
- 许可：AGPLv3。仅 shell-out 调用，不链接，不影响 windowctl 许可。

## 9. 环境注意

被测机器同时存在 `DISPLAY=:1`（XWayland）与 `WAYLAND_DISPLAY`
——会话判定必须优先信 `XDG_SESSION_TYPE=wayland`，不能只看
`DISPLAY` 是否存在。
