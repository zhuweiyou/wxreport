---
description: 增量统计微信群最近 24 小时消息，生成 TOP10 话题总结与参与者统计
argument-hint: "[群 ID 或群名；不传时读 config/local.env 的 SIWX_CHAT]"
allowed-tools: mcp__stories-in-wx__list_sessions, Read, Write, Bash(date:*), Bash(go run ./cmd/wxreport:*)
---

# 微信群 24 小时话题 TOP10 报告（增量）

## 失败处理（最高优先级）

- 任何一步失败（时间命令、消息导出、`state-plan`、写更新文件、`state-apply`、`report`、`shot` 返回非 0）都**立即停止本次执行**，不要继续执行后续步骤，也不要输出完整报告或成功确认。
- 只输出一行：`执行失败：<步骤> - <错误摘要>`。
- 不要删除 `data/topics.json` 或已生成的报告文件。
- 失败**不影响定时任务本身**：下一次计划（4 小时后）仍会按计划重新执行。

## 固定参数

- 账号：由 `wxreport` 自动读取 `config/local.env` 的 `SIWX_ACCOUNT`（环境变量同名时优先），命令里不写 wxid。
- 群参数：`$ARGUMENTS` 优先。为空时先 Read `config/local.env`，取 `SIWX_CHAT`（群 username）和 `SIWX_CHAT_NAME`（群名）；两者都没有时停止，并提示「请在 config/local.env 配置 SIWX_CHAT，或传群 ID」。
- 如果传的是群名而不是 `xxx@chatroom`，先调用 `mcp__stories-in-wx__list_sessions` 查到对应 username 和群名。
- 群名：优先用 `SIWX_CHAT_NAME`；如果群参数不是它，必须从 `list_sessions` 取实际群名，替换下面所有 `--chat-name`。
- 报告目录：`data/reports/`
- 消息导出文件：`data/wxmsgs.json`
- 增量状态文件：`data/topics.json`
- 增量计划文件：`data/wxstate-plan.json`
- 模型更新文件：`data/wxstate-update.json`

## 执行步骤

0. 刷新 DB 缓存：在项目根目录运行 `go run ./cmd/wxreport refresh-db`，等待输出「DB 缓存已刷新」。失败则按「失败处理」停止。
1. 取时间（系统时区为北京时间）：
   - `date +%s` → NOW
   - `date -v-24H +%s` → START
   - `date -v-24H '+%Y-%m-%d %H:%M'` 和 `date '+%Y-%m-%d %H:%M'` → 可读的统计窗口
2. 导出消息：
   ```bash
   go run ./cmd/wxreport export --chat <群 username> --start START --end NOW --out data/wxmsgs.json
   ```
   - 命令输出「消息总数：N」「参与人数：M」
   - 每条消息有稳定 `mid`（跨小时不变）、`ts`、`sender`、`kind`、`text`、`link`
   - `sender` 已经是群昵称优先的安全昵称（群昵称 → 微信昵称 → wxid，绝不使用备注）
   - **若消息总数为 0：直接回复「过去 24 小时无消息」并结束；不写状态、不生成报告、不 shot**
   - 失败按「失败处理」停止
3. 生成增量计划：
   ```bash
   go run ./cmd/wxreport state-plan --export data/wxmsgs.json --state data/topics.json --out data/wxstate-plan.json --chat-name '群名' --tail-hours 6
   ```
   - 命令会输出模式（bootstrap / 增量）、上一轮话题数、新消息数、尾部未归属消息数
   - **读取 `data/wxstate-plan.json`**（一行一条消息，可分段 Read）。这是本次唯一需要阅读的消息集合，不要通读完整的 `data/wxmsgs.json`
   - 计划结构：
     - `bootstrap: true`：首次全量，`topics` 为空，`new_messages` 是当前 24 小时全部消息
     - `topics`：上一轮**所有活跃话题**（不只 TOP10），含 `id/title/commentary/summary/resources/active/expired/participants`
     - `active_messages`：对 `expired > 0` 的话题，列出它当前仍在 24h 窗口内的消息（带 `topic_id`；最多最近 120 条）
     - `new_messages`：上次窗口结束后的新消息
     - `tail_messages`：最近 6 小时内没有被归入任何话题的尾部消息
   - **关键：`tail_messages` 上一小时可能没成为热门，但可能和新消息一起组成新话题；必须和 `new_messages` 一起考虑，不要丢弃**
