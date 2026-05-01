# Qwen2API_Go

把 Qwen Chat 包装成 OpenAI / Anthropic 兼容接口，带管理后台、账号池、文件上传、图片/视频生成。

适合：

- 个人自用
- 小规模内网转发
- 给现有 OpenAI SDK / Anthropic SDK 直接接入

不适合：

- 公益池
- 多人共享大并发
- 对外售卖

## 使用前提醒

### 请尽量仅自用

Qwen 官方服务本身存在限流与风控。多人共用、固定出口 IP 高频请求、公开分发账号，都更容易触发 429、验证升级、掉号或限速。

建议：

- 仅个人或小范围使用
- 控制并发和请求频率
- 不要长期把同一批账号暴露给大量用户
- 触发 429 或异常验证后先降速，再检查出口 IP、代理和共享规模

## Docker 部署

这是最推荐的部署方式。

### 直接拉取云端镜像

默认会推送到 GHCR：

```bash
docker pull ghcr.io/xxxxteam/qwen2api_go:latest
```

如果你要固定版本，改成具体 tag：

```bash
docker pull ghcr.io/xxxxteam/qwen2api_go:0.2.0
```

### 准备配置文件

在宿主机新建一个 `.env` 文件，最少内容如下：

```env
API_KEY=sk-admin-change-me,sk-user-change-me
DATA_SAVE_MODE=file
QWEN_CHAT_PROXY_URL=https://chat.qwen.ai
SERVICE_PORT=3000
```

如果你要预置账号，可以继续加：

```env
ACCOUNTS=user1@example.com:password1,user2@example.com:password2
```

### 直接运行容器

```bash
docker run -d \
  --name qwen2api \
  --restart unless-stopped \
  -p 3000:3000 \
  --env-file .env \
  -v ./data:/app/data \
  ghcr.io/xxxxteam/qwen2api_go:latest
```

说明：

- `-p 3000:3000` 是把服务暴露到宿主机 `3000` 端口
- `--env-file .env` 用于加载配置
- `-v ./data:/app/data` 用于持久化本地数据

### 使用 docker compose

```yaml
services:
  qwen2api:
    image: ghcr.io/xxxxteam/qwen2api_go:latest
    container_name: qwen2api
    restart: unless-stopped
    ports:
      - "3000:3000"
    env_file:
      - .env
    volumes:
      - ./data:/app/data
```

启动：

```bash
docker compose up -d
```

更新：

```bash
docker compose pull
docker compose up -d
```

查看日志：

```bash
docker logs -f qwen2api
```

## 裸机部署

如果你不想用 Docker，也可以直接运行程序。

### Windows

下载发布包后，解压并运行：

```powershell
.\qwen2api.exe
```

### Linux / macOS

给执行权限后启动：

```bash
chmod +x ./qwen2api
./qwen2api
```

程序会自动读取根目录 `.env`。如果不存在，会自动生成默认模板。

## 登录后台

默认地址：

```text
http://127.0.0.1:3000
```

登录方式：

- 使用 `API_KEY` 中第一个 key 作为管理员 key

## 配置热更新

后台中的以下运行参数支持热更新，保存后立即生效，同时会写回 `.env`：

- 自动刷新账号令牌
- 自动刷新间隔
- 批量登录并发
- 输出思考过程
- 搜索信息模式
- 简化模型映射

如果你是直接手动修改服务器上的 `.env`，也可以在后台设置页点击“重新加载 .env”，无需重启进程。

## 常用接口

### OpenAI Chat Completions

```bash
curl http://127.0.0.1:3000/v1/chat/completions \
  -H "Authorization: Bearer sk-admin-change-me" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "qwen3-235b-a22b",
    "reasoning_effort": "high",
    "messages": [
      {"role": "user", "content": "你好"}
    ],
    "stream": false
  }'
```

`reasoning_effort` 鍏煎 OpenAI Chat Completions 椋庢牸鍙傛暟锛屾敮鎸?`none`銆乴inimal`銆乴ow`銆乵edium`銆乨igh`銆乺high`銆?`none/minimal/low` 浼氭槧灏勪负 Fast锛?`medium/high/xhigh` 浼氭槧灏勪负 Thinking銆?

### Anthropic Messages

支持 `x-api-key` 和 `Authorization: Bearer ...` 两种鉴权方式。

```bash
curl http://127.0.0.1:3000/v1/messages \
  -H "x-api-key: sk-admin-change-me" \
  -H "anthropic-version: 2023-06-01" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "qwen3-235b-a22b-thinking",
    "max_tokens": 1024,
    "messages": [
      {"role": "user", "content": "你好"}
    ]
  }'
```

### Count Tokens

```bash
curl http://127.0.0.1:3000/v1/messages/count_tokens \
  -H "x-api-key: sk-admin-change-me" \
  -H "anthropic-version: 2023-06-01" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "qwen3-235b-a22b",
    "system": "You are helpful",
    "messages": [
      {"role": "user", "content": "你好"}
    ]
  }'
```

### 文件上传

```bash
curl http://127.0.0.1:3000/v1/uploads \
  -H "Authorization: Bearer sk-admin-change-me" \
  -F "files=@demo.png"
```

## 常用配置

- `API_KEY`
  多个 key 用逗号分隔，第一个默认是管理员 key
- `DATA_SAVE_MODE`
  可选 `none`、`file`、`redis`
- `ACCOUNTS`
  预置账号，格式 `email:password,email:password`
- `SERVICE_PORT`
  服务端口，默认 `3000`
- `QWEN_CHAT_PROXY_URL`
  上游地址，默认 `https://chat.qwen.ai`
- `PROXY_URL`
  可选代理地址
- `REDIS_URL`
  Redis 地址，仅 `DATA_SAVE_MODE=redis` 时需要

## 运行建议

- 管理后台和业务接口只暴露给可信环境
- 管理员 key 与业务 key 分开使用
- 如果放在反代后面，记得只开放必要端口
- 如果账号频繁失效，优先检查并发、共享规模、出口 IP 与代理稳定性

## 从源码启动

如果你就是要自己编译运行：

```powershell
go run ./cmd/qwen2api
```
