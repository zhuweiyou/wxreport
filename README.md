# wxreport

每 4 小时（00:00、04:00、08:00、12:00、16:00、20:00）读取微信群最近 24 小时的消息，由 Claude 归纳成 **TOP10 话题**（摘要、点评、参与者、资源），生成 Markdown 报告到 `data/reports/`；`wxreport shot` 把最新一篇渲染成手机长图 `data/latest-report.png`，配置 OSS 时覆盖上传。

消息读取依赖开源项目 [SIWX（Stories in WX）](https://github.com/ImUpXuu/SIWX)：它负责微信 4.x 本地数据库的发现、解密，并提供 HTTP API 与 MCP Server。本项目不包含解密逻辑，也不会上传聊天数据。

总结是**增量**的：`data/topics.json` 持久化上一轮所有活跃话题和稳定的 message_id；下一轮只让模型看新消息和最近几小时未被归属的尾部消息，更新受影响的话题，再由 Go 重新算分、排序并渲染。首次没有状态时做一次全量 bootstrap。

## 运行时依赖：SIWX

- 仓库：<https://github.com/ImUpXuu/SIWX>（AGPL-3.0，禁止商用）
- 本机需要保持 SIWX 运行、微信保持登录，且解密缓存可读

| 用途 | 接口 |
| --- | --- |
| 查询群列表（工作流传群名时） | `.mcp.json` 以 stdio 启动 `stories-in-wx mcp`，调用 `list_sessions` |
| 刷新解密缓存、导出消息、资源、昵称 | 本机 HTTP API（默认 `http://127.0.0.1:8787`，可用 `SIWX_BASE_URL` 修改） |

`.mcp.json` 里的可执行文件路径需按本机安装位置调整（macOS 默认 `/Applications/stories-in-wx.app/Contents/MacOS/stories-in-wx`）。

> SIWX 仅用于处理你本人拥有合法访问权限的数据；本项目只读取，不修改微信原始数据，也不向第三方上传。首次使用 SIWX 需按它的控制台引导完成账号选择、密钥提取与数据库解密。

## 目录

```
.claude/commands/wx-top10.md            每 4 小时执行的工作流
.claude/settings.json                   交互式会话的权限配置
cmd/wxreport/                           本地 CLI：cobra 命令 + 各子命令一个文件
internal/siwx/                          DB 刷新、消息导出、昵称映射、资源提取
internal/fsutil/                        原子写入工具（临时文件 + rename）
internal/state/                         增量状态：计划、差异合并、报告渲染
internal/rank/                          话轮模型：话轮、有效参与人数、文字/密集度
internal/shot/                          长图：Markdown 渲染、headless 截图、可选 OSS 上传
config/local.env.example                本机私有配置模板（账号、群 ID、群名）
config/oss.env.example                  OSS 上传配置模板（可选）
deploy/com.zhuweiyou.wxreport.plist     launchd 定时任务模板（安装前替换占位符）
.mcp.json                               headless Claude 的 MCP 配置
Makefile                                测试 / 清理
data/                                   本机数据目录（已 gitignore）
logs/                                   launchd 日志（已 gitignore，安装前创建）
```

## 配置

### config/local.env

```bash
cp config/local.env.example config/local.env
```

| 变量 | 用途 |
| --- | --- |
| `SIWX_ACCOUNT` | 微信账号 wxid；所有命令默认读取它 |
| `SIWX_CHAT` | 默认群 username（`xxx@chatroom`）；工作流未传参数时使用 |
| `SIWX_CHAT_NAME` | 默认群名，用于报告标题 |

同名环境变量优先于文件。可选环境变量：

| 变量 | 用途 |
| --- | --- |
| `SIWX_BASE_URL` | SIWX HTTP API 地址，默认 `http://127.0.0.1:8787` |
| `CHROME_PATH` | `shot` 使用的 Chrome/Chromium 路径；不设则自动查找 |

### config/oss.env（可选）

```bash
cp config/oss.env.example config/oss.env
```

| 变量 | 用途 |
| --- | --- |
| `OSS_BUCKET` | Bucket 名称，需公共读 |
| `OSS_REGION` / `OSS_ENDPOINT` | 地域与 Endpoint，默认 `oss-cn-shanghai` |
| `OSS_ACCESS_KEY_ID` / `OSS_ACCESS_KEY_SECRET` | 访问密钥 |
| `OSS_PATH` | 上传路径，默认 `latest-report.png`（Bucket 根目录） |
| `OSS_DOMAIN` | 可选的自定义域名/CDN |

不配置 OSS 时 `shot` 只生成本地图，跳过上传，退出码仍为 0。

## 数据目录

`data/` 整个目录已在 `.gitignore`，不会提交：

```
data/
  topics.json           持久状态：上一轮所有活跃话题
  wxmsgs.json           本次导出的 24 小时消息
  wxstate-plan.json     本次给模型的计划
  wxstate-update.json   模型输出的差异
  latest-report.png     最近一次长图（覆盖写）
  reports/              每 4 小时的 Markdown 报告
```

除 `topics.json` 外都是最近一次运行的产物，可随时删除；删掉整个 `data/` 后下一轮会重新 bootstrap。

## 命令

```bash
go run ./cmd/wxreport help
# 查看全部命令与参数

go run ./cmd/wxreport refresh-db
# 刷新 SIWX 的 DB 解密缓存

go run ./cmd/wxreport export --chat ID --start UNIX --end UNIX [--out data/wxmsgs.json]
# 导出窗口内消息（安全昵称 + 稳定 mid）

go run ./cmd/wxreport resources --chat ID
# 最近 24h 的网址/卡片资源（TSV：n、ts、title、url）

go run ./cmd/wxreport state-plan [--export data/wxmsgs.json] [--state data/topics.json] [--out data/wxstate-plan.json] [--tail-hours 6]
# 计算上一轮话题 + 新消息 + 尾部未归属消息

go run ./cmd/wxreport state-apply [--state data/topics.json] [--plan data/wxstate-plan.json] [--update data/wxstate-update.json] [--export data/wxmsgs.json]
# 合并模型输出的增量差异

go run ./cmd/wxreport report --out data/reports/x.md [--state data/topics.json] [--export data/wxmsgs.json]
# 算分、截断 TOP10、渲染最终报告

go run ./cmd/wxreport shot [--no-upload] [--oss-path latest-report.png] [--out data/latest-report.png]
# 生成长图；配置了 OSS 则覆盖上传

go run ./cmd/wxreport rank --messages data/wxmsgs.json data/reports/x.md
go run ./cmd/wxreport rank --dry-run data/reports/x.md
# 手动按话轮模型重排已有报告

go run ./cmd/wxreport names --chat ID
# 调试：sender -> 昵称映射（排除备注）

make test
# 跑全部测试
```

## 每 4 小时的工作流

`.claude/commands/wx-top10.md` 定义了完整步骤，由 launchd 调用的 headless Claude 执行：

0. `refresh-db` 刷新解密缓存
1. 取当前时间，确定 24 小时窗口
2. `export` 导出消息到 `data/wxmsgs.json`；0 条消息则直接结束
3. `state-plan` 生成 `data/wxstate-plan.json`
4. `resources` 提取最近 24h 的网址/卡片
5. Claude 只读计划，归纳话题，写 `data/wxstate-update.json`
6. `state-apply` 合并差异，写回 `data/topics.json`
7. `report` 渲染 `data/reports/<YYYYMMDDHHMMSS>.md`
8. `shot` 生成长图，可选上传 OSS

任一步失败立即停止本次执行，详见「失败处理」。

## 增量状态

`data/topics.json` 持久化上一轮所有活跃话题（不只 TOP10）：

```json
{"id":"t1","title":"...","commentary":"...","summary":"...[[res:1]]","message_ids":["p:..."],"resources":[...]}
```

每轮：

1. `export` 导出当前 24h，每条消息带稳定的 `mid`
2. `state-plan` 计算：
   - `new_messages`：上次 `window_end` 之后的新消息
   - `tail_messages`：最近 6 小时内没有被归入任何话题的尾部消息（最多 500 条）
   - `topics`：上一轮所有活跃话题的摘要与活跃/过期条数
   - `active_messages`：对 `expired > 0` 的话题，列出当前窗口内仍在的消息（每个话题最多 120 条）
3. Claude 只读这份计划，输出差异 JSON：分配消息、更新有变化的话题、新建话题；对 `expired > 0` 的话题重写摘要并收缩资源
4. `state-apply` 合并状态、移除 24h 外的 message_id、丢掉空话题
5. `report` 把 message_ids 映射回当前 24h 消息，算分、截断 TOP10、生成参与者行并渲染

**淘汰会同时影响排名、内容和资源列表**：排名每轮按当前 24h 消息重算；有过期消息的话题会触发摘要重写；资源列表只展示当前摘要实际引用到的资源，已淘汰内容对应的资源可能从报告里消失（状态里仍会累加保留）。

**尾部消息**用于跨轮次话题：上一轮没成为热门、甚至没被归入任何话题的消息，会和下一轮的新消息一起交给模型，因此可能在这轮才形成新话题。上一轮排 11-20 的话题也保留在状态里，有新消息时可以重新进榜。

首次没有状态时做一次全量 bootstrap，之后都是增量。滚动窗口下，指标和参与者始终是精确的 24h；摘要以增量方式维护，过期消息不会从摘要里逐句删除（可能保留一点旧内容）。

## 话题排序

话题**不按原始消息条数排序**——一个人连发几十条就能把话题刷到第一。`report` 把每个话题的 `message_ids` 映射到当前 24h 消息，计算话轮指标后排序：

```text
话轮：同一人在同一话题内连续消息算一个话轮；同一人两条消息间隔 >20 分钟、
      或中间切到别的话题，则拆开；换人立即新话轮
N_eff = 1 / Σ(pᵢ²)          pᵢ = 第 i 个人在该话题的话轮占比
θ     = 带文字话轮 / 总话轮
μ     = 密集话轮 / 总话轮    相邻话轮间隔 ≤5 分钟算密集
分数  = 0.45 × (N_eff / ΣN_eff) + 0.40 × θ × (话轮数 / Σ话轮数) + 0.15 × μ
```

- 连发几十条只算 1～2 个话轮，刷屏不会霸榜
- 有效参与人数看「有多少人真正在交替发言」，一人占 80% 话轮会把 N_eff 拉低
- 话轮多、多人参与、讨论密集的话题排前面
- `report` / `rank` 会根据消息明细生成 `参与者：昵称(消息数)、昵称(消息数)`，校正头部「消息总数/参与人数」，删除 `<!-- msgs -->` 注释和被截断话题的未引用脚注
- 排序完全确定性，同样输入重复运行结果一致
- 手动 `rank` 没有 `--messages` 时使用粗略分数 `N_eff × log2(2 + 消息数)`，只用于预览

## 昵称规则

`wxreport export` 在导出消息时就把 `sender_wxid` 解析成安全昵称，优先级：

1. **群昵称**：`contact.db` 的 `chat_room.ext_buffer`（protobuf 成员列表）里的群内昵称
2. **微信昵称**：`contact.nick_name`
3. **wxid**：前两者为空或全是不可见字符时回退

**`remark` 字段完全不会被读取**，报告里不可能出现备注名。导出文件里的 `sender` 就是最终报告使用的昵称，Claude 不再直接使用 MCP 或消息库里的 `sender_name`。

`wxreport names --chat ...` 输出 `preferred / wxid` 映射，规则相同。

## 报告格式

```markdown
# 「群名」24 小时话题 TOP10

- 统计窗口：YYYY-MM-DD HH:MM ~ YYYY-MM-DD HH:MM
- 消息总数：N
- 参与人数：M

## 1. 话题标题

> 一句话点评……

正文……`张三` 提到……[^1]

参与者：张三(3)、李四(1)

## 2. 话题标题

> 一句话点评……

正文……

参与者：王五(2)

## 资源列表

[^1]: 资源标题 — [https://example.com/a](https://example.com/a)
[^2]: 另一个资源 — [https://example.com/b](https://example.com/b)
```

最终报告由 `wxreport report` 从增量状态渲染：状态里的摘要用 `[[res:n]]` 引用该话题 `resources` 数组的第 n 个资源，渲染时统一编号为 `[^n]` 并生成「资源列表」。**最终报告不含 `[[res:n]]`、`<!-- msgs -->` 等内部标记。**

`wxreport shot` 用 goldmark 把最新一篇 Markdown 渲染成自包含 HTML，再由 headless Chrome 截成长图；样式定义在 `internal/shot/render.go`。长图顶部使用该文件里的固定标题，不跟随 Markdown 的 `#` 标题。

格式规则：

- 每个话题：标题 → `>` 开头的一句话点评 → 摘要正文 → 参与者
- 不写「摘要：」标签
- 摘要中只把人名本身用反引号包裹，不要把整句/整段包进去；参与者行由 `report`/`rank` 生成，不加反引号，消息数用半角括号：张三(3)
- 状态里可以保留超过 10 个活跃话题；最终由 `report` 算分并保留前 10；不足 10 个按实际数量；0 条消息整期跳过
- 资源用 goldmark 脚注：摘要中的 `[^1]` 在长图中渲染为上标 `[1]`，底部生成「资源列表」有序列表
- 资源按 URL 去重并跨话题累加，数量不限；状态里的 `resources` 也是累加式的，更新时不会因为本轮没提到就丢掉旧资源
- 资源列表显示 `标题 — 完整URL`；标题是纯文本，URL 为蓝色链接
- 脚注区自带的粗横线和回链箭头已隐藏

## 数据刷新

SIWX 会缓存已解密的 DB，但不会自动解密最新写入的消息库。直接查询可能只看到旧消息，因此工作流第 0 步会执行 `refresh-db`：

- 调用 `POST /api/run`（`{"mode":"auto","db_dir":"..."}`），只刷新目标账号，并轮询 `/api/job` 直到完成
- 失败时按 fail-fast 停止本次执行，下一轮再试
- 本地消息接口单页最多返回 300 条；`export`、`resources`、`names` 都会用 `before` 自动向前翻页

## 失败处理

单次执行严格 **fail-fast**：任一步返回非 0 立即停止，不继续后续步骤，也不删除 `data/topics.json` 或已生成的报告。

失败不会停止定时任务：launchd 下一次计划仍会照常执行。日志在 `logs/launchd.out.log` 与 `logs/launchd.err.log`。

## 无人值守（launchd）

`deploy/com.zhuweiyou.wxreport.plist` 是模板，安装前替换占位符：

| 占位符 | 替换为 |
| --- | --- |
| `__HOME__` | 用户主目录，如 `/Users/alice` |
| `__PROJECT_DIR__` | 本项目绝对路径 |
| `__CHAT_ID__` | 群 username，如 `12345678901@chatroom` |
| `__CLAUDE_BIN__` | `which claude` 的绝对路径 |
| `__PATH__` | 启动时的 PATH，至少包含 claude、go、node 所在目录 |

label 与文件名使用 `com.zhuweiyou.wxreport`，每天 **00:00、04:00、08:00、12:00、16:00、20:00** 各执行一次。需要满足：

- macOS 保持开机、用户保持登录
- `magpie.app` 保持运行（`127.0.0.1:3425` 模型代理）
- 微信保持登录，SIWX 能读取本地消息库
- 已安装 Chrome/Chromium（`shot` 截图用）
- 可选：配置 `config/oss.env` 后长图覆盖上传 OSS

安装 / 重新安装：

```bash
launchctl bootout "gui/$(id -u)/com.zhuweiyou.wxreport" 2>/dev/null || true
mkdir -p logs
# 先按下表替换 deploy 模板里的占位符，再复制到 LaunchAgents
cp deploy/com.zhuweiyou.wxreport.plist ~/Library/LaunchAgents/
launchctl bootstrap "gui/$(id -u)" ~/Library/LaunchAgents/com.zhuweiyou.wxreport.plist
```

手动触发、查看状态、卸载：

```bash
launchctl kickstart -k "gui/$(id -u)/com.zhuweiyou.wxreport"
launchctl print "gui/$(id -u)/com.zhuweiyou.wxreport"
launchctl bootout "gui/$(id -u)/com.zhuweiyou.wxreport"
```

headless 调用依赖 `~/.claude/settings.json` 中的 `ANTHROPIC_BASE_URL` / `ANTHROPIC_AUTH_TOKEN` 以及 `magpie` 代理。

无人值守时 Claude 的工具白名单由 plist 的 `--allowedTools` 决定，不依赖 `~/.claude.json` 里的 workspace 信任标记。首次在新目录运行时，`logs/launchd.err.log` 可能出现「workspace has not been trusted」并忽略 `.claude/settings.json` 的权限，属正常现象，不影响工作流执行。
