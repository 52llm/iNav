# iNav

智能收藏夹 / 导航站：浏览器插件一键收藏当前页 → 后端用大模型**自动打标签 + 生成摘要** → 导航站按标签陈列、点击即走的个人门户。

单个 Go 二进制自部署，数据全在一个 SQLite 文件。

## 它解决什么

传统收藏夹要自己建分类、自己选分类，越用越乱。iNav 把「收藏」做到零摩擦（点一下即可，秒回），分类整理交给大模型在后台异步完成：

- **多标签**而非单一目录树——一个页面可同时属于多个主题。
- **标签复用优先 + 归一化**，抑制「前端 / Frontend / frontend」式的标签爆炸。
- 最终呈现为一个**每天打开、按标签筛选、搜索、点击跳转**的导航门户，而不是一个需要翻找的仓库。

## 架构

一个后端 + 两个前端，全部由同一个二进制托管：

| 组件 | 技术 | 说明 |
|---|---|---|
| 后端 | Go + `net/http` + SQLite（`modernc.org/sqlite`，纯 Go 无 CGo） | API + 异步打标签 worker + 内嵌导航站静态页 |
| 导航站 | React + Vite + Tailwind | 构建为静态文件，用 `embed.FS` 编译进二进制 |
| 浏览器插件 | 原生 Chrome MV3（无构建） | 抓当前页正文/favicon，POST 到后端 |

大模型只通过一个 **OpenAI 兼容**接口调用，API key 仅存于后端环境变量，插件与导航站永不接触。

## 快速开始

需要 Go 1.22+。前端构建产物已随仓库提交，所以**编译二进制不需要 Node**。

```bash
go build -o inav .

INAV_TOKEN=your-secret \
INAV_LLM_BASE_URL=https://api.openai.com/v1 \
INAV_LLM_API_KEY=sk-... \
INAV_LLM_MODEL=gpt-4o-mini \
./inav
```

打开 `http://localhost:8080`，输入上面的 `INAV_TOKEN` 登录。

> 不配 `INAV_LLM_*` 也能跑：收藏会正常保存，只是停在 `pending` 不会自动打标签。可填任意 OpenAI 兼容端点（OpenAI / 兼容网关 / DeepSeek / 本地 Ollama 等）。

### 安装浏览器插件

1. Chrome 打开 `chrome://extensions`，开启「开发者模式」。
2. 「加载已解压的扩展程序」，选择本仓库的 `extension/` 目录。
3. 点插件图标 →「设置」→ 填后端地址（如 `http://localhost:8080`）和 `INAV_TOKEN` → 保存。
4. 在任意网页点「收藏此页」即可。详见 [`extension/README.md`](extension/README.md)。

## 配置（环境变量）

| 变量 | 必填 | 默认 | 说明 |
|---|---|---|---|
| `INAV_TOKEN` | ✅ | — | 鉴权令牌。写操作必须携带 `Authorization: Bearer <token>` |
| `INAV_DB_PATH` | | `inav.db` | SQLite 文件路径 |
| `INAV_LISTEN_ADDR` | | `:8080` | 监听地址 |
| `INAV_PUBLIC_READ` | | `false` | 设为 `true` 时导航站只读浏览免 token（写操作仍需 token） |
| `INAV_LLM_BASE_URL` | | — | OpenAI 兼容接口地址（如 `https://api.openai.com/v1`） |
| `INAV_LLM_API_KEY` | | — | 模型 API key |
| `INAV_LLM_MODEL` | | — | 模型名 |

## API

所有 `/api/*` 写操作需 `Authorization: Bearer <INAV_TOKEN>`；读操作在 `INAV_PUBLIC_READ=true` 时可匿名。

| 方法 | 路径 | 说明 |
|---|---|---|
| `POST` | `/api/bookmarks` | 收藏页面 `{url,title,faviconUrl,excerpt,content}`，立即返回并异步打标签 |
| `GET` | `/api/bookmarks?tag=&q=` | 列出收藏，按标签筛选 / 关键词搜索 |
| `GET` | `/api/tags` | 列出所有标签 |
| `PATCH` | `/api/bookmarks/{id}/tags` | 增删标签 `{add[],remove[]}` |
| `POST` | `/api/bookmarks/{id}/retag` | 重新打标签 |
| `DELETE` | `/api/bookmarks/{id}` | 删除收藏 |
| `POST` | `/api/tags/rename` | 重命名标签 `{oldName,newName}` |
| `POST` | `/api/tags/merge` | 合并标签 `{sources[],target}` |

## 开发

改后端：

```bash
go test ./...        # 运行后端测试
go run .             # 本地运行
```

改前端（导航站，需要 Node 20.19+ / 22+，Vite 7 要求）：

```bash
cd web
npm install
npm run dev          # Vite 开发服务器（需后端在 :8080）
npm run test         # 前端单测（vitest）
```

构建发布产物（重新打包前端并嵌入二进制）：

```bash
make build           # = cd web && npm install && npm run build，然后 go build -o inav .
make test            # 后端 + 前端全部测试
```

> 前端构建产物 `internal/web/dist/` 随仓库提交，因此普通用户 `go build` / `go install` 即可得到带 UI 的二进制，无需 Node；只有修改前端的人才需要 Node 重新 `make build`。

## 部署

- **裸二进制**：`go build` 出 `inav`，配好环境变量后用 systemd / pm2 长驻；需要 HTTPS 可在前面挂 Caddy（自动证书）。
- **备份**：拷贝那个 `.db` 文件即可。

## 路线图

- **v1（当前）**：一键收藏、异步自动打标签+摘要、标签复用/归一化、导航站（卡片墙 + 标签筛选 + 搜索）、CRUD 后台（增删标签 / 重命名 / 合并 / 重打标 / 删除）、单用户 token、单二进制。
- **v1.1（计划）**：自然语言「整理助手」——用一句话描述调整意图，模型翻译成操作清单，预览确认后执行（复用现有操作层）。

## License

MIT
