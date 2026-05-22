# 配置文件说明

Qwen2API_Go 使用项目根目录下的 `.env` 作为配置文件。程序启动时会先检查 `.env` 是否存在；如果不存在，会自动生成一份默认模板，然后读取其中的键值。

> [!NOTE]
> `.env` 是纯文本键值文件，每行格式为 `KEY=value`。空行和以 `#` 开头的注释会被忽略。

> [!IMPORTANT]
> 如果同名环境变量已经由系统、Docker、Compose 或进程管理器注入，启动时默认不会被 `.env` 覆盖。后台的“重新加载 .env”会用 `.env` 覆盖当前进程中的同名变量。

## 最小可用配置

```env
API_KEY=sk-admin-change-me,sk-user-change-me
DATA_SAVE_MODE=file
QWEN_CHAT_PROXY_URL=https://chat.qwen.ai
SERVICE_PORT=3000
```

如果希望启动时预置 Qwen 账号，可以再加入：

```env
ACCOUNTS=user1@example.com:password1,user2@example.com:password2
```

> [!WARNING]
> 不要把真实 `.env` 提交到公开仓库。`API_KEY`、账号密码、Redis 地址都属于敏感配置。

## 读取规则

| 规则 | 说明 |
| --- | --- |
| 自动生成 | 根目录没有 `.env` 时，程序会写入默认模板。 |
| 解析方式 | 每行按第一个 `=` 分割；首尾空白会被去除；外层单引号或双引号会被去除。 |
| 布尔值 | 使用 Go `strconv.ParseBool`，支持 `true`、`false`、`1`、`0` 等格式；非法值回退默认值。 |
| 整数值 | 使用十进制整数；非法值回退默认值。 |
| 逗号列表 | 多值字段用英文逗号分隔，空项会被忽略。 |
| 多行提示词 | `QWEN_WEB2_CONTROL_PROMPT` 会把字面量 `\n` 转成换行。 |

## 热更新配置

后台设置页可以修改部分运行时配置，保存后立即生效，并写回 `.env`。

| 配置项 | 是否热更新 | 说明 |
| --- | --- | --- |
| `AUTO_REFRESH` | 是 | 是否自动刷新账号令牌。 |
| `AUTO_REFRESH_INTERVAL` | 是 | 自动刷新间隔。 |
| `BATCH_LOGIN_CONCURRENCY` | 是 | 批量登录并发数。 |
| `OUTPUT_THINK` | 是 | 是否向客户端输出 reasoning 内容。 |
| `SEARCH_INFO_MODE` | 是 | 搜索信息输出模式。 |
| `SIMPLE_MODEL_MAP` | 是 | 是否启用简化模型映射。 |
| `CHAT_CLEANUP_MODE` | 是 | 对话清理策略。 |
| `QWEN_WEB2_CONTROL_PROMPT` | 是 | Qwen Web2 控制提示词。 |
| `PROMPT_OVERRIDES_JSON` | 是 | Prompt 模板覆盖。 |

> [!TIP]
> 手动修改服务器上的 `.env` 后，可以在后台设置页点击“重新加载 .env”，无需重启进程。

## 访问与鉴权

| 配置项 | 默认值 | 作用 |
| --- | --- | --- |
| `API_KEY` | 空 | 允许访问 API 和后台的 key 列表，多个 key 用逗号分隔。第一个 key 会作为管理员 key。 |
| `SERVICE_PORT` | `3000` | HTTP 服务监听端口。 |
| `LISTEN_ADDRESS` | `0.0.0.0` | HTTP 服务监听地址。留空时运行时也会回退到 `0.0.0.0`。 |

### `API_KEY`

`API_KEY` 是必填项。程序启动后如果没有任何 key，会直接退出。

```env
API_KEY=sk-admin-change-me,sk-user-change-me
```

- 第一个 key：管理员 key，可登录后台和调用管理接口。
- 后续 key：普通业务 key，可调用兼容接口。
- 请求 OpenAI 兼容接口时使用 `Authorization: Bearer <key>`。
- 请求 Anthropic 兼容接口时支持 `x-api-key: <key>` 或 `Authorization: Bearer <key>`。

> [!CAUTION]
> 不建议把管理员 key 给业务客户端使用。管理员 key 能访问后台管理能力，泄露后影响更大。

