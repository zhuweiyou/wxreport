# wxreport

默认每 4 小时（00:00、04:00、08:00、12:00、16:00、20:00）读取微信群最近 24 小时的消息，调用 OpenAI 兼容接口归纳成 **TOP10 话题**（摘要、点评、参与者、资源），生成 Markdown 报告到 `data/reports/`，再把最新一篇渲染成手机长图 `data/latest-report.png`；配置 OSS 且 `WXREPORT_UPLOAD=true` 时覆盖上传。

消息读取依赖开源项目 [SIWX（Stories in WX）](https://github.com/ImUpXuu/SIWX)：它负责微信 4.x 本地数据库的发现与解密，并提供本机 HTTP API。本项目不包含解密逻辑，也不会上传聊天数据。

整条流程由同一个进程完成，模型只负责「增量计划 → 状态差异」这一步：

```
刷新缓存 → 导出消息 → 生成计划 → LLM 总结 → 合并状态 → 渲染报告 → 长图
```

总结是**增量**的：`data/topics.json` 持久化上一轮所有活跃话题和稳定的 message_id；下一轮只让模型看新消息和最近几小时未被归属的尾部消息，更新受影响的话题，再由程序重新算分、排序并渲染。首次没有状态时做一次全量 bootstrap。

## 运行时依赖：SIWX

- 仓库：<https://github.com/ImUpXuu/SIWX>（AGPL-3.0，禁止商用）
- 本机需要保持 SIWX 运行、微信保持登录，且解密缓存可读

`wxreport` 通过本机 HTTP API（默认 `http://127.0.0.1:8787`，可用 `SIWX_BASE_URL` 修改）完成 DB 刷新和消息导出；导出时同时解析安全昵称。

> SIWX 仅用于处理你本人拥有合法访问权限的数据；本项目只读取，不修改微信原始数据，也不向第三方上传。首次使用 SIWX 需按它的控制台引导完成账号选择、密钥提取与数据库解密。

## 目录

```
cmd/wxreport/                  CLI：常驻定时 / --once 单次
internal/pipeline/             固定顺序编排整条流程
internal/siwx/                 DB 刷新、消息导出、昵称解析
internal/state/                增量状态：计划、差异合并、报告渲染
internal/summarize/            OpenAI 兼容接口调用与总结提示词
internal/rank/                 话轮模型：话轮、有效参与人数、文字/密集度
internal/shot/                 长图：Markdown 渲染、headless 截图、可选 OSS 上传
internal/fsutil/               原子写入
.env.example                   本机配置模板
Makefile                       构建 / 测试 / 清理
data/                          本机数据目录（已 gitignore）
logs/                          daemon 日志目录（已 gitignore，可选）
```

## 配置

所有配置都在**当前工作目录**的 `.env`（已被 `.gitignore` 忽略），支持同名环境变量覆盖：

```bash
cp .env.example .env
```

| 变量 | 用途 |
| --- | --- |
| `SIWX_ACCOUNT` | 微信账号 wxid；程序启动时读取 |
| `SIWX_CHAT` | 群 username（`xxx@chatroom`） |
| `SIWX_CHAT_NAME` | 群名，用于报告标题 |
| `OPENAI_BASE_URL` | OpenAI 兼容接口地址；留空使用 OpenAI 官方地址 |
| `OPENAI_API_KEY` | 接口密钥 |
| `OPENAI_MODEL` | 模型名 |
| `WXREPORT_TIMEZONE` | daemon 时区，默认 `Asia/Shanghai` |
| `WXREPORT_CRON` | daemon 计划，默认 `0 0 0,4,8,12,16,20 * * *` |
| `WXREPORT_UPLOAD` | 是否上传 OSS，默认 `false` |
| `WXREPORT_PARTICIPANTS` | 是否输出每话题的参与者名单，默认 `true` |
| `OSS_BUCKET` | OSS Bucket，需公共读；不配置则无法上传 |
| `OSS_REGION` / `OSS_ENDPOINT` | OSS 地域与 Endpoint，默认 `oss-cn-shanghai` |
| `OSS_ACCESS_KEY_ID` / `OSS_ACCESS_KEY_SECRET` | OSS 访问密钥 |
| `OSS_PATH` | 上传路径，默认 `latest-report.png`（Bucket 根目录） |
| `OSS_DOMAIN` | 可选的自定义域名/CDN |

其他可选环境变量：

| 变量 | 用途 |
| --- | --- |
| `SIWX_BASE_URL` | SIWX HTTP API 地址，默认 `http://127.0.0.1:8787` |
| `CHROME_PATH` | 截图使用的 Chrome/Chromium 路径；不设则自动查找 |

默认只生成本地长图；只有 `WXREPORT_UPLOAD=true` 且 `OSS_*` 配置完整时才上传。

## 数据目录

`data/` 整个目录已在 `.gitignore`，不会提交：

```
data/
  topics.json           持久状态：上一轮所有活跃话题
  wxmsgs.json           本次导出的 24 小时消息
  wxstate-plan.json     本次给模型的计划
  wxstate-update.json   模型输出的差异
  latest-report.png     最近一次长图（覆盖写）
  reports/              每次执行的 Markdown 报告
```

除 `topics.json` 外都是最近一次运行的产物，可随时删除；删掉整个 `data/` 后下一轮会重新 bootstrap。

## 使用

编译到项目根目录：

```bash
make build   # 生成 wxreport（Windows 为 wxreport.exe）
```

运行：

```bash
wxreport          # 常驻，按 WXREPORT_CRON 定时执行
wxreport --once   # 立即执行一次后退出
```

所有行为都由 `.env` 决定，命令行只区分「常驻」和「单次」。

程序在哪个目录启动，就读取该目录的 `.env`，并把 `data/` 放在该目录下。分发时把可执行文件和 `.env` 放同一目录，在该目录启动即可，不依赖 `go.mod`。

## 内部结构

各步骤都在 `internal/` 下，可单独测试：

| 包 | 职责 | 测试 |
|---|---|---|
| `internal/pipeline` | 固定顺序编排整条流程 | 端到端 fake、空消息跳过、总结失败 |
| `internal/siwx` | 刷新、导出、昵称解析 | 分页、群昵称解析 |
| `internal/state` | 增量计划、差异合并、报告渲染 | 计划、合并、渲染、参与者开关 |
| `internal/summarize` | OpenAI 兼容调用与提示词 | 解析重试 |
| `internal/rank` | 话轮模型与排序 | 话轮、密集度、回退分数 |
| `internal/shot` | Markdown 渲染、截图、OSS | 报告选择、HTML 渲染、公网地址 |
| `internal/fsutil` | 原子写入 | 原子写入与目录创建 |

```bash
make test                      # 全部测试
go test ./internal/pipeline    # 只测编排
```

## 单次执行流程

`--once` 按固定顺序执行，任一步失败立即停止：

1. 刷新 SIWX 解密缓存
2. 确定 24 小时窗口（当前时间往前 24h）
3. 导出消息到 `data/wxmsgs.json`；0 条消息则直接结束
4. 生成增量计划 `data/wxstate-plan.json`
5. 调用 OpenAI 兼容接口总结，输出 `data/wxstate-update.json`
   - 解析失败时把错误反馈给模型重试，最多 3 次
6. 合并差异，写回 `data/topics.json`
7. 渲染 `data/reports/<YYYYMMDDHHMMSS>.md`
8. 生成长图；按配置决定是否上传 OSS

## 增量状态

`data/topics.json` 持久化上一轮所有活跃话题（不只 TOP10）：

```json
{"id":"t1","title":"...","commentary":"...","summary":"...[[res:1]]","message_ids":["p:..."],"resources":[...]}
```

每轮：

1. 导出当前 24h，每条消息带稳定的 `mid`
2. 生成计划：
   - `new_messages`：上次 `window_end` 之后的新消息
   - `tail_messages`：最近 6 小时内没有被归入任何话题的尾部消息（最多 500 条）
   - `topics`：上一轮所有活跃话题的摘要与活跃/过期条数
   - `active_messages`：对 `expired > 0` 的话题，列出当前窗口内仍在的消息（每个话题最多 120 条）
3. 模型只读这份计划，输出差异 JSON：分配消息、更新有变化的话题、新建话题；对 `expired > 0` 的话题重写摘要并收缩资源
4. 合并状态、移除 24h 外的 message_id、丢掉空话题
5. 把 message_ids 映射回当前 24h 消息，算分、截断 TOP10、生成参与者行并渲染

**淘汰会同时影响排名、内容和资源列表**：排名每轮按当前 24h 消息重算；有过期消息的话题会触发摘要重写；资源列表只展示当前摘要实际引用到的资源，已淘汰内容对应的资源可能从报告里消失（状态里仍会累加保留）。

**尾部消息**用于跨轮次话题：上一轮没成为热门、甚至没被归入任何话题的消息，会和下一轮的新消息一起交给模型，因此可能在这轮才形成新话题。上一轮排 11-20 的话题也保留在状态里，有新消息时可以重新进榜。

首次没有状态时做一次全量 bootstrap，之后都是增量。滚动窗口下，指标和参与者始终是精确的 24h；摘要以增量方式维护，过期消息不会从摘要里逐句删除（可能保留一点旧内容）。

## 话题排序

话题**不按原始消息条数排序**——一个人连发几十条就能把话题刷到第一。渲染报告时，把每个话题的 `message_ids` 映射到当前 24h 消息，计算话轮指标后排序：

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
- 渲染时会生成 `参与者：昵称(消息数)、昵称(消息数)`，校正头部「消息总数/参与人数」，删除 `<!-- msgs -->` 注释和被截断话题的未引用脚注
- 排序完全确定性，同样输入重复运行结果一致

## 昵称规则

导出阶段就把 `sender_wxid` 解析成安全昵称，优先级：

1. **群昵称**：`contact.db` 的 `chat_room.ext_buffer`（protobuf 成员列表）里的群内昵称
2. **微信昵称**：`contact.nick_name`
3. **wxid**：前两者为空或全是不可见字符时回退

**`remark` 字段完全不会被读取**，报告里不可能出现备注名。导出文件里的 `sender` 就是最终报告使用的昵称，模型不再直接使用消息库里的 `sender_name`。

## 报告格式

```markdown
# 「群名」24 小时话题 TOP10

- 统计窗口：MM-DD HH:MM ~ MM-DD HH:MM
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

最终报告由渲染阶段从增量状态生成：状态里的摘要用 `[[res:n]]` 引用该话题 `resources` 数组的第 n 个资源，渲染时统一编号为 `[^n]` 并生成「资源列表」。**最终报告不含 `[[res:n]]`、`<!-- msgs -->` 等内部标记。**

长图阶段用 goldmark 把最新一篇 Markdown 渲染成自包含 HTML，再由 headless Chrome 截图；样式定义在 `internal/shot/render.go`。长图顶部使用该文件里的固定标题，不跟随 Markdown 的 `#` 标题。

格式规则：

- 每个话题：标题 → `>` 开头的一句话点评 → 摘要正文 → 参与者
- 不写「摘要：」标签
- 摘要中只把人名本身用反引号包裹，不要把整句/整段包进去；参与者行由渲染阶段生成，不加反引号，消息数用半角括号：张三(3)
- 状态里可以保留超过 10 个活跃话题；最终保留前 10；不足 10 个按实际数量；0 条消息整期跳过
- 资源用 goldmark 脚注：摘要中的 `[^1]` 在长图中渲染为上标 `[1]`，底部生成「资源列表」有序列表
- 资源按 URL 去重并跨话题累加，数量不限；状态里的 `resources` 也是累加式的，更新时不会因为本轮没提到就丢掉旧资源
- 资源列表显示 `标题 — 完整URL`；标题是纯文本，URL 为蓝色链接
- 脚注区自带的粗横线和回链箭头已隐藏
- 统计窗口时间不带年份（`MM-DD HH:MM`）
- `WXREPORT_PARTICIPANTS=false` 可省略每个话题的 `参与者：` 行

## 数据刷新

SIWX 会缓存已解密的 DB，但不会自动解密最新写入的消息库。直接查询可能只看到旧消息，因此每次执行的第一步都会刷新 DB：

- 调用 `POST /api/run`（`{"mode":"auto","db_dir":"..."}`），只刷新目标账号，并轮询 `/api/job` 直到完成
- 失败时按 fail-fast 停止本次执行，下一轮再试
- 本地消息接口单页最多返回 300 条；导出会用 `before` 自动向前翻页，拿到完整窗口

## 失败处理

单次执行严格 **fail-fast**：任一步失败立即停止，不继续后续步骤，也不删除 `data/topics.json` 或已生成的报告。

失败不会停止 daemon：下一轮 cron 仍会照常执行。

## 无人值守

```bash
wxreport
```

- cron 和时区来自 `WXREPORT_CRON` / `WXREPORT_TIMEZONE`
- cron 为 6 段（秒 分 时 日 月 周），默认每天 00/04/08/12/16/20 点执行
- 同一时间只允许一轮运行（`SkipIfStillRunning`），每轮最长 10 分钟
- 进程退出后定时停止；需要开机自启时交给系统服务管理

日志建议重定向到 `logs/`：

```bash
mkdir -p logs
wxreport >> logs/wxreport.log 2>&1
```

程序本身不安装系统服务，按平台选择一种方式守护 `wxreport`：

- macOS：launchd（`KeepAlive`）
- Linux：systemd（`Restart=always`）
- Windows：任务计划程序（开机启动）或 Windows Service

运行环境需要满足：

- 微信保持登录，SIWX 能读取本地消息库
- 已安装 Chrome/Chromium（生成长图用）
- 可选：配置 `.env` 里的 `OSS_*` 且 `WXREPORT_UPLOAD=true` 后长图覆盖上传
