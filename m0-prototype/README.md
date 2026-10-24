# DeskCanvas — M0 技术验证原型

验证目标只有一个问题：**一个 Go + Wails 的透明窗口，能不能真的沉到 macOS 桌面层，并能在「锁定（只看、鼠标穿透）↔ 编辑（可操作）」之间可靠切换？**

通了，整个产品成立；不通，及时换 Swift 薄壳，不浪费 M1 的工。

## 结构

| 文件 | 作用 |
|---|---|
| `main.go` | Wails 入口：无边框透明窗口、透明 WebView（注：Wails v2.16 没有 ActivationPolicy 选项，无 Dock 图标由原生层实现） |
| `app.go` | 模式状态机（locked/edit）+ `/api/state` `/api/toggle` `/api/mode` `/api/level` 四个 HTTP 接口（前端用 fetch 调用，无需生成 bindings） |
| `platform_darwin.m` | **核心**：把窗口设到桌面图标层附近、穿透开关、铺满主屏、Carbon 全局快捷键 ⌥⌘D |
| `platform_darwin.go` | cgo 桥 + 快捷键回调进 Go |
| `platform_other.go` | 非 macOS 桩：只打日志，保证 Linux 上也能 build/vet |
| `frontend/dist/` | 验证画布：左上验证面板（模式徽章、4 项验收勾选、层级 offset 切换）+ 记事本/表格/贴图三张示例卡（编辑模式可拖标题栏） |

## 在 Mac 上跑

前置：Go（与 magpiedb 同一套即可）；有 Wails CLI 更好，没有也能跑。

```bash
./build-mac.sh
# 有 wails CLI → open build/bin/DeskCanvas.app
# 无 wails CLI → ./build/bin/deskcanvas
```

启动后应用默认就是**锁定模式**：画布应贴在桌面上，点不动它。用 **⌥⌘D** 切到编辑模式（面板按钮、卡片拖动这时才有效），再按一次沉回去。退出：编辑模式下关窗口，或终端 `pkill deskcanvas` / 活动监视器结束。

## 验收清单（4 项，全过才算 M0 通过）

| # | 操作 | 期望 |
|---|---|---|
| 1 | 打开任意应用（如访达/Safari） | 应用窗口盖住画布；画布不浮在任何窗口上面 |
| 2 | 锁定模式下点击画布位置处的桌面图标 | 点击穿透，图标正常被选中/双击打开；画布纹丝不动 |
| 3 | 按 ⌥⌘D | 切到编辑模式：徽章变橙、面板可点、卡片标题栏可拖；再按一次沉回锁定 |
| 4 | 切换 Space、在调度中心逛一圈再回来 | 画布仍在桌面层，不丢、不跑到别的 Space |

外加一项定参数：**层级 offset**。面板里依次试 -5 / -1 / 0 / +1 / +5，看卡片与桌面图标的上下关系，把「图标完整可见、卡片又不显得被压在壁纸里」的那档记下来——这就是 M1 要写死的默认值（当前默认 -1）。

验收结果记录（在 Mac 上跑完填）：

| 项 | 结果（✅/❌） | 备注 |
|---|---|---|
| 1 窗口覆盖 | | |
| 2 锁定穿透 | | |
| 3 ⌥⌘D 切换 | | |
| 4 Space 跟随 | | |
| 最佳 offset | | |

## 自动更新（v0.1.1-m0 起）

发布方式：给仓库打 `v*` 标签推送，CI 会把标签注入二进制版本并自动创建 GitHub Release（挂 DMG）。已安装的应用启动后自动检查 Release，发现新版会在面板提示，点「更新并重启」即自动完成：下载 DMG → 按 GitHub 资产 digest 校验 SHA-256 → 解出 .app → 新二进制以 `--apply-update` 模式换包、去隔离属性、重启；旧包保留为 `DeskCanvas.previous.app`，新版正常启动后自动清除，换包失败会自动回滚。

仓库自 2026-10-24 起为**公开**仓库（用户拍板，图省事），应用可直接读取 Release，**不需要任何令牌**。（若将来再改回私有，更新器也支持从环境变量 `DESKCANVAS_UPDATE_TOKEN` 或文件 `~/Library/Application Support/DeskCanvas/update-token` 读一个仅本仓库 Contents 只读的 fine-grained PAT；令牌永不写进二进制。）

## 已知边界

- 只处理主显示器；多显示器是 MVP 的 M1 之后的事。
- `platform_darwin.*` 只能在 Mac 上编译验证（cgo + Cocoa/Carbon），Linux 侧已验证的是非 darwin 桩构建与前端语法——与 magpiedb 的 darwin 代码同样的验证方式。
- 无签名/公证，自用路线，与既定策略一致；若从浏览器下载构建产物，首次打开需右键「打开」。
- 若第 2/3 项失败（穿透或层级不生效）：检查启动终端日志里 `deskcanvas: applied locked=… level=…` 与 `hotkey … register status=0`，把日志带回来定位。