### `SERVICE_PORT`

控制服务端口：

```env
SERVICE_PORT=3000
```

Docker 部署时还需要配合端口映射，例如 `-p 3000:3000`。

### `LISTEN_ADDRESS`

控制监听网卡：

```env
LISTEN_ADDRESS=0.0.0.0
```

- `0.0.0.0`：监听所有网卡，适合 Docker 或需要外部访问的部署。
- `127.0.0.1`：只监听本机，适合放在反向代理后面或本地使用。

## 账号与数据存储

| 配置项 | 默认值 | 作用 |
| --- | --- | --- |
| `DATA_SAVE_MODE` | 代码默认 `none`，模板默认 `file` | 账号、令牌和会话映射的存储模式。 |
| `ACCOUNTS` | 空 | 预置 Qwen 账号，格式为 `email:password,email:password`。 |
| `REDIS_URL` | 空 | Redis 连接地址，仅 `DATA_SAVE_MODE=redis` 时必填。 |

### `DATA_SAVE_MODE`

```env
DATA_SAVE_MODE=file
```

可选值：

| 值 | 行为 |
| --- | --- |
| `guest` | 使用匿名 guest cookies；账号存储为空。 |
| `none` | 只读取 `ACCOUNTS`，不持久化账号变更。 |
| `file` | 使用 `data/data.json` 保存账号、令牌和会话映射。 |
| `redis` | 使用 Redis 保存账号和会话映射。 |

> [!NOTE]
> 默认模板使用 `DATA_SAVE_MODE=file`，更适合 Docker 挂载 `./data:/app/data` 后长期运行。代码层面的空值默认是 `none`。

### `ACCOUNTS`

```env
ACCOUNTS=user1@example.com:password1,user2@example.com:password2
```

- 只在 `DATA_SAVE_MODE=none` 时作为只读账号来源直接使用。
- 在 `file` 或 `redis` 模式下，账号主要从持久化存储读取；可通过后台添加或批量登录。
- 单个账号格式为 `email:password`，多个账号用逗号分隔。

> [!WARNING]
> `ACCOUNTS` 不支持密码里包含未转义的英文逗号。需要复杂密码或多账号管理时，优先使用后台写入 `file`/`redis` 存储。

### `REDIS_URL`

```env
REDIS_URL=redis://127.0.0.1:6379/0
```

仅当 `DATA_SAVE_MODE=redis` 时需要。程序会使用 Redis 保存：

- `user:<email>`：账号、token、过期时间。
- `chat_session:<hash>`：OpenAI 兼容多轮对话与上游 Qwen chat_id 的映射。

## Qwen 上游与网络

| 配置项 | 默认值 | 作用 |
| --- | --- | --- |
| `QWEN_CHAT_PROXY_URL` | `https://chat.qwen.ai` | Qwen Chat 上游地址。 |
| `PROXY_URL` | 空 | 出站 HTTP 代理。 |
| `CACHE_MODE` | `default` | 缓存模式标记；当前版本主要用于配置展示，保持默认即可。 |

### `QWEN_CHAT_PROXY_URL`

```env
QWEN_CHAT_PROXY_URL=https://chat.qwen.ai
```

用于所有 Qwen Web 请求。一般保持默认；如果你有反代、网关或自定义上游，可以改成对应 base URL。

### `PROXY_URL`

```env
PROXY_URL=http://127.0.0.1:7890
```

用于服务端访问上游时的出站代理。留空表示直连。

> [!TIP]
> 账号频繁触发 429 或验证升级时，优先检查共享规模、并发、出口 IP 和代理稳定性，而不是单纯增加账号数量。

## 运行时行为

| 配置项 | 默认值 | 作用 |
| --- | --- | --- |
| `AUTO_REFRESH` | `true` | 是否后台自动刷新账号令牌。 |
| `AUTO_REFRESH_INTERVAL` | `21600` | 自动刷新间隔，单位秒。默认 6 小时。 |
| `BATCH_LOGIN_CONCURRENCY` | `5` | 批量登录任务的并发账号数。 |
| `SIMPLE_MODEL_MAP` | `false` | 是否返回简化后的模型列表。 |
| `SEARCH_INFO_MODE` | `text` | 搜索信息输出模式，可选 `text` 或 `table`。 |
| `OUTPUT_THINK` | `false` | 是否输出模型思考内容。 |
| `CHAT_CLEANUP_MODE` | `0` | Qwen 上游历史对话清理策略。 |

