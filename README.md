# WebClip 网络剪贴板

基于 **GoFrame v2** + **Vue 3** 的极简网络剪贴板，支持文本消息与文件共享。

## 功能特性

- **房间管理** — 通过 6 位短码创建/加入房间，支持自定义房间名称和密码
- **消息流** — 房间内独立消息列表，倒序展示，支持复制和删除
- **文件共享** — 上传文件至 S3 兼容存储，生成预签名下载链接（可选功能）
- **实时同步** — 基于 WebSocket 的多端实时同步，消息即时推送
- **自动过期** — 房间支持 TTL 自动清理，每次写入刷新过期时间
- **单二进制** — SQLite 存储，编译后单文件即可运行

## 目录结构

```
webclip/
├── server/                        # GoFrame 后端
│   ├── internal/
│   │   ├── cmd/cmd.go             # 路由注册、迁移、定时任务
│   │   ├── controller/
│   │   │   ├── clip.go            # REST 控制器（房间/消息/文件）
│   │   │   └── ws.go              # WebSocket 控制器
│   │   ├── logic/
│   │   │   ├── clip/clip.go       # 房间与消息业务逻辑
│   │   │   ├── hub/hub.go         # 房间广播 Hub
│   │   │   └── storage/           # S3 对象存储抽象层
│   │   └── model/entity/clip.go   # 数据实体（Clip, ClipMessage）
│   ├── manifest/config/config.yaml # 配置文件
│   └── data/                      # 运行时数据（SQLite/日志/会话）
├── web/                           # Vue 3 前端
│   └── src/
│       ├── views/
│       │   ├── Home.vue           # 首页：房间列表 + 创建/加入
│       │   └── Room.vue           # 房间页：消息流 + 文件上传
│       ├── api/clip.ts            # REST/WS 封装
│       └── stores/room.ts         # Pinia 状态管理
├── Dockerfile                     # 多阶段构建
└── .dockerignore
```

## 快速开始

### 开发模式（双进程）

启动后端：

```bash
cd server
go run .
# 默认监听 :8080
```

启动前端：

```bash
cd web
npm install
npm run dev
# http://localhost:5173 （Vite 代理 /api 与 /api/ws 到后端）
```

> 注意：若使用 oh-my-zsh，`gf` 会被别名覆盖为 `git fetch`，请使用 `go run .` 或 `\gf run .` 代替。

### 生产构建

```bash
# 1. 构建前端
cd web && npm run build   # 产物输出到 server/resource/public

# 2. 编译后端
cd ../server && go build -o webclip .

# 3. 启动
./webclip
# 访问 http://localhost:8080
```

### Docker 部署

```bash
# 构建镜像
docker build -t webclip:latest .

# 基础运行（仅文本消息）
docker run -d \
  --name webclip \
  -p 8080:8080 \
  -v webclip-data:/app/data \
  webclip:latest

# 启用文件共享（通过环境变量注入 S3 密钥）
docker run -d \
  --name webclip \
  -p 8080:8080 \
  -v webclip-data:/app/data \
  -e S3_ACCESS_KEY=你的AccessKey \
  -e S3_SECRET_KEY=你的SecretKey \
  webclip:latest
```

| 参数 | 说明 |
|------|------|
| `-p 8080:8080` | 映射端口，左侧为宿主机端口 |
| `-v webclip-data:/app/data` | 持久化数据卷（SQLite/日志/会话） |
| `-e S3_ACCESS_KEY` | S3 AccessKey，优先于 config.yaml |
| `-e S3_SECRET_KEY` | S3 SecretKey，优先于 config.yaml |

## API 概览

### 房间接口

| 方法 | 路径 | 说明 |
| --- | --- | --- |
| POST | `/api/clip` | 创建房间，body `{ password?, name? }` → `{ code, hasPassword, token }` |
| GET | `/api/rooms` | 列出所有房间 |
| GET | `/api/clip/{code}/meta` | 查询房间元信息（免鉴权） |
| POST | `/api/clip/{code}/auth` | 校验密码，返回 `{ token }` |
| GET | `/api/clip/{code}` | 获取房间内容 |
| PUT | `/api/clip/{code}` | 更新房间内容 |
| PATCH | `/api/clip/{code}` | 修改房间名称/密码 |
| DELETE | `/api/clip/{code}` | 删除房间 |

### 消息接口

| 方法 | 路径 | 说明 |
| --- | --- | --- |
| GET | `/api/clip/{code}/messages?beforeId=&limit=&contentType=` | 消息列表（倒序，游标分页） |
| POST | `/api/clip/{code}/messages` | 发送文本消息，body `{ content, contentType }` |
| DELETE | `/api/clip/{code}/messages/{id}` | 删除消息 |

### 文件接口

| 方法 | 路径 | 说明 |
| --- | --- | --- |
| POST | `/api/clip/{code}/files/upload` | 上传文件（multipart/form-data） |
| GET | `/api/clip/{code}/messages/{id}/download` | 获取预签名下载链接 |

### WebSocket

连接地址：`/api/ws/{code}?token=...`

客户端 → 服务端：

```json
{ "type": "send", "content": "...", "contentType": "text" }
{ "type": "send_file", "fileKey": "...", "fileName": "...", "fileSize": 1234 }
{ "type": "delete", "id": 1 }
{ "type": "ping" }
```

服务端 → 客户端：

```json
{ "type": "message_created", "message": { "id", "content", "contentType", "createdAt", ... }, "from": "<clientId>" }
{ "type": "message_deleted", "id": 1 }
{ "type": "pong" }
{ "type": "error", "error": "..." }
```

## 配置说明

配置文件：`server/manifest/config/config.yaml`

```yaml
webclip:
  jwtSecret: "123456"        # JWT 签名密钥，生产环境请修改
  tokenExpireHours: 2        # Token 有效期（小时）
  ttlDays: 1                 # 房间过期天数（每次写入刷新）

s3:
  vendor: "cos"              # cos=腾讯云COS，aws=AWS S3，留空则禁用文件功能
  endpoint: ""               # S3 端点
  region: ""
  bucket: ""
  accessKey: ""              # 建议通过环境变量 WEBCLIP_S3_ACCESSKEY 注入
  secretKey: ""              # 建议通过环境变量 WEBCLIP_S3_SECRETKEY 注入
  pathStyle: false           # MinIO 需 true，COS/AWS 需 false
  publicUrl: ""              # 可选，自定义下载域名（如 CDN）
  maxFileSize: 524288000     # 500MB
```

- 房间过期清理：每 10 分钟扫描一次
- 短码字符集：`ABCDEFGHJKLMNPQRSTUVWXYZ23456789`（去除易混字符 0/O/1/I）
- S3 密钥环境变量优先于配置文件，避免密钥明文写入配置

## 技术栈

**后端：** Go 1.25 · GoFrame v2 · SQLite (modernc.org/sqlite，纯 Go 无 CGO) · gorilla/websocket · AWS SDK for Go v2

**前端：** Vue 3 · TypeScript · Vite · Pinia · Vue Router · Axios

**部署：** Docker 多阶段构建 · Alpine Linux · 非 root 用户运行
