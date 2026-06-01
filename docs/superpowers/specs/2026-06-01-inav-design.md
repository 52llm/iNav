# inav 设计方案

> 智能收藏夹 / 导航站：浏览器插件一键收藏当前页 → 后端异步用 LLM 自动打标签 + 生成摘要 → 导航站按标签陈列、点击即走的个人门户。单二进制自部署。

- 状态：已通过需求讨论，待评审
- 日期：2026-06-01

## 1. 目标与定位

解决传统收藏夹「要自己维护分类、自己选分类，用着不方便」的问题：收藏动作做到零摩擦（点一下即可），分类整理交给 LLM 在后台自动完成，最终以一个「每天打开、按标签陈列、点击即走」的导航门户对外呈现。

- **使用对象**：单人自用 + 多设备同步；项目开源，支持自由部署。
- **不是什么**：不是笔记库（区别于 Obsidian 那种"仓库"），核心交付是可日常使用的**导航门户**，收藏插件只是入口。

### 成功标准

1. 在任意网页点一下插件 → 1 秒内提示「已收藏」（不被 LLM 延迟阻塞）。
2. 收藏后数秒内，该条目自动出现合理的标签和一句话摘要。
3. 打开导航站，能按标签快速找到并跳转到收藏的网站。
4. 自部署者拿到一个二进制 + 配置几个环境变量即可运行，无需额外中间件。

## 2. 总体架构

一个后端 + 两个前端产物：

- **`inav` 二进制（Go）**：HTTP API + 后台打标签 worker + 用 `embed.FS` 内嵌的导航站静态页。数据全部存于一个 SQLite 文件。运行方式：`./inav` 直接跑，无需 Docker。
- **浏览器插件（TypeScript, Chrome MV3，WXT 脚手架）**：抓取当前页信息并 POST 给后端。
- **导航站（TypeScript SPA，React + Vite + Tailwind）**：构建为静态文件，编译进二进制由后端托管。

### 选型理由

- **Go 后端**：单个静态二进制（`modernc.org/sqlite` 纯 Go、无 CGo），镜像/产物小、空闲内存低（~10–30MB）、毫秒级启动、交叉编译简单；用 `embed.FS` 把导航站打进二进制，部署就是「一个二进制 + 一个 `.db` 文件」。
- **TS 前端/插件**：插件与导航站都跑在浏览器里，本就用 TS；后端虽是 Go（两种语言），但后端很薄（CRUD + 调 LLM + job 轮询），代价可控。
- **SQLite + 进程内 worker**：自用规模下不需要 Postgres / Redis。异步队列用一张 `jobs` 表 + 后台轮询即可。

## 3. 数据流

### 3.1 收藏（必须秒回）

1. 用户在网页点插件（或右键菜单）。
2. 插件内容脚本抓取：`url`、`title`、`favicon`、meta 描述（`excerpt`）、正文（Readability 式提取，`content`）。
3. 插件 `POST /api/bookmarks`（带 token）。
4. 后端按 `url` 去重（已存在则更新），落库 `status=pending`，向 `jobs` 表插入一条打标任务，**立即返回 200**。
5. 插件提示「已收藏」。

### 3.2 打标签（后台异步）

6. worker 轮询 `jobs` 取到任务 → 读取**现有标签词表** + 该条目正文，组装 prompt 调 LLM。
7. LLM 返回 **标签数组 + 一句话摘要**（JSON）。
8. 后端校验 JSON → 标签名归一化匹配（复用已有标签 / 必要时新建）→ 写入 `bookmark_tags`、`summary` → `status=tagged`，`tagged_at` 置时间，job 置 `done`。
9. 失败处理：`attempts++`，按退避重试 N 次；仍失败 → 条目 `status=failed`、job `status=failed` 并记 `last_error`。导航站可见 failed 条目并提供「重试」。

### 3.3 浏览（导航站）

- 加载 → `GET /api/bookmarks?tag=&q=` → 卡片网格按标签分区展示（favicon + 标题 + 摘要）。
- 左侧标签筛选（标签列表/标签云），顶部搜索（标题 / URL / 摘要 / 标签）。
- 点击卡片 → 新标签打开原网址。

### 3.4 人工干预（CRUD 后台）

- 通过导航站后台对收藏与标签做手动调整：改标签、合并标签、重命名标签、删除收藏、重新打标（重新入队）。
- 所有变更只能走第 5 节定义的「操作层」，不允许直接执行 SQL。

## 4. 数据模型（SQLite）

```
bookmarks
  id           INTEGER PK
  url          TEXT UNIQUE NOT NULL
  title        TEXT
  favicon_url  TEXT
  excerpt      TEXT          -- meta description
  summary      TEXT          -- LLM 生成的一句话摘要
  content      TEXT          -- 提取的正文，保留以便「重新打标」时无需重新抓页面
  status       TEXT          -- pending | tagged | failed
  created_at   DATETIME
  tagged_at    DATETIME

tags
  id           INTEGER PK
  name         TEXT UNIQUE NOT NULL  -- 归一化后的规范名
  created_at   DATETIME

bookmark_tags
  bookmark_id  INTEGER FK
  tag_id       INTEGER FK
  PRIMARY KEY (bookmark_id, tag_id)

jobs
  id           INTEGER PK
  bookmark_id  INTEGER FK
  status       TEXT          -- queued | running | done | failed
  attempts     INTEGER
  last_error   TEXT
  created_at   DATETIME
  updated_at   DATETIME
```