### `AUTO_REFRESH`

```env
AUTO_REFRESH=true
```

开启后，账号服务会按间隔刷新已保存账号的令牌，减少运行中 token 过期造成的失败。

### `AUTO_REFRESH_INTERVAL`

```env
AUTO_REFRESH_INTERVAL=21600
```

单位是秒。值越小，刷新越频繁；值越大，账号令牌过期前被刷新的概率越低。

### `BATCH_LOGIN_CONCURRENCY`

```env
BATCH_LOGIN_CONCURRENCY=5
```

控制后台批量登录时同时处理的账号数量。

> [!CAUTION]
> 并发过高更容易触发上游限流、验证码或账号风控。除非明确知道出口和账号池承载能力，否则不要盲目调大。

### `SIMPLE_MODEL_MAP`

```env
SIMPLE_MODEL_MAP=false
```

控制模型列表返回方式。开启后会使用更简化的模型映射，适合只需要常见 OpenAI 兼容模型名的客户端。

### `SEARCH_INFO_MODE`

```env
SEARCH_INFO_MODE=text
```

可选值：

- `text`：以普通文本形式输出搜索相关信息。
- `table`：以表格形式输出搜索相关信息。

非法值会回退为 `text`。

### `OUTPUT_THINK`

```env
OUTPUT_THINK=false
```

控制是否向客户端暴露模型的思考/推理摘要。

- OpenAI 兼容接口：开启后输出到 `reasoning_content` 字段。
- 关闭时：思考阶段内容会被过滤，只保留最终回答。

> [!WARNING]
> 开启 `OUTPUT_THINK` 会把上游返回的思考摘要透传给客户端。公开服务或多租户场景建议保持关闭。

### `CHAT_CLEANUP_MODE`

```env
CHAT_CLEANUP_MODE=0
```

可选值：

| 值 | 行为 |
| --- | --- |
| `0` | 不删除上游历史对话。 |
| `1` | 只删除本程序创建并记录过的对话。 |
| `2` | 删除 1 天前的所有上游对话。 |

> [!CAUTION]
> `CHAT_CLEANUP_MODE=2` 会清理更大范围的上游历史对话。除非你确认该账号只用于本服务，否则不要启用。

## 日志

| 配置项 | 默认值 | 作用 |
| --- | --- | --- |
| `LOG_LEVEL` | `INFO` | 日志等级配置；当前版本主要用于配置展示，实际过滤由 `DEBUG_MODE` 控制。 |
| `DEBUG_MODE` | `false` | 是否启用调试日志。 |
| `ENABLE_FILE_LOG` | `false` | 文件日志开关配置；当前版本主要用于配置展示，日志仍输出到标准输出。 |
| `LOG_DIR` | `./logs` | 文件日志目录配置；当前版本主要用于配置展示。 |
| `MAX_LOG_FILE_SIZE` | `10` | 单个日志文件最大大小配置，单位 MB；当前版本主要用于配置展示。 |
| `MAX_LOG_FILES` | `5` | 保留日志文件数量配置；当前版本主要用于配置展示。 |
| `NO_COLOR` | 空 | 设置为任意值后，控制台日志不输出 ANSI 颜色。 |

### `DEBUG_MODE`

```env
DEBUG_MODE=false
```

开启后会输出更多调试信息，适合排查上游响应、工具调用解析、请求转换等问题。

> [!WARNING]
> 调试日志可能包含请求内容、账号标识或上游响应片段。排查结束后建议关闭。

### 文件日志

```env
ENABLE_FILE_LOG=true
LOG_DIR=./logs
MAX_LOG_FILE_SIZE=10
MAX_LOG_FILES=5
```

这些字段保留为文件日志配置入口。当前版本日志仍输出到标准输出；Docker 部署时通常通过 `docker logs` 或容器日志驱动收集。

## Prompt 覆盖

