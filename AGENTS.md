# wxreport 开发说明

供开发者和 coding agent 使用。使用方文档见 [README.md](README.md)。

## 项目结构

```
cmd/wxreport/                  CLI：常驻定时 / --once 单次
internal/pipeline/             固定顺序编排整条流程（Steps 接口可注入 fake）
internal/siwx/                 SIWX HTTP API：DB 刷新、消息导出、昵称解析、SQLite 读取
internal/state/                增量状态：计划、差异合并
internal/report/               报告构建：结构化 JSON、HTML 渲染
internal/summarize/            OpenAI 兼容接口调用与总结提示词
internal/rank/                 话轮模型：话轮、有效参与人数、文字/密集度
internal/shot/                长图：headless 截图
internal/oss/                  阿里云 OSS 上传
internal/compress/             本地 256 色 PNG 压缩
internal/fsutil/               原子写入
.env.example                   配置模板
.github/workflows/ci.yml       push / PR 的 lint + test
.github/workflows/release.yml  手动触发的发布 workflow
.goreleaser.yml                跨平台构建配置
Makefile                       构建 / 测试 / 清理
data/                          运行数据（已 gitignore）
logs/                          daemon 日志目录（已 gitignore，可选）
```

## 入口与配置

- `cmd/wxreport/main.go` 是唯一非测试代码文件。
- 标准库 `flag` 解析，只有 `--once`；不带参数进入常驻 cron 模式。
- 启动时 `os.Getwd()` 作为根目录，`godotenv.Load()` 读取当前目录 `.env`；同名环境变量优先。
- 所有行为都由 `.env` 决定，没有命令行覆盖参数。
- 编译产物默认在项目根目录（`make build`），分发时与 `.env` 放同一目录。
- 配置优先级：环境变量 > `.env` > 代码默认值。
- `OSSConfig` 也会通过 `godotenv.Read(root/.env)` 读取 OSS 配置。

## 单次执行流程

`pipeline.Run` 按固定顺序执行，任一步失败立即返回：

1. 刷新 SIWX 解密缓存
2. 确定 24 小时窗口（当前时间往前 24h）
3. 导出消息到 `data/wxmsgs.json`；0 条消息则直接结束
4. 生成增量计划 `data/wxstate-plan.json`
5. 调用 OpenAI 兼容接口总结，输出 `data/wxstate-update.json`
   - 解析失败时把错误反馈给模型重试，最多 3 次
6. 合并差异，写回 `data/topics.json`
7. 生成 `data/reports/<YYYYMMDDHHMMSS>.json` 存档
8. 生成长图，按 `WXREPORT_COMPRESS` 做 256 色压缩，按配置上传 OSS

`pipeline.Steps` 抽象了 Refresh / Export / Summarize / Shot 四个外部副作用，测试用 fake 实现。

## 增量状态

`data/topics.json` 持久化上一轮所有活跃话题（不只 TOP10）：

```json
{"id":"t1","title":"...","commentary":"...","summary":"...[[res:1]]","message_ids":["p:..."],"resources":[...]}
```

每轮：

1. 导出当前 24h，每条消息带稳定 `mid`
2. 生成计划：
   - `new_messages`：上次 `window_end` 之后的新消息
   - `tail_messages`：最近 6 小时内未被归入任何话题的尾部消息（最多 500 条）
   - `topics`：上一轮所有活跃话题的摘要与活跃/过期条数
   - `active_messages`：对 `expired > 0` 的话题，列出当前窗口内仍在的消息（每个话题最多 120 条）
3. 模型只读计划，输出差异 JSON：分配消息、更新有变化的话题、新建话题；对 `expired > 0` 的话题重写摘要并收缩资源
4. 合并状态、移除 24h 外的 message_id、丢掉空话题
5. 把 message_ids 映射回当前 24h 消息，算分、截断 TOP10、生成参与者行并渲染

淘汰会同时影响排名、内容和资源列表：排名每轮按当前 24h 消息重算；有过期消息的话题会触发摘要重写；资源列表只展示当前摘要实际引用到的资源（状态里仍累加保留）。

尾部消息用于跨轮次话题；上一轮排 11-20 的话题也保留在状态里。首次没有状态时做全量 bootstrap。

## 话题排序

渲染报告时把每个话题的 `message_ids` 映射到当前 24h 消息，计算话轮指标：

