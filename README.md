# cnv-locale-router

把多语言文案、区域路由前缀、回退语言链与翻译版本记录成可查询的服务，支持按区域和命名空间下发文案，并可追溯每一次翻译内容的变更。

## 运行要求

- Go 1.26 或以上
- SQLite（本服务自带存储，不需要外部数据库）

## 构建、测试与启动

```bash
go build ./...
go test ./...
go run .
```

服务默认监听 `127.0.0.1:8080`。可用环境变量覆盖：

| 变量 | 默认值 | 用途 |
|---|---|---|
| `ADDR` | `127.0.0.1:8080` | HTTP 监听地址 |
| `DB_PATH` | `cnv-locale-router.db` | SQLite 数据库文件路径 |

## 已公开的入口

### `GET /healthz`

返回服务与存储状态。正常时 HTTP 200：

```json
{"status":"ok","database":"ok"}
```

存储不可用时 HTTP 503：

```json
{"error":{"code":"storage_unavailable","message":"database is not available"}}
```

### `POST /message-entries`

登记一条文案及其区域路由前缀。请求体必须是单个 JSON 对象，包含五个必填字符串字段：

| 字段 | 规则 |
|---|---|
| `namespace` | 原值保存，不能全为空白 |
| `locale` | BCP 47 语言标签，保存时规范化大小写（如 `EN-us` → `en-US`） |
| `region` | 两个 ASCII 字母，保存时转大写 |
| `prefix` | 以 `/` 开头，不含空白、`?`、`#` |
| `content` | 原值保存，不能为空 |

成功返回 HTTP 201 及五个字段的保存值。同一 `namespace`+规范化 `locale` 只有一条文案，同一 `region` 只对应一个 `prefix`：重复提交且 `region`、`prefix`、`content` 完全一致时仍返回 201；任一值不同或区域已被绑定到其他前缀时返回 409 `LocaleConflictError`，原记录不变。

### `GET /route-mappings/{locale}?namespace=...`

按语言标签（规则同登记）与必填的 `namespace` 查询参数解析路由，返回 HTTP 200 及 `namespace`、`locale`、`region`、`prefix`（不含 `content`）。不做语言回退。

可能的错误：非法语言 400 `InvalidLocaleInputError`；`namespace` 缺失、重复或为空白 400 `InvalidQueryInputError`；无对应文案 404 `LocaleNotFoundError`。

## 错误约定

所有错误响应都是单个顶层 `error` 对象，包含 `code` 与 `message` 两个字符串字段；`message` 不包含 SQL、堆栈或文件路径。
