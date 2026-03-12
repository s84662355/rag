# Go + Vue RAG

本项目是一个完整的 RAG 系统：
- 后端：Go（`net/http`，不依赖后端框架）
- 前端：Vue 3（`vite + vue-router + axios`）

## 功能特性

- 文档解析：`md`、`pdf`、`html`、`txt`
- 支持通过 URL 解析网页内容
- 长文档自动切分 chunk
- 使用 Ollama（`bge-m3`）进行向量化
- 支持多知识库（通过 `kb_id` 隔离）
- 多路召回检索（向量 + 关键词）
- Rewrite -> Search -> Rerank -> QA -> Chat 流程
- 支持 chunk 编辑并重新向量化
- 自动生成 QA 问答对
- 基于 `session_id` 的多轮对话
- 文档删除接口
- 文档重建索引接口

## 项目结构

```text
cmd/server/main.go
internal/config
internal/httpapi
internal/service
internal/store
internal/vector
internal/parser
internal/chunker
internal/embedding
internal/llm
migrations/001_init.sql
config.yaml
web/                    # Vue 前端项目
```

## 环境要求

- Go 1.24.1+
- Node.js 18+
- MySQL 8+
- Elasticsearch 8+
- Ollama（可用 embedding 接口）
- DeepSeek API 访问权限

## 数据库初始化

```sql
CREATE DATABASE IF NOT EXISTS rag DEFAULT CHARSET utf8mb4 COLLATE utf8mb4_general_ci;
```

执行迁移：

```bash
mysql -uroot -p123456 rag < migrations/001_init.sql
```

## 前端（Vue）

```bash
cd web
npm install
npm run build
```

构建产物目录为 `web/dist`。

## 后端

```bash
go mod tidy
go run ./cmd/server
```

打开：

- `http://127.0.0.1:8080/`

`config.yaml` 当前配置为：

```yaml
server:
  staticDir: "web/dist"
```

## 主要接口

- `GET /api/health`
- `GET/POST /api/kbs`
- `DELETE /api/kbs/{id}`
- `POST /api/documents/upload`
- `POST /api/documents/url`
- `GET /api/documents?kb_id=...`
- `DELETE /api/documents/{id}`
- `POST /api/documents/{id}/reindex`
- `GET /api/chunks?doc_id=...`
- `PUT /api/chunks/{id}`
- `POST /api/search`
- `POST /api/retrieve/debug`
- `POST /api/qa/generate`
- `GET /api/qa?doc_id=...`
- `POST /api/chat`
