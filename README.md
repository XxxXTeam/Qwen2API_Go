# Qwen2API_Go

将 Qwen Chat 的能力包装成 OpenAI 兼容接口的 Go 版本，内置管理后台、账号池、图片/视频生成、OSS 上传和静态前端服务。

## 功能

- OpenAI 兼容接口
  - `/v1/chat/completions`
  - `/v1/models`
  - `/v1/images/generations`
  - `/v1/images/edits`
  - `/v1/videos`
  - `/v1/uploads`
  - `/v1/files/upload`
- 管理后台
  - 账号池管理
  - 系统设置
  - 模型能力查看
  - 文件上传
  - 接口调试
- 存储模式
  - `none`
  - `file`
  - `redis`

## 首次启动

程序启动时会自动检查项目根目录的 `.env`：

- 如果 `.env` 已存在，直接读取
- 如果 `.env` 不存在，自动生成一份带注释的默认配置模板

你只需要修改里面最关键的配置：

```env
API_KEY=sk-admin-change-me,sk-user-change-me
DATA_SAVE_MODE=file
QWEN_CHAT_PROXY_URL=https://chat.qwen.ai
```

如果你要预置账号，可以补：

```env
ACCOUNTS=user1@example.com:password1,user2@example.com:password2
```

## 编译

### 当前平台直接编译

```powershell
go build -o qwen2api.exe ./cmd/qwen2api
```

Linux / macOS 只需要把输出名改掉：

```bash
go build -o qwen2api ./cmd/qwen2api
```

### 手工交叉编译

Windows amd64:

```powershell
$env:GOOS="windows"
$env:GOARCH="amd64"
$env:CGO_ENABLED="0"
go build -trimpath -o dist/windows-amd64/qwen2api.exe ./cmd/qwen2api
```

Linux amd64:

```powershell
$env:GOOS="linux"
$env:GOARCH="amd64"
$env:CGO_ENABLED="0"
go build -trimpath -o dist/linux-amd64/qwen2api ./cmd/qwen2api
```

macOS arm64:

```powershell
$env:GOOS="darwin"
$env:GOARCH="arm64"
$env:CGO_ENABLED="0"
go build -trimpath -o dist/darwin-arm64/qwen2api ./cmd/qwen2api
```

全部支持平台列表可以查看：

```powershell
go tool dist list
```

## 运行

### 直接运行源码

```powershell
go run ./cmd/qwen2api
```

### 运行编译后的程序

Windows:

```powershell
.\qwen2api.exe
```

Linux / macOS:

```bash
./qwen2api
```

## 使用方式

### 管理后台

默认监听地址：

```text
http://127.0.0.1:3000
```

进入页面后，使用 `API_KEY` 中的第一个 key 作为管理员 key 登录。

### OpenAI 兼容接口

聊天示例：

```bash
curl http://127.0.0.1:3000/v1/chat/completions \
  -H "Authorization: Bearer sk-admin-change-me" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "qwen3-235b-a22b",
    "messages": [
      {"role": "user", "content": "你好"}
    ],
    "stream": false
  }'
```

上传文件示例：

```bash
curl http://127.0.0.1:3000/v1/uploads \
  -H "Authorization: Bearer sk-admin-change-me" \
  -F "files=@demo.png"
```

## 常用配置说明

- `API_KEY`
  API 访问密钥，多个用逗号分隔，第一个默认是管理员 key。
- `DATA_SAVE_MODE`
  可选 `none`、`file`、`redis`。
- `ACCOUNTS`
  预置账号列表，格式 `email:password,email:password`。
- `QWEN_CHAT_PROXY_URL`
  上游 Qwen 地址。
- `PROXY_URL`
  可选代理地址。
- `REDIS_URL`
  Redis 存储地址，仅 `DATA_SAVE_MODE=redis` 时使用。
- `SERVICE_PORT`
  服务端口，默认 `3000`。

## 开发检查

```powershell
go test ./...
go build ./...
```