```text
话轮：同一人在同一话题内连续消息算一个话轮；同一人两条消息间隔 >20 分钟、
      或中间切到别的话题，则拆开；换人立即新话轮
N_eff = 1 / Σ(pᵢ²)          pᵢ = 第 i 个人在该话题的话轮占比
θ     = 带文字话轮 / 总话轮
μ     = 密集话轮 / 总话轮    相邻话轮间隔 ≤5 分钟算密集
分数  = 0.45 × (N_eff / ΣN_eff) + 0.40 × θ × (话轮数 / Σ话轮数) + 0.15 × μ
```

- 连发几十条只算 1～2 个话轮
- 有效参与人数看交替发言，一人占 80% 话轮会拉低 N_eff
- 渲染时生成 `参与者：昵称(消息数)`，校正头部「消息总数/参与人数」，删除 `<!-- msgs -->` 注释和被截断话题的未引用脚注
- 排序完全确定性

## 昵称规则

导出阶段把 `sender_wxid` 解析成安全昵称，优先级：

1. 微信昵称：`contact.nick_name`
2. 群昵称：`contact.db` 的 `chat_room.ext_buffer`（protobuf 成员列表）
3. wxid：前两者都不可见时回退

「不可见」指整串只包含空白、换行、控制符、零宽/格式字符、组合标记和常见空白填充字符（如 `ㅤ`、U+2800）。微信昵称常被写成这种占位形式，此时自动改用群昵称。

**`remark` 字段完全不会被读取**，报告中不可能出现备注名。

## 报告与长图

报告由 `internal/report.Build` 从增量状态生成结构化 `Report`，写入 `data/reports/<YYYYMMDDHHMMSS>.json` 存档：

- 话题经 `rank.Score` 按话轮模型排序，取 TOP10
- 摘要里的 `[[res:n]]` 在构建时解析成 `resource` 片段，资源跨话题按 URL 去重编号
- 反引号人名解析成 `person` 片段，HTML 直接按片段渲染，不再生成 Markdown
- 统计窗口时间格式为 `MM-DD HH:MM`，不带年份
- `WXREPORT_PARTICIPANTS=false` 时不输出参与者

报告 HTML 由 `internal/report.HTML` 直接渲染 `Report`：

- 主题机制见下方「主题」章节；模板与样式入口在 `internal/report/themes/moments/report.*`
- 不使用 Markdown 渲染器，也没有 github-markdown css
- 标题优先 `WXREPORT_TITLE`，未配置时使用 `Report.Title`

长图由 `internal/shot`：

- headless Chrome 截图
- 默认输出 `data/latest-report.png`

OSS 上传由 `internal/oss`：

- 默认关闭，`WXREPORT_UPLOAD=true` 且 `OSS_*` 完整时才上传

压缩由 `internal/compress`：

- 解码 PNG → `median.Quantizer(256)` 量化 → `png.BestCompression` 重编码
- 纯 Go、无 cgo、无网络
- 压缩失败或结果不小于原图时，调用方保留原图
- 依赖 `github.com/soniakeys/quant`（MIT）

## 主题

主题按 `internal/report/themes/<name>/` 分目录，每个主题是一个独立 Go package，固定包含：

```text
internal/report/themes/<name>/
├── theme.go
├── report.html.tmpl
└── report.css
```

- `theme.go`：用 `//go:embed` 打包当前主题的模板/CSS，并实现 `New() theme.Theme`
- `report.html.tmpl`：主题 HTML 模板
- `report.css`：主题样式
- 共享字体在 `internal/report/theme/fonts/`，所有主题共用，不要在每个主题重复放 ttf；`theme.SharedFontCSS()` 统一注入 `@font-face`
- 主题 CSS 的 `font-family` 需包含 `"Shot Lepcha", "Shot Tai Tham"`；只有主题确需额外字体时，才在主题目录放自己的字体文件，并通过 `Theme.ExtraFontCSS` 追加
- 默认主题是 `moments`；`WXREPORT_THEME` 为空或未设置时使用它。当前内置主题：`moments`、`minimal`

### 新增主题

1. 新建目录 `internal/report/themes/<name>/`，放入 `theme.go`、`report.html.tmpl`、`report.css`。
2. `theme.go` 参考实现：

```go
package blue

import (
	_ "embed"
	"html/template"

	"wxreport/internal/report/theme"
)

//go:embed report.html.tmpl
var templateSource string

//go:embed report.css
var cssSource string

func New() theme.Theme {
	return theme.Theme{
		Name:     "blue",
		Title:    "蓝色",
		Width:    390,
		Template: template.Must(template.New("blue").Parse(templateSource)),
		CSS:      template.CSS(cssSource),
	}
}
```

