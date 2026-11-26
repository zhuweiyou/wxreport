# wxreport

读取微信群最近 24 小时的消息，调用 OpenAI 兼容接口归纳成 **TOP10 话题**，生成 Markdown 报告和手机长图。

## 运行时依赖

- **SIWX**：<https://github.com/ImUpXuu/SIWX>。负责微信 4.x 本地数据库的发现和解密，本机运行并提供 HTTP API。
  - 下载：从 [SIWX Releases](https://github.com/ImUpXuu/SIWX/releases) 获取对应平台版本：
    - Windows：`stories-in-wx-v*-windows-x64.exe`
    - macOS：`stories-in-wx-v*-macos.dmg`
  - 启动后按 Web 控制台引导完成：选择账号 → 提取密钥并解密；运行期间保持微信登录。
  - 本项目通过本机 HTTP API 访问 SIWX，默认 `http://127.0.0.1:8787`（可用 `SIWX_BASE_URL` 修改）。消息内容会发送到你在 `.env` 中配置的模型接口用于总结。
- **Chrome/Chromium**：生成长图用，未安装时长图步骤会报错（Markdown 报告仍会生成）。可用 `CHROME_PATH` 指定路径。
- **SQLite**：已内置纯 Go 驱动，无需安装 `sqlite3` 命令。

## 快速开始

1. 从 [Releases](https://github.com/zhuweiyou/wxreport/releases) 下载对应平台的 `wxreport_*.zip`，解压到独立目录；压缩包内包含 `wxreport`（Windows 为 `wxreport.exe`）和 `.env.example`。
2. 把 `.env.example` 复制为 `.env` 并编辑（macOS/Linux 下是隐藏文件，可用 `ls -a` 查看），至少填写：

```ini
SIWX_ACCOUNT=wxid_xxxxxxxxxxxx
SIWX_CHAT=12345678901@chatroom
SIWX_CHAT_NAME=群名

OPENAI_BASE_URL=https://api.openai.com/v1
OPENAI_API_KEY=sk-xxxxxxxx
OPENAI_MODEL=gpt-6
```

3. 启动 SIWX，保持微信登录，并确认解密缓存可读。
4. 在该目录运行：

```bash
./wxreport          # 常驻，按 WXREPORT_CRON 定时执行
./wxreport --once   # 立即执行一次后退出
```

5. 结果：

```
data/reports/<YYYYMMDDHHMMSS>.md   每轮 Markdown 报告
data/latest-report.png             最近一次长图
```

程序在哪个目录启动，就读取该目录的 `.env`，并把 `data/` 写在该目录。从源码运行时，复制 `.env.example` 为 `.env` 即可。

## 配置

所有配置都在**启动目录**的 `.env`，同名环境变量优先。

| 变量 | 说明 |
| --- | --- |
| `SIWX_ACCOUNT` | 微信账号 wxid，必填 |
| `SIWX_CHAT` | 群 username（`xxx@chatroom`），必填 |
| `SIWX_CHAT_NAME` | 群名，用于报告标题 |
| `OPENAI_BASE_URL` | OpenAI 兼容接口地址；留空使用 OpenAI 官方地址 |
| `OPENAI_API_KEY` | 接口密钥，必填 |
| `OPENAI_MODEL` | 模型名，必填 |
| `WXREPORT_CRON` | 定时计划，默认 `0 0 0,4,8,12,16,20 * * *`（6 段，含秒） |
| `WXREPORT_TIMEZONE` | 时区（daemon 计划与报告时间）；未配置或读取失败时使用 UTC+8 |
| `WXREPORT_UPLOAD` | 是否上传 OSS，默认 `false` |
| `WXREPORT_PARTICIPANTS` | 是否输出每话题参与者名单，默认 `true` |
| `WXREPORT_COMPRESS` | 是否压缩长图，默认 `true`；失败或压不小则保留原图 |
| `WXREPORT_TITLE` | 长图顶部标题；默认使用报告标题（即「群名」24 小时话题 TOP10） |
| `SIWX_BASE_URL` | SIWX API 地址，默认 `http://127.0.0.1:8787` |
| `SIWX_OUTPUT_ROOT` | SIWX 未返回解密输出目录时的兜底路径 |
| `CHROME_PATH` | Chrome/Chromium 路径，不设则自动查找 |
| `OSS_*` | 可选的 OSS 上传配置，见 `.env.example` |

### OSS 上传（可选）

默认只生成本地长图。配置 `.env` 里的 `OSS_BUCKET`、`OSS_ACCESS_KEY_ID`、`OSS_ACCESS_KEY_SECRET` 等，并设置 `WXREPORT_UPLOAD=true` 后，长图会覆盖上传到 `OSS_PATH`（默认 Bucket 根目录的 `latest-report.png`）。

## 行为说明

- 每轮处理当前时间往前 24 小时的滚动窗口；0 条消息则跳过本轮，不写状态、不生成报告。
- 总结是增量的：`data/topics.json` 保存上一轮活跃话题和 message_id，下一轮只处理新消息和尾部未归属消息；首次运行做一次全量 bootstrap。
- 报告保留 TOP10 话题，包含摘要、点评、参与者、资源链接。
- 长图默认做 256 色本地压缩，纯 Go、无网络、无 native 依赖；压缩失败或没有收益时使用原图。
- 任一步失败都会立即停止本轮，不删除 `data/topics.json` 或已生成的报告；常驻模式下一轮 cron 会照常执行。
- 常驻模式同一时间只运行一轮，每轮最长 10 分钟。

## 后台运行

```bash
mkdir -p logs

# macOS
nohup ./wxreport >> logs/wxreport.log 2>&1 &

# Windows (cmd)
start /b "" cmd /c ".\wxreport.exe >> logs\wxreport.log 2>&1"
```

## 开发

项目结构、内部流程、增量状态、排序模型、测试和发布流程见 [AGENTS.md](AGENTS.md)。
