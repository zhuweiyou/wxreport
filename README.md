# wxreport

每 4 小时（00:00、04:00、08:00、12:00、16:00、20:00）在本地读取微信群最近 24 小时的消息，由 Claude 归纳成 **TOP10 话题**（摘要、AI 点评、参与者、资源脚注），生成 Markdown 报告。报告写到本地 `reports/`，随后由 `wxreport shot` 生成手机长图；配置了 OSS 时覆盖上传到固定地址，未配置时只保留本地 PNG。

消息读取依赖开源项目 [SIWX（Stories in WX）](https://github.com/ImUpXuu/SIWX)：由它负责微信 4.x 本地数据库的发现、解密，以及 MCP / HTTP 接口。本项目不包含数据库解密逻辑，也不会上传聊天数据。

总结是**增量**的：`state/topics.json` 持久化上一轮所有活跃话题和稳定的 message_id；下一次只让模型看新消息 + 最近几小时未被归属的尾部消息，更新受影响的话题摘要，再由 Go 重新算分、排序并渲染报告。首次没有状态时做一次全量 bootstrap，之后每次只处理差异。

## 运行时依赖：SIWX

本项目不直接解析微信数据库，消息全部来自本机运行的开源项目 **SIWX（Stories in WX）**：

- 仓库：<https://github.com/ImUpXuu/SIWX>（AGPL-3.0，禁止商用）
- 能力：发现微信 4.x 数据目录 → 提取密钥 → 解密 SQLCipher 数据库（`message_*.db`、`contact.db`、`sns.db`）→ 提供 Web 控制台、HTTP 接口与 MCP Server
- 平台：Windows 完整支持；macOS 支持 Apple Silicon（微信 4.1.x，捕获密钥时微信需处于登录状态）；Linux 实验性
- 安装：从 [Releases](https://github.com/ImUpXuu/SIWX/releases) 下载 `stories-in-wx-*-windows-x64.exe` 或 `stories-in-wx-*-macos.dmg`；也可源码运行 `pip install -r requirements.txt && python run.py serve`

本项目和 SIWX 的接口：

| 用途 | 接口 |
| --- | --- |
| 读取会话与消息 | `.mcp.json` 以 stdio 启动 `stories-in-wx mcp`，使用 `list_sessions` / `get_messages` / `search_messages` |
| 刷新解密缓存 | `wxreport refresh-db` → `POST http://127.0.0.1:8787/api/run`，轮询 `/api/job` |

`.mcp.json` 里的可执行文件路径需按本机安装位置调整（macOS 默认 `/Applications/stories-in-wx.app/Contents/MacOS/stories-in-wx`）。

> SIWX 仅用于处理你本人拥有合法访问权限的数据；本项目只读取，不修改微信原始数据，也不向第三方上传。首次使用 SIWX 需按它的 Web 控制台引导完成账号选择、密钥提取与数据库解密。

## 目录

```
.claude/commands/wx-top10.md                 每 4 小时执行的工作流（Claude 读取）
cmd/wxreport/main.go                        本地 CLI：刷新 DB、导出消息、增量状态、报告、截图上传
internal/siwx/                              DB 刷新、消息导出、昵称映射、资源提取
internal/state/                             增量状态：计划、差异合并、报告渲染
internal/rank/                              话轮模型：话轮、有效参与人数、文字/密集度
internal/shot/                              报告长图：Markdown 渲染、headless 截图、可选 OSS 上传
Makefile                                    测试 / 清理
deploy/com.zhuweiyou.wxreport.plist     launchd 定时任务模板（安装前替换占位符）
config/local.env.example                    本机私有配置模板（账号、群 ID、群名）
config/oss.env.example                      OSS 上传配置模板（可选）
.mcp.json                                   headless Claude 的 MCP 配置
state/topics.json                           增量话题状态（本地，已 gitignore）
reports/                                    每 4 小时的 Markdown 报告（整个目录只在本机，已 gitignore；report 会自动创建）
```

## 本机私有配置

真实账号和群信息只放在本机，不进入仓库：

```bash
cp config/local.env.example config/local.env
# 编辑 config/local.env，填写 SIWX_ACCOUNT / SIWX_CHAT / SIWX_CHAT_NAME
```

| 变量 | 用途 |
| --- | --- |
| `SIWX_ACCOUNT` | 微信账号 wxid；`wxreport` 的所有命令默认读取它 |
| `SIWX_CHAT` | 默认群 username（`xxx@chatroom`）；`/wx-top10` 未传参数时使用 |
| `SIWX_CHAT_NAME` | 默认群名，用于报告标题 |

`config/local.env` 已在 `.gitignore` 中，不会被提交。同名环境变量优先于文件里的值。

## 本地命令

```bash
go run ./cmd/wxreport refresh-db          # 刷新 stories-in-wx 的 DB 解密缓存
go run ./cmd/wxreport export --chat ID --start UNIX --end UNIX --out /tmp/wxmsgs.json
                                          # 导出窗口内消息（安全昵称 + 稳定 mid）
go run ./cmd/wxreport resources --chat ID # 最近 24h 的网址/卡片资源
go run ./cmd/wxreport state-plan --export /tmp/wxmsgs.json --state state/topics.json --out /tmp/wxstate-plan.json
                                          # 计算上一轮话题 + 新消息 + 尾部未归属消息
go run ./cmd/wxreport state-apply --state state/topics.json --plan /tmp/wxstate-plan.json --update /tmp/wxstate-update.json --export /tmp/wxmsgs.json
                                          # 合并模型输出的增量差异
go run ./cmd/wxreport report --state state/topics.json --export /tmp/wxmsgs.json --out reports/x.md
                                          # 算分、截断 TOP10、渲染最终报告
go run ./cmd/wxreport shot                # 生成长图；配置了 OSS 则上传，未配置自动跳过（--no-upload 强制只生成本地图）

# 兼容/手动：旧的全量草稿排序
go run ./cmd/wxreport rank --messages /tmp/wxmsgs.json reports/x.md
go run ./cmd/wxreport rank --dry-run reports/x.md

go run ./cmd/wxreport names --chat ID     # 调试用：sender -> 昵称映射（排除备注）

make test                                 # 跑全部测试
```

## OSS 上传（可选）

上传是自用分发步骤，不是必需项：

- 不配置 `config/oss.env`（也没有 `OSS_*` 环境变量）时，`wxreport shot` 只生成本地 `runs/latest.png`，跳过上传，退出码仍为 0
- 需要上传时：`cp config/oss.env.example config/oss.env`，填写 Bucket（公共读）与 AccessKey
- 只想要 Markdown 报告：不执行 `shot` 即可；只想要本地图片：不配置 OSS，或给 `shot` 加 `--no-upload`

## 数据刷新

`stories-in-wx` 会缓存已解密的 DB，但**不会自动解密最新写入的消息库**。如果直接查询，可能只看到旧消息。因此：

- 每次的 `wx-top10` 工作流第 0 步会先执行 `go run ./cmd/wxreport refresh-db`
- 该命令调用本地 `http://127.0.0.1:8787/api/run`（`{"mode":"auto","db_dir":"..."}`），只刷新目标账号，并轮询 `/api/job` 直到完成
- 应用自带的“自动刷新”会遍历所有账号，并对未登录账号尝试 LLDB 提取，单次约 70 秒；因此当前保持关闭，改用按需的 `refresh-db`
- 如果刷新失败，按 fail-fast 停止本次执行；下一次计划（4 小时后）会重试
- 本地消息接口单页最多返回 300 条；`export`、`resources`、`names` 都会用 `before` 自动向前翻页，不会只拿到最新 300 条

## 昵称规则

`wxreport export` 在导出消息时就把 `sender_wxid` 解析成安全昵称，优先级是：

1. **群昵称**：`contact.db` 的 `chat_room.ext_buffer`（protobuf 成员列表）里的群内昵称
2. **微信昵称**：`contact.nick_name`
3. **wxid**：前两者为空或全是不可见字符时回退

**`remark` 字段完全不会被读取**，报告里不可能出现备注名。导出文件里的 `sender` 就是最终报告使用的昵称，Claude 不再直接使用 MCP 或消息库里的 `sender_name`（那可能是备注）。

`wxreport names --chat ...` 保留为调试命令，输出 `preferred / wxid` 映射，规则相同。

## 失败处理

单次执行严格 **fail-fast**：时间获取、消息导出、`state-plan`、写更新文件、`state-apply`、`report`、`shot` 任一步失败，立即停止本次执行，不继续后续步骤；不会删除 `state/topics.json` 或已生成的报告。

失败**不会停止定时任务**：launchd 下一次计划仍会照常执行。日志在 `logs/launchd.out.log` 与 `logs/launchd.err.log`。

## 无人值守（launchd）

`deploy/com.zhuweiyou.wxreport.plist` 是模板，安装前替换这些占位符，再复制到 `~/Library/LaunchAgents/`：

| 占位符 | 替换为 |
| --- | --- |
| `__HOME__` | 用户主目录，如 `/Users/alice` |
| `__PROJECT_DIR__` | 本项目绝对路径 |
| `__CHAT_ID__` | 群 username，如 `12345678901@chatroom` |
| `__CLAUDE_BIN__` | `which claude` 的绝对路径 |
| `__PATH__` | 启动时的 PATH，至少包含 claude、go、node 所在目录 |

label 与文件名使用 `com.zhuweiyou.wxreport`。如果之前安装过旧版本，先 bootout 旧的 label 并删除旧 plist，再安装这一份。每天 **00:00、04:00、08:00、12:00、16:00、20:00** 各执行一次。关闭终端不会停止；需要满足：

- macOS 保持开机、用户保持登录（关机/注销会停）
- `magpie.app` 保持运行（`127.0.0.1:3425` 模型代理，launchd 托管）
- 微信保持登录，`stories-in-wx` 能读取本地消息库
- 已安装 Chrome/Chromium（`wxreport shot` 截图用）
- 可选：配置 `config/oss.env` 后长图会覆盖上传 OSS；不配置则只生成本地 `runs/latest.png`

手动触发一次：

```bash
launchctl kickstart -k "gui/$(id -u)/com.zhuweiyou.wxreport"
```

查看状态 / 卸载 / 重新安装：

```bash
launchctl print "gui/$(id -u)/com.zhuweiyou.wxreport"
launchctl bootout "gui/$(id -u)/com.zhuweiyou.wxreport"
cp deploy/com.zhuweiyou.wxreport.plist ~/Library/LaunchAgents/
launchctl bootstrap "gui/$(id -u)" ~/Library/LaunchAgents/com.zhuweiyou.wxreport.plist
```

headless 调用依赖 `~/.claude/settings.json` 中的 `ANTHROPIC_BASE_URL` / `ANTHROPIC_AUTH_TOKEN` 以及 `magpie` 代理。

## 报告格式

```markdown
# 「群名」24 小时话题 TOP10

- 统计窗口：YYYY-MM-DD HH:MM ~ YYYY-MM-DD HH:MM
- 消息总数：N
- 参与人数：M

## 1. 话题标题

> 一句话总结……

正文……`张三` 提到……[^1]

参与者：张三(3)、李四(1)

## 2. 话题标题

> 一句话总结……

正文……

参与者：王五(2)

## 资源列表

[^1]: 资源标题 — [https://example.com/a](https://example.com/a)
[^2]: 另一个资源 — [https://example.com/b](https://example.com/b)
```

最终报告由 `wxreport report` 从增量状态渲染：状态里的摘要用 `[[res:n]]` 引用该话题 `resources` 数组的第 n 个资源，渲染时统一编号为 `[^n]` 并生成「资源列表」。**最终报告不含 `[[res:n]]`、`<!-- msgs -->` 等内部标记。**

页面地址来自报告时间戳：`reports/20261123213900.md` → `/reports/20261123213900`。Markdown 正文在请求时由 goldmark 渲染，模板与样式改动不需要重新生成任何 HTML 文件。

格式规则：

- 每个话题：标题 → `>` 开头的一句话点评 → 摘要正文 → 参与者
- 不写「摘要：」标签
- 摘要中只把人名本身用反引号包裹，不要把整句/整段包进去；参与者行由 `report`/`rank` 生成，不加反引号，消息数用半角括号：张三(3)
- 状态里可以保留超过 10 个活跃话题；最终由 `wxreport report` 算分并保留前 10；不足 10 个按实际数量；0 条消息整期跳过
- 资源用 goldmark 脚注：摘要中的 `[^1]` 渲染为可点击上标 `[1]`，跳转到页面底部「资源列表」有序列表
- 资源按 URL 去重并跨话题累加，数量不限，可以超过 10 条；状态里的话题 `resources` 也是累加式的，更新时不会因为本轮没提到就丢掉旧资源
- 资源列表始终显示 `标题 — 完整URL`；标题是纯文本、不可点击、显示黑色；URL 是蓝色链接，数量不限，外部链接新窗口打开；普通浏览和分享图一致
- 脚注区自带的粗横线和回链箭头已隐藏，只保留「资源列表」标题下的一根细线
- 报告页右上角有「分享」按钮：用 modern-screenshot 生成 PNG 并自动下载；分享图只隐藏「返回目录」，资源列表与普通浏览完全一致；modern-screenshot 加载失败时回退到打印
- 所有站内链接使用 `/` 绝对路径；404 页的「返回目录」指向 `/`

## 增量总结

`state/topics.json` 持久化上一轮所有活跃话题（不只 TOP10）：

```json
{"id":"t1","title":"...","commentary":"...","summary":"...[[res:1]]","message_ids":["p:..."],"resources":[...]}
```

每 4 小时：

1. `wxreport export` 导出当前 24h（用于算指标），每条消息带稳定的 `mid`
2. `wxreport state-plan` 计算：
   - `new_messages`：上次 `window_end` 之后的新消息
   - `tail_messages`：最近 6 小时内**没有被归入任何话题**的尾部消息（最多 500 条）
   - `topics`：上一轮所有活跃话题的摘要、活跃/过期条数
3. Claude 只读这份计划，输出差异 JSON：
   - 把新消息/尾部消息分配到已有话题，或新建话题
   - 只更新有变化的话题摘要
   - 对 `expired > 0` 的话题，用 `active_messages`（当前窗口内仍在的消息）重写摘要、收缩资源，去掉已经淘汰的内容
4. `wxreport state-apply` 合并状态、移除 24h 外的 message_id、丢掉空话题
5. `wxreport report` 把 message_ids 映射回当前 24h 消息，用话轮模型算分、截断 TOP10、生成参与者行并渲染报告

因此**淘汰会同时影响排名、内容和资源列表**：排名每轮都按当前 24h 消息重算；有过期消息的话题会触发摘要重写；资源列表只展示当前摘要实际引用到的资源，已经淘汰内容对应的资源可能从报告里消失（状态里仍会累加保留）。

**尾部消息**是专门为跨轮次话题准备的：上一轮没成为热门、甚至没被归入任何话题的消息，会和下一轮的新消息一起交给模型，因此可以在这轮才形成新话题。上一轮排 11-20 的话题也会保留在状态里，有新消息时可以重新进榜。

首次没有状态时做一次全量 bootstrap，之后都是增量。滚动窗口下，指标和参与者始终是精确的 24h；摘要以增量方式维护，过期消息不会从摘要里逐句删除（可能保留一点旧内容）。

## 话题排序

话题**不按原始消息条数排序**——一个人连发几十条就能把话题刷到第一。增量链路：

1. 状态里的每个话题保存稳定的 `message_ids`
2. `wxreport report` 把这些 id 映射到当前 24h 消息，计算话轮指标
3. 按下面的分数排序，保留 TOP10

话轮模型：

```text
话轮：同一人在同一话题内连续消息算一个话轮；同一人两条消息间隔 >20 分钟、
      或中间切到别的话题，则拆开；换人立即新话轮
N_eff = 1 / Σ(pᵢ²)          pᵢ = 第 i 个人在该话题的话轮占比
θ     = 带文字话轮 / 总话轮
μ     = 密集话轮 / 总话轮    相邻话轮间隔 ≤5 分钟算密集
分数  = 0.45 × (N_eff / ΣN_eff) + 0.40 × θ × (话轮数 / Σ话轮数) + 0.15 × μ
```

- 连发几十条只算 1～2 个话轮，刷屏不会再霸榜
- 有效参与人数看「有多少人真正在交替发言」，一人占 80% 话轮会把 N_eff 拉低
- 话轮多、多人参与、讨论密集的话题排前面；长文但不密集的话题不会碾压小讨论
- `report`/`rank` 会根据消息明细生成 `参与者：昵称(消息数)、昵称(消息数)`、校正头部「消息总数/参与人数」、删除 `<!-- msgs -->` 注释和被截断话题的未引用脚注
- 重复引用、无效消息编号会打印警告；排序完全确定性，同样输入重复运行结果一致
- 手动 `rank` 没有 `--messages` 时退回粗略分数 `N_eff × log2(2 + 消息数)`，只用于兼容历史报告或预览
- `rank --dry-run` 只打印分数和保留/截断状态，不写回文件