4. 资源提取：运行 `go run ./cmd/wxreport resources --chat <群 username>`，得到最近 24 小时内的网址/卡片资源（TSV：n、ts、title、url）。没有资源也不影响本步。
5. 增量总结：
   - **bootstrap**：为所有有意义的消息创建话题；每个话题给出 `title`、`commentary`、`summary`、`message_ids`
   - **增量**：
     - 把 `new_messages` / `tail_messages` 中属于已有话题的消息写进 `assignments`
     - 如果它们形成新话题，写进 `new_topics`
     - 有内容变化的已有话题写进 `updates`；没有变化的话题**不要**重复输出
     - 上一轮排 11-20 的话题也在 `topics` 里；有新消息时更新它，它可能重新进入 TOP10
   - **淘汰处理**：如果话题的 `expired > 0`，说明有消息已经滑出 24h。读 `active_messages` 里该 `topic_id` 的当前消息，重写摘要，去掉只属于已过期消息的内容；资源只保留仍然相关的，摘要里的 `[[res:n]]` 同步对应修订后的 resources。`active_truncated: true` 表示只给了最近 120 条，可结合旧摘要保守改写。没有 `expired` 也没有新消息的话题不要动。
   - 每条消息最多归入一个话题；纯噪音（无意义复读、纯表情）可以不归属，但不要丢掉整段讨论
   - 每个话题写 2~3 句中文摘要，**不要写「摘要：」标签**，直接写正文
   - 摘要中只把人名本身用反引号包裹，例如 `张三`；不要把整句/整段放进反引号
   - 每个话题写一句 `commentary` 点评（渲染时会变成 `>` 引用）
   - 资源写进该话题的 `resources` 数组：`[{"title":"...","url":"..."}]`；摘要中用 `[[res:1]]`、`[[res:2]]` 引用对应资源
   - `resources` 是该话题的**完整累加列表**：保留上一轮仍然有效的资源 + 本轮新增，按 URL 去重；**资源数量不限，可以超过 10 条**
   - 摘要里的 `[[res:n]]` 对应这个完整列表的序号；已有资源不要因为本轮没有再次提到就删掉
   - **不要自己写 `[^n]` 脚注，也不要写「资源列表」章节**；最终由 Go 跨话题去重、统一编号并生成（同样不限制 10 条）
   - 不要逐条复述，不要编造
6. 写 `data/wxstate-update.json`（严格 JSON，不要 Markdown 代码块、不要注释）：
   ```json
   {
     "assignments": [
       {"mid": "p:8967882821408419834", "topic_id": "t1"}
     ],
     "updates": [
       {
         "id": "t1",
         "title": "话题标题",
         "commentary": "一句话点评",
         "summary": "摘要……[[res:1]]",
         "resources": [{"title": "资源标题", "url": "https://example.com/a"}]
       }
     ],
     "new_topics": [
       {
         "temp_id": "n1",
         "title": "新话题标题",
         "commentary": "一句话点评",
         "summary": "摘要……",
         "message_ids": ["p:...", "p:..."],
         "resources": []
       }
     ],
     "drop": []
   }
   ```
   - `assignments` 的 `topic_id` 可以是已有 `tN`，也可以是 `new_topics` 里的 `temp_id`
   - `updates` 只写有变化的已有话题，字段必须完整（title/commentary/summary/resources）
   - `new_topics` 的 `temp_id` 用 `n1`、`n2`…；`message_ids` 至少一条
   - `drop` 可选，只删除确实不再有意义的话题
   - 所有 `mid` 必须来自计划的 `new_messages` / `tail_messages`，不要编造
7. 合并状态：
   ```bash
   go run ./cmd/wxreport state-apply --state data/topics.json --plan data/wxstate-plan.json --update data/wxstate-update.json --export data/wxmsgs.json
   ```
   - 会打印重复归属/无效消息等警告；退出码非 0 时立即按「失败处理」停止
8. 生成最终报告：
   ```bash
   go run ./cmd/wxreport report --state data/topics.json --export data/wxmsgs.json --out data/reports/<YYYYMMDDHHMMSS>.md --chat-name '群名'
   ```
   - 时间取统计窗口结束时间（北京时间，秒补 00）
   - Go 会按 `message_ids` 映射当前 24 小时消息，计算话轮分数、截断 TOP10、生成参与者行、资源脚注
   - 会打印每个话题的分数和「保留/截断」状态；**不要再手动改报告**
9. 生成长图（OSS 上传可选）：
   ```bash
   go run ./cmd/wxreport shot
   ```
   - 读取 `data/reports/` 下最新一篇，生成 375 宽 2x 手机长图到 `data/latest-report.png`
   - 已配置 `config/oss.env` 或 `OSS_*` 环境变量时，覆盖上传到 OSS 固定地址，输出「已上传（覆盖）https://...」
   - 未配置 OSS 时自动跳过上传，只保留本地图片，退出码仍为 0，输出「未配置 OSS，跳过上传」
   - 退出码非 0（本地图片生成本身失败）时立即按「失败处理」停止，不要继续第 10 步
10. 最后只输出一行确认：`已生成 data/reports/<YYYYMMDDHHMMSS>.md：N 条消息、M 人参与、K 个话题；图片 <第 9 步输出的 OSS 地址；未配置 OSS 时用本地 data/latest-report.png 路径>`（N/M 用第 2/3 步的导出统计，K 用第 8 步「保留 N 个」的 N）。报告内容已经在文件里，**不要**在对话里重复输出全文，也不要附加其他解释。
