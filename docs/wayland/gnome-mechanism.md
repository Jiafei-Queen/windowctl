# GNOME Shell 扩展机制：实测细节

> 2026-10 在 Debian 13 (trixie) / GNOME Shell **48.7** / Wayland 上
> 逐项验证。本文件记录 [design.md](./design.md) §8 P2 的前置 spike
> 结论与全部踩坑细节，实现时的“为什么这么写”都源于这些实验。

## 1. 机制总览：常驻扩展 D-Bus 服务

与 KWin 的“一次性脚本注入 + callDBus 回调”不同，GNOME 的窗口管理
走**常驻 GNOME Shell 扩展**：扩展在 Shell 进程内以
`Gio.bus_own_name` 持有会话总线名并 export D-Bus 对象，windowctl
侧（godbus）**直接方法调用**——无需回调、tag、超时机制，D-Bus 自带
超时。

```
windowctl (godbus)                     GNOME Shell 进程 (扩展)
  ├─ ConnectSessionBus()
  ├─ proxy.Call("Move", json) ───────► 扩展 export 的对象
  │   （同步/异步 reply，D-Bus 超时兜底）   ├─ global.get_window_actors()
  │                                       ├─ meta_window.move_resize_frame()
  │                                       └─ new Shell.Screenshot()（进程内截图）
```

## 2. 扩展生命周期（spike #1 结论）

**结论：新装扩展无法免注销生效；必须注销登录一次。此后每次登录
自动加载，无需再干预。**

（正式实现：扩展源码随二进制 `go:embed` 分发
（`wayland/gnome/extension/`），windowctl 首次调用发现服务不可达时
自动完成"拷文件 + gsettings 预置"，并把"需注销一次"作为明确错误
返回；服务可达但报 ServiceUnknown 且文件已就位时，错误提示区分
"刚装完待注销"与"锁屏中"两种情形。VM 集成验证覆盖了完整
fresh-install 路径。）

实测记录（Shell 48.7，并核对 48.7/main 源码 `js/ui/extensionSystem.js`）：

| 尝试 | 结果 |
|------|------|
| 拷入 `~/.local/share/gnome-shell/extensions/<uuid>/` 后 `gnome-extensions enable` | ❌ “扩展不存在”——Shell 不感知 |
| 切换 `UserExtensionsEnabled` 属性（D-Bus） | ❌ 只影响已知扩展列表，不重扫描 |
| 触碰扩展目录/文件（猜测 FileMonitor） | ❌ 无 FileMonitor，无反应 |
| D-Bus `ReloadExtension`（`/org/gnome/Shell`） | ❌ 已废弃，返回 `NotSupported` |
| `disable` → `enable` | ❌ 状态照旧——**模块 import 有缓存，不重新执行** |
| 改 `extension.js` 后重启会话 | ✅ 新代码生效 |
| （实测陷阱）VM 空闲锁屏 | ⚠️ 锁屏期间 Shell 切到 unlock-dialog 模式，未声明 `session-modes` 含 `unlock-dialog` 的扩展被**统一 disable**（bus 名消失、无任何错误日志）；解锁后随 enabled-extensions 自动恢复 |

源码佐证：目录扫描只在 `_loadExtensions()`（会话启动）跑一次；
48 与 main（51.x）均无重扫描入口，也无目录监听。

**锁屏陷阱对实现的影响**：agent 会话可能超过用户空闲阈值，锁屏
会让服务中途消失（且用户输入不会重置 idle 计时器——agent 本来就
不打键盘）。正式扩展因此声明
`"session-modes": ["user", "unlock-dialog"]`，锁屏期间保持服务。

另两个关键开关（Debian 13 实测值）：

- `org.gnome.shell allow-extension-installation`：扫描用户目录的
  前提，**必须为 true**（Debian 默认已是 true）。
- `org.gnome.shell enabled-extensions`：直接写入即可**预置 enable
  状态**，登录后自动激活（实测有效，无需任何手动操作）。

**对实现的影响**：windowctl 自动安装 = 拷文件 + 写
`enabled-extensions` + 提示用户注销一次；文档如实标注这是
GNOME 48+ 的会话语义，不是 windowctl 能绕过的。

## 3. D-Bus 服务通道（spike #2 结论）

扩展端（ESM，GNOME 45+ 格式）：

