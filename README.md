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

登记一条文案及其区域路由前缀。请求体必须是单个 JSON 对象，且包含五个字符串字段：

| 字段 | 规则 |
|---|---|
| `namespace` | 必填，不能全为空白；保存原值 |
| `locale` | 必填，BCP 47 语言标签；空值、前后带空白或非法标签不接受；保存时规范化大小写（语言小写、区域大写、书写体首字母大写） |
| `region` | 必填，恰好两个 ASCII 字母，独立于语言校验；保存为大写 |
| `prefix` | 必填，以 `/` 开头，不含空白、`?`、`#`，允许包含 `/` |
| `content` | 必填，不能为空字符串；保存原值 |

成功返回 HTTP 201 及这五个字段的保存值。同一 `(namespace, locale)` 完全一致的重复提交（区域、前缀、正文均相同）仍返回 201 与相同内容；已有区域被提交不同前缀，或同一 `(namespace, locale)` 的任一值不同，返回 409 `LocaleConflictError`。登记失败不会改动既有记录，也不会留下单独的文案或区域映射。

### `GET /route-mappings/{locale}`

以必填的 `namespace` 查询参数（必须恰好出现一次且不能为空白）选择文案，路径中的 `locale` 遵循与登记相同的 BCP 47 规则。成功返回 HTTP 200 及除 `content` 外的登记字段（`namespace`、`locale`、`region`、`prefix`），前缀取自区域映射，不做任何语言回退。合法的语言与命名空间组合没有文案时返回 404 `LocaleNotFoundError`。

## 错误码

| code | 状态码 | 含义 |
|---|---|---|
| `InvalidMessageEntryInputError` | 400 | 请求体不是单个 JSON 对象、字段缺失、类型错误，或非语言字段非法 |
| `InvalidLocaleInputError` | 400 | 语言标签为空、带前后空白或不符合 BCP 47 |
| `InvalidQueryInputError` | 400 | 语言合法但 `namespace` 查询参数缺失、重复或为空白 |
| `LocaleConflictError` | 409 | 同一命名空间与语言已有不同登记，或区域已映射到其他前缀 |
| `LocaleNotFoundError` | 404 | 合法的命名空间与语言组合没有文案 |
| `storage_unavailable` | 503 | 存储不可用 |
| `route_not_found` | 404 | 未知路径 |

输入校验（含语言校验）先于冲突判断；语言字段以外的校验先于语言校验。

## 错误约定

所有错误响应都是单个顶层 `error` 对象，包含 `code` 与 `message` 两个字符串字段；`message` 不包含 SQL、堆栈或文件路径。