3. 在 `internal/report/themes/themes.go` 的 `init()` 中注册并补 import：

```go
import bluetheme "wxreport/internal/report/themes/blue"

func init() {
	register(momentstheme.New())
	register(minimaltheme.New())
	register(bluetheme.New())
}
```

4. `.env` 设置 `WXREPORT_THEME=blue`；如果是内置主题，可同步更新 `.env.example` 的提示。
5. 模板可用字段：报告级 `ChatName`、`WindowStart`、`WindowEnd`、`MessageTotal`、`ParticipantTotal`、`TopicTotal`、`Topics`、`Resources`；话题级 `Rank`、`Title`、`Commentary`、`Segments`、`Participants`、`MessageCount`、`Score`、`Initial`。`Segments` 的 `Kind` 为 `text`、`person`、`resource`；资源字段为 `Index`、`Title`、`URL`。
6. 测试和预览：

```bash
go test ./...
REAL_SHOT=1 REAL_PREVIEW_THEME=blue go test ./internal/report -run TestRealShot
```

预览图输出到 `data/theme-preview-blue.png`。确认后执行 `make build` 并重启 daemon；主题是编译期注册，新增主题必须重新编译，不支持运行时从外部目录加载。

## SIWX 接口细节

- API 默认 `http://127.0.0.1:8787`，可用 `SIWX_BASE_URL` 修改
- 刷新：`POST /api/run`（`{"mode":"auto","db_dir":"..."}`），轮询 `/api/job` 到完成
- 消息接口单页最多 300 条；导出用 `before` 自动向前翻页拿到完整窗口
- SIWX 不会自动解密最新写入的消息库，所以每次执行先刷新
- SIWX 未返回解密输出目录时，使用 `SIWX_OUTPUT_ROOT`
- SQLite 通过 `modernc.org/sqlite` 纯 Go 驱动读取，不依赖系统 `sqlite3`
- 刷新失败按 fail-fast 停止本次执行，下一轮再试

## 失败处理

单次执行严格 fail-fast：任一步失败立即停止，不删除 `data/topics.json` 或已生成的报告。常驻模式下一轮 cron 仍会执行。

daemon：

- cron 为 6 段（秒 分 时 日 月 周），`cron.WithSeconds()`
- 默认每天 00/04/08/12/16/20 点
- `SkipIfStillRunning`，每轮 10 分钟超时

## 测试与构建

```bash
make build                     # 输出到项目根目录
make test                      # go test ./...
make lint                      # golangci-lint run
make check                     # lint + test
go test ./internal/pipeline    # 只测编排
```

测试覆盖：

| 包 | 覆盖点 |
|---|---|
| `internal/pipeline` | 端到端 fake、0 消息跳过、总结失败、压缩/回退 |
| `internal/siwx` | 分页、群昵称解析 |
| `internal/state` | 计划、合并、淘汰 |
| `internal/report` | 报告构建、资源去重与编号、摘要片段、参与者开关、HTML/标题 |
| `internal/summarize` | 解析重试 |
| `internal/rank` | 话轮、密集度、回退分数 |
| `internal/shot` | 无自动测试（依赖 Chrome） |
| `internal/oss` | OSS 配置、公网地址 |
| `internal/compress` | 尺寸保持、非法输入 |
| `internal/fsutil` | 原子写入、目录创建 |

## 发布

`.github/workflows/release.yml` 通过 `workflow_dispatch` 手动触发，只需填版本号（如 `v0.4.0`）：

1. `actions/setup-go` 按 `go.mod` 安装 Go
2. `golangci-lint run ./...` + `go test ./...`
3. 创建并推送 tag
4. GoReleaser 构建 darwin/windows × amd64/arm64 的 zip；包内是 `wxreport`（Windows 为 `wxreport.exe`）和 `.env.example`，版本只出现在 zip 文件名
5. Release notes 自动从提交记录生成，不排除任何提交

## 代码约定

- CLI 保持单文件；新增步骤放到 `internal/` 并写测试
- 新的外部副作用通过 `pipeline.Steps` 注入，便于测试
- 原子写入统一用 `internal/fsutil`
- 配置只从 `.env` / 环境变量读取，不加命令行覆盖参数
- 运行目录就是启动目录，`.env` 和 `data/` 都在该目录