```js
import Gio from 'gi://Gio';
import GLib from 'gi://GLib';

export default class Extension {
    enable() {
        this._ownerId = Gio.bus_own_name(Gio.BusType.SESSION, BUS_NAME,
            Gio.BusNameOwnerFlags.NONE,
            this._onBusAcquired.bind(this), null, null);
    }
    disable() { Gio.bus_unown_name(this._ownerId); }

    _onBusAcquired(conn) {
        const info = Gio.DBusNodeInfo.new_for_xml(IFACE_XML);
        conn.register_object(OBJ_PATH, info.interfaces[0],
            (c, sender, path, iface, method, params, invocation) => {
                // 处理后：
                invocation.return_value(new GLib.Variant('(s)', [json]));
            }, null, null);
    }
}
```

实测确认的细节：

- **`conn.register_object` 必须 5 个参数**（get/set property 回调
  必须显式传 `null, null`），GJS 会按参数个数校验，少了直接抛
  TypeError。
- **延迟回复可用**：handler 内起异步操作，稍后在 Promise 的
  `then` 里 `invocation.return_value(...)` 即可，GDBus 允许。
- 扩展持有的 bus 名在 `busctl --user list` 里可见，owner 显示为
  gnome-shell 进程。
- 外部调用用 `dbus-send` / godbus 均可；方法回传 JSON 字符串。

## 4. GJS 环境的两个坑

### 4.1 `Gio._promisify` 包装（截图 API 的签名陷阱）

Shell 启动时（`js/ui/screenshot.js`）对 `Shell.Screenshot.prototype`
做了 `Gio._promisify`，因此原型上出现 `_original_*` 方法。**必须用
promise 式调用**；若仍传 callback，包装器自己创建的 promise 无人
await → **unhandled promise rejection → Shell 直接禁用扩展**
（bus 名消失、状态 INACTIVE，实测踩坑）。

### 4.2 `file_get_contents` 返回 Uint8Array

`GLib.file_get_contents` 返回 `Uint8Array`。在独立 gjs 里隐式
toString 仍是字节字符串（旧行为），但在 **Shell 进程内会得到
逗号分隔的数字串**，`eval()` 后是合法的逗号表达式——静默求值为
数字、不报错、什么都不做。必须显式 `new TextDecoder().decode(bytes)`。

（spike 用了“薄壳 extension.js + eval 热重载 logic.js”的架构验证
API，FileMonitor 触发重 eval，免注销迭代。**正式实现不用 eval**，
扩展代码直接落盘，改动走重新登录。）

## 5. API 逐项实测（spike #3 结论）

### 窗口枚举

```js
for (const actor of global.get_window_actors()) {
    const w = actor.meta_window;
    // 稳定 ID：String(w.get_stable_sequence())  ← 数字，对应 KWin 的 "{uuid}"
    w.get_title() / get_wm_class() / get_gtk_application_id() / get_pid()
    w.get_window_type()   // 0 = NORMAL（对应 KWin normalWindow 过滤）
    w.get_frame_rect()    // 逻辑坐标，含装饰（对应 KWin frameGeometry）
    w.get_buffer_rect()   // 客户区
    w.get_maximized() / is_skip_taskbar() / get_monitor()
    w.get_workspace().index() / has_focus()
}
```

**列表序 = 自底向上（stacking order）**：`global.get_window_actors()`
按 z-order 返回 actor，**最后**一个才是最前的。公共层的 `stampFocused`
取"落在 focused 屏上的第一个窗口"，所以扩展必须**把焦点窗口提到首位**
——否则 `Focused` 会盖在最底下那个窗口上（与 KWin 同一个 bug）。

锚点用 `global.display.get_focus_window()`（与 `listMonitors` 里算
focused 屏用的是同一个），比对用 `get_stable_sequence()` 而非对象
`===`——GJS 两次调用拿到的 Meta.Window 包装不保证是同一个 JS 对象。

注意：**焦点窗口不一定是最顶层的那个**（别的窗口可以被抬到它上面），
所以不能靠"最后一个"来代替焦点信号。

### 扩展代码改了必须重新登录（ESM 缓存）

GNOME 45+ 用 `import()` 加载扩展，模块按 **URL** 缓存。同一个扩展
目录的 URL 不变 → `gnome-extensions disable/enable` 拿到的还是旧模块
（实测：改了 `extension.js` 再 enable，D-Bus 回复的字段仍是旧的）。
`org.gnome.Shell.Extensions.ReloadExtension` 在 48.7 **未实现**；
`gnome-shell --replace` 在 Wayland 上也接不了已有 seat（实测它会另起
一个 `wayland-1`，变成嵌套会话）。

→ **改扩展代码 = 必须登出登入一次**。这与 `install.go` 注释里说的
"新装的扩展要登出登入一次才生效"是同一个原因。

顺带一个实测坑：`disable()` 只 `Gio.bus_unown_name()`，**没有**
`conn.unregister_object()`（`register_object` 返回的 id 也没留）。
所以同一会话里 disable → enable，新实例的 `register_object` 会因为
路径已占用而失败，**旧 handler 继续应答**（实测：disable 之后
`ListWindows` 仍能调通，返回的是 disable 之前的代码）。GNOME 每次
登录都是新进程，所以平时碰不到。