| 配置项 | 默认值 | 作用 |
| --- | --- | --- |
| `QWEN_WEB2_CONTROL_PROMPT` | 空 | 兼容旧配置，等价于覆盖 `qwen.web2.control`。 |
| `PROMPT_OVERRIDES_JSON` | `{}` | 使用 JSON 对象覆盖内置 prompt 模板。 |

### `QWEN_WEB2_CONTROL_PROMPT`

```env
QWEN_WEB2_CONTROL_PROMPT=请保持简洁回答
```

该提示词会注入到 Qwen Web2 聊天请求最前方。为空则不注入。

### `PROMPT_OVERRIDES_JSON`

```env
PROMPT_OVERRIDES_JSON={"qwen.web2.control":"请保持简洁回答","assets.image_edit.default":"请基于上传图片完成自然编辑"}
```

支持的 prompt ID：

<details>
<summary>点击展开 prompt ID 列表</summary>

| ID | 用途 |
| --- | --- |
| `qwen.web2.control` | Qwen Web2 控制提示词。 |
| `openai.toolcall.prompt` | OpenAI 工具调用总提示词。 |
| `openai.toolcall.instructions` | OpenAI 工具调用 XML 协议。 |
| `openai.toolcall.reminder` | 最新用户消息前的工具提醒。 |
| `anthropic.response_format.json_object` | Anthropic `json_object` 响应格式提示。 |
| `anthropic.response_format.json_schema` | Anthropic `json_schema` 响应格式提示。 |
| `anthropic.response_format.json_schema_fallback` | Anthropic JSON schema 兜底提示。 |
| `assets.image_edit.default` | 图片编辑接口默认 prompt。 |
| `frontend.debug.system` | 后台调试台默认 system prompt。 |
| `frontend.image.default` | 后台生图页默认 prompt。 |
| `frontend.video.default` | 后台生视频页默认 prompt。 |

</details>

> [!IMPORTANT]
> 部分 prompt 模板包含占位符，例如 `{{tool_details}}`、`{{instructions}}`、`{{schema}}`。覆盖这些模板时要保留对应占位符，否则相关协议可能失效。

## 完整示例

下面是一份偏生产使用的 `.env` 示例。按需替换 key、账号、Redis 和代理配置。

```env
# Qwen2API_Go configuration

# Access keys. The first key is the admin key.
API_KEY=sk-admin-change-me,sk-user-change-me

# Account and persistence mode:
# guest = anonymous guest cookies
# none  = read ACCOUNTS only, no persistence
# file  = persist accounts to data/data.json
# redis = persist accounts to Redis via REDIS_URL
DATA_SAVE_MODE=file
ACCOUNTS=user1@example.com:password1,user2@example.com:password2
REDIS_URL=

# Service listen settings
LISTEN_ADDRESS=0.0.0.0
SERVICE_PORT=3000

# Qwen upstream and outbound proxy
QWEN_CHAT_PROXY_URL=https://chat.qwen.ai
PROXY_URL=
CACHE_MODE=default

# Runtime behavior
AUTO_REFRESH=true
AUTO_REFRESH_INTERVAL=21600
BATCH_LOGIN_CONCURRENCY=5
SIMPLE_MODEL_MAP=false
SEARCH_INFO_MODE=text
OUTPUT_THINK=false
CHAT_CLEANUP_MODE=0

# Prompt overrides
QWEN_WEB2_CONTROL_PROMPT=
PROMPT_OVERRIDES_JSON={}

# Logging
LOG_LEVEL=INFO
DEBUG_MODE=false
ENABLE_FILE_LOG=false
LOG_DIR=./logs
MAX_LOG_FILE_SIZE=10
MAX_LOG_FILES=5

```

## 常见组合

### 单机 Docker 持久化

```env
DATA_SAVE_MODE=file
```

配合 Docker volume：

```bash
-v ./data:/app/data
```

### 只从环境变量读取账号

```env
DATA_SAVE_MODE=none
ACCOUNTS=user1@example.com:password1
```

适合临时测试，不会保存后台新增账号或刷新后的 token。

### 多实例共享账号池

```env
DATA_SAVE_MODE=redis
REDIS_URL=redis://127.0.0.1:6379/0
```

适合多个实例共享账号、token 和会话映射。

> [!WARNING]
> 多实例共享同一批账号时，要额外控制总并发。每个实例单独限流并不等于账号池整体安全。