说明：`content` 选择**保留**（不在打标后丢弃），以支持「重新打标」时无需重新抓取页面；自用规模下存储成本可接受。

## 5. 操作层（统一变更入口）

后端定义一组**确定性操作原语**，是所有数据变更的唯一入口（CRUD 后台与未来的 LLM 整理助手都复用它）：

- `rename_tag(tag, newName)`
- `merge_tags([tags], target)`
- `split_tag(...)`（v1 可不实现 UI，但接口预留）
- `add_tag(bookmarkIds, tag)` / `remove_tag(bookmarkIds, tag)`
- `retag_bookmark(bookmarkId)`（重新入队打标）
- `delete_bookmark(bookmarkId)`

设计原则：

- 任何调用方都**碰不到原始 SQL**，只能调操作原语。
- 每个操作都能返回「影响范围」用于预览（为 v1.1 的预览/确认流程铺路）。
- v1 由 CRUD 后台按钮直接映射调用；v1.1 增加「自然语言 → 操作清单 → 预览 → 确认执行」的入口，复用同一套操作层。

## 6. 标签防爆炸机制（核心价值）

LLM 自由打标签最大的风险是标签爆炸（「前端 / Frontend / frontend / web」并存）。对策：

1. **复用优先**：打标时把现有标签词表喂给 LLM，prompt 明确要求「优先复用现有标签，确实没有合适的才新建」，并归一化大小写/语言。
2. **入库归一化匹配**：后端在写库前对标签名做归一化（trim、统一大小写等）再匹配，避免重复标签。
3. **自动合并/拆分留待以后**（YAGNI）：v1 靠「复用优先 + 归一化」压住绝大多数漂移；自动合并/拆分作为 v1.1 LLM 整理助手的能力。

## 7. LLM 接入

- 仅对接一个 **OpenAI 兼容**接口，配置驱动：`INAV_LLM_BASE_URL` / `INAV_LLM_API_KEY` / `INAV_LLM_MODEL`。自部署者可填 OpenAI / 兼容网关 / DeepSeek / 本地 Ollama 等。
- 一个 OpenAI 兼容客户端实现，封装在接口后便于 mock 测试。
- API key **只在后端**（环境变量），插件与导航站永不接触。
- LLM 返回做 JSON schema 校验，格式不合规当失败处理并重试。

## 8. 鉴权（单用户）

- 配置 `INAV_TOKEN`。**写操作一律需要 Bearer token。**
- 读操作默认也需 token；可选开关 `INAV_PUBLIC_READ=true` 让导航站只读公开（写仍需 token）。
- 插件把 token 存在扩展存储；导航站登录一次后存 localStorage。
- 不做注册/多用户体系（YAGNI）；开源后若需多用户再扩展。

## 9. 配置（环境变量，12-factor）

| 变量 | 说明 |
|---|---|
| `INAV_TOKEN` | 鉴权令牌（必填） |
| `INAV_LLM_BASE_URL` | OpenAI 兼容接口地址 |
| `INAV_LLM_API_KEY` | LLM key |
| `INAV_LLM_MODEL` | 模型名 |
| `INAV_DB_PATH` | SQLite 文件路径（默认如 `./inav.db`） |
| `INAV_LISTEN_ADDR` | 监听地址（默认如 `:8080`） |
| `INAV_PUBLIC_READ` | 是否开放只读浏览（默认 false） |

## 10. 错误处理

- 收藏端点**永不阻塞在 LLM 上**，始终快速返回。
- worker 重试带退避；failed 任务在导航站可见并可手动重试。
- LLM 返回畸形 → 校验失败 → 计入重试。
- 后端不可达时插件展示错误提示，由用户重试（不做本地离线队列，YAGNI）。

## 11. v1 范围

**做**：

- 浏览器插件一键收藏（URL / 标题 / favicon / meta / 正文）。
- 后端异步 LLM 打标签 + 一句话摘要。
- 标签复用优先 + 归一化（防爆炸）。
- 导航站：卡片网格 + 标签筛选 + 搜索 + 点击跳转。
- 传统 CRUD 后台（改标签 / 合并 / 重命名 / 删除 / 重打标），构建于操作层之上。
- 单用户 token 鉴权（含可选只读公开）。
- 单二进制部署。

**不做（留待以后）**：

- LLM 自然语言整理助手（v1.1，操作层已为其预留）。
- 截图缩略图；多用户；自动合并/拆分标签；导入浏览器已有书签；Chromium 之外的浏览器；移动端。

## 12. 测试策略

- **Go**：标签归一化/匹配、job 状态机、操作层各原语、LLM 客户端（mock）、API handler；测试用内存 SQLite。
- **插件**：正文提取对样例 HTML 做单测。
- **导航站**：卡片 / 筛选 / 搜索组件轻量测试。

## 13. 默认决定（评审时可推翻）

- 导航站前端：React + Vite + Tailwind。
- 插件脚手架：WXT（专做 MV3）。
- 正文 `content` 在打标后保留，便于重打标免重抓。