### 移动

```js
w.move_resize_frame(true, x, y, width, height);  // 5 参！首参 user_op
```

实测移动后**立即读回已是新值**（GNOME 内部同步应用；与 KWin 的
异步落定不同）。smoke 断言仍保留容差，但不依赖等待。

**边界：刚 map 的窗口会忽略几何修改**——实现阶段的 smoke 观测到，
窗口刚被 launch/映射后立即 move_resize_frame 静默无效（返回 ok 但
几何不变），等约 1s（初始 configure 周期完成）后同一调用正常生效。
smoke 脚本因此在检测到窗口后先等 1s 再 move。

### 聚焦

```js
w.activate(global.get_current_time());  // 实测 has_focus 立即为 true
```

### 显示器

```js
const d = global.display;
d.get_n_monitors();
d.get_monitor_geometry(i);   // 逻辑坐标 {x,y,width,height}
d.get_monitor_scale(i);      // 存在（Meta.Display 原型实测确认）
d.get_primary_monitor();     // 主屏序号
// get_monitor_name 不存在（原型过滤实测确认）；名称需走 MonitorManager
```

### 光标

```js
const [x, y, mods] = global.get_pointer();
```

### 截图

- D-Bus 接口 `/org/gnome/Shell/Screenshot`（`ScreenshotArea` 签名
  `iiiibs → bs`）有**调用方白名单**（`DBusSenderChecker`：portal、
  gsd-media-keys、org.gnome.Screenshot 等四个），外部直接调用
  `AccessDenied`——**必须经扩展进程内 API**。
- 进程内正确用法（抄自 `ScreenshotService.ScreenshotAreaAsync`）：

  ```js
  const shot = new Shell.Screenshot();
  const stream = Gio.File.new_for_path(path)
      .replace(null, false, Gio.FileCreateFlags.NONE, null);
  await shot.screenshot_area(x, y, w, h, stream);  // 无 cursor/filename 参
  stream.close(null);
  ```
- **坐标是设备像素**：D-Bus 层传逻辑坐标，服务内部
  `_scaleArea` 乘 `St.ThemeContext.get_for_stage(global.stage)
  .scale_factor` 后再调进程内 API。扩展实现需自行换算。
- scale=1 实测：输出 PNG 尺寸 == 请求的逻辑尺寸（360×616 →
  360×616 RGBA），内容与窗口位置目检一致。

### 启动应用（扩展内，仅 spike 用）

```js
Shell.AppSystem.get_default().lookup_app(id).launch(0, -1, []);  // ✅
```

windowctl 的 `Launch` 保持 exec 路径不变；此 API 仅作调试手段。

### 不可用路径

- `org.gnome.Shell.Eval`：方法存在但被锁（返回 `false`，需
  unsafe-mode），不可依赖。

## 6. 环境与检测注意

- 图形会话环境（`XDG_SESSION_TYPE=wayland`、
  `XDG_CURRENT_DESKTOP=GNOME`）在 gnome-shell 进程 environ 里；
  **SSH 会话是独立的 tty 会话**，`XDG_SESSION_TYPE=tty`。
- 从 SSH 跑 windowctl 测试 Wayland 后端需注入环境：
  `XDG_SESSION_TYPE=wayland XDG_CURRENT_DESKTOP=GNOME
  DBUS_SESSION_BUS_ADDRESS=unix:path=/run/user/1000/bus`。
- 这佐证了 design.md §2 “`XDG_SESSION_TYPE` 必须优先于
  DISPLAY”的检测顺序。

## 7. 对 design.md 的实现输入（P2 定稿要点)

| 项 | 结论 |
|----|------|
| 通道 | 常驻扩展 D-Bus 服务，直接调用；无 tag/超时机制（KWin 式回调不需要） |
| 窗口 ID | `String(get_stable_sequence())` |
| 移动 | `move_resize_frame(true, ...)`，观测为同步生效 |
| 截图 | 扩展内 `Shell.Screenshot` + GOutputStream；坐标 × scale |
| HiDPI | scale=1 实测；scale>1 的降采样沿用 design.md §5，方向与 KDE 相同（设备像素→逻辑点） |
| 输入 | ydotool 直接复用（未在 GNOME 重复验证，机制与 compositor 无关） |
| 安装 | 拷文件 + gsettings 预置 + 一次注销；`allow-extension-installation` 须为 true |
| 版本锁定 | metadata `shell-version: ["48"]`；Debian 13 = GNOME 48 |
