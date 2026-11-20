# shellkit

一个小工具，用来验证 [magpie](https://github.com/yetone/magpie) 那样的**一核多壳**结构：一个 Go 二进制里放一个核（笔记的增删查 + 一个 JSON 文件 + 一个 HTTP 面），壳只是同一份核上的不同出入口，编译时用 build tag 挑。

功能是借口，结构才是内容。真正被验证的是四条规矩：

1. **核不知道壳存在。** `internal/core` 不 import 任何壳，`deps_test.go` 会失败拦住你。
2. **一个二进制，多个形态。** CLI / web / TUI / desktop / 容器 / release，全部来自同一份源码和同一组 tag，不是分叉的仓库。
3. **前端零框架零构建。** 三个静态文件（`index.html` / `app.css` / `app.js`），`//go:embed` 打进二进制，`assets_test.go` 用字节预算拦着不让它悄悄长成一个前端工程。
4. **能被检验。** `go vet` + `go test` 覆盖三系统；桌面壳和 Docker 真编译、真推送，只是发生在本机缺依赖时改由 CI 做。

## 形态

| 产物 | 构建 tag | 是什么 |
|---|---|---|
| `shellkit` | 无 | 默认：命令树 + 网关 + 内嵌页面（`web`） |
| `shellkit-cli` | `nogui` | 没有页面，适合服务端 / 容器 |
| `shellkit-tui` | `tui` | 终端里一枚 ANSI 面板，秒级刷新 |
| `shellkit-desktop` | `desktop` | Wails v3 的原生窗口，指向同一个网关 |
| `shellkit-desktop` (gtk3) | `desktop gtk3` | 老发行版：GTK3 + WebKitGTK 4.1 |
| 容器 | `nogui` | 多阶段构建，非 root，只暴露 3427 |
| release 资产 | 交叉编译 | linux/darwin/windows × amd64/arm64 + `SHA256SUMS` |

壳之间不共享渲染代码：桌面窗口只是把 webview 指向 `http://127.0.0.1:3427/`，和浏览器看到的完全是同一张页面。

## 快速开始

Go 版本按 `go.mod` 走（1.27.4）：`go` 命令在 Go ≥ 1.21 上会自动拉取 `toolchain` 指定的版本，不需要手工装。本机另装了一份在 `/home/user/tools/go1.27.4`，Makefile 检测到就用它，检测不到退回 PATH 上的 `go`。

```bash
make test                                  # gofmt + vet + 测试（含 nogui 那一档）
make sizes                                 # 三个壳的体积
./bin/shellkit add -t inbox "call the dentist"
./bin/shellkit list -n 5
./bin/shellkit web                         # http://127.0.0.1:3427/
./bin/shellkit serve                       # 同一个网关，不挂页面（容器里就是它）
./bin/shellkit configs                     # 看 claude / codex / gemini 的 model
./bin/shellkit configs set codex gpt-5     # 只改那一个 key，不重写整份文件
./bin/shellkit update --check              # 查 GitHub release（私有库要 GITHUB_TOKEN）
```

数据默认落在 `~/.shellkit/notes.json`，`SHELLKIT_HOME` 可改；监听地址 `SHELLKIT_ADDR` 可改。CLI 与正在跑的服务同时写同一个文件时有 flock + 进程内互斥，谁都不会丢；文件权限沿用原文件的。

## 一个核

```
internal/core   store.go   笔记：Add / List / Remove / Count，临时文件 + rename 落盘
                server.go  HTTP 面：/healthz /api/state /api/notes
internal/ui     页面：3 个静态文件，embed；nogui 档换成 501 说明
internal/tui    终端：读 stdin 的 q，每秒重画
internal/desktop 窗口：Wails v3，只在 -tags desktop 下编译
internal/configs 三个真实 agent 配置文件的读写（JSON 解码后再编码、TOML 只碰顶层 key）
internal/update  GitHub release → 下载 → sha256 → rename 替换，失败留 .old 可回滚
internal/cli     命令树，唯一一个所有构建都有的壳
```

API：

```
GET  /healthz
GET  /api/state
GET  /api/notes?q=&limit=
POST /api/notes      {"text":"…","tags":["…"]}   → 201
DELETE /api/notes/{id}
```

## 桌面壳为什么不在这里编

Wails v3 在 Linux 上默认要 GTK4 和 WebKitGTK 6.0 的头文件（要 GTK3 + WebKitGTK 4.1 就加 `gtk3` tag）。开发机（云主机，无桌面）没有这些，也不需要为了一个原型装一套 GUI 依赖：`internal/desktop/desktop_on.go` 只在 `-tags desktop` 时参与编译，`go build ./...` 永远不会碰到它。真编译交给 `.github/workflows/desktop.yml`：那里装齐 GTK3/GTK4 与 WebKitGTK 4.1/6.0，分别跑 `-tags desktop` 和 `-tags "desktop gtk3"` 两条。`-tags "desktop nogui"` 会被直接拒绝——窗口加载的正是被 nogui 换掉的那张页面。

这就是"本机没有就走 CI"的具体形状：约束不藏起来，而是写成一条会红的流水线。

## CI

| workflow | 干什么 |
|---|---|
| `test.yml` | ubuntu/macos/windows 三系统：vet、test（gofmt 与 -race 只在 ubuntu），外加 `-tags nogui` / `tui` / `tui,nogui` 的编译 |
| `desktop.yml` | Linux 装 GTK3/GTK4 与两个 WebKitGTK，真编 `-tags desktop` 与 `-tags "desktop gtk3"`，产物上传 |
| `docker.yml` | 构建镜像并推 `ghcr.io/llwsb001/shellkit`（main 与 tag） |
| `release.yml` | 打 `v*` tag：六份交叉编译资产 + `SHA256SUMS`，建 release |

私有仓库的 release 与 package 都用仓库自带的 `GITHUB_TOKEN`，不需要另配 secret。普通 push 跑 test / docker；`git push origin v0.1.0` 才会跑 release。Docker 镜像先在本机 daemon 里 load 一份跑冒烟（PR 也跑），通过后再多架构推 `ghcr.io`。

## 不做什么

不做多协议互译（那是 magpie 的活，这个小工具不需要）。不做账号体系、不做插件市场、不做 89 个 agent 的接线。笔记就是笔记。
