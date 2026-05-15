# WebClip 网络剪贴板

基于 **GoFrame v2** + **Vue 3** 的极简网络剪贴板：
- 通过 6 位短码创建/加入房间
- 可选房间密码（留空则无密码）
- 基于 WebSocket 的多端实时同步（A 粘贴后 B 立即看到）
- SQLite 单机部署，单二进制可运行

## 目录结构

```
webclip/
├── server/   # GoFrame 后端
└── web/      # Vue 3 前端
```

## 开发（双进程）

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
# http://localhost:5173  （Vite 代理 /api 与 /api/ws 到后端）
```

## 生产构建（单二进制）

```bash
# 1. 构建前端，产物会输出到 server/resource/public
cd web
npm run build

# 2. 编译后端
cd ../server
go build -o webclip .

# 3. 启动
./webclip
# 访问 http://localhost:8080
```

## 目录说明

后端：
- `server/main.go`                        入口
- `server/manifest/config/config.yaml`    配置（端口、SQLite 文件、JWT 密钥、TTL 等）
- `server/internal/cmd/cmd.go`            路由注册、迁移、定时任务、SPA fallback
- `server/internal/controller/clip.go`    REST 控制器
- `server/internal/controller/ws.go`      WebSocket 控制器
- `server/internal/logic/clip/clip.go`    业务逻辑（短码、密码、token、过期清理）
- `server/internal/logic/hub/hub.go`      房间广播 Hub
- `server/internal/model/entity/clip.go`  数据实体
- `server/data/webclip.db`                运行时自动创建的 SQLite 数据库

前端：
- `web/src/views/Home.vue`   首页：创建 / 加入
- `web/src/views/Room.vue`   房间页：编辑 + WebSocket 实时同步
- `web/src/api/clip.ts`      REST 封装（自动注入 token）
- `web/src/stores/room.ts`   Pinia：按 code 存储 token（sessionStorage）

## API 概览

| 方法 | 路径 | 说明 |
| --- | --- | --- |
| POST | `/api/clip` | 创建房间，body `{ password? }` → `{ code, hasPassword, token }` |
| GET | `/api/clip/{code}/meta` | 免鉴权，查询房间元信息 |
| POST | `/api/clip/{code}/auth` | 校验密码（空密码房间传空串亦通过），返回 `{ token }` |
| GET | `/api/clip/{code}` | 获取内容，Header `Authorization: Bearer <token>` |
| PUT | `/api/clip/{code}` | 更新内容，body `{ content, contentType }` |
| GET | `/api/ws/{code}?token=...` | WebSocket 实时同步入口 |

WebSocket 消息协议（JSON）：
- 客户端 → 服务端：`{ "type": "update", "content": "...", "contentType": "text" }` / `{ "type": "ping" }`
- 服务端 → 客户端：`{ "type": "update", "content": "...", "from": "<clientId>", "updatedAt": "..." }`

## 默认配置

- 房间 TTL：7 天（每次写入刷新）
- Token 有效期：2 小时
- 过期清理：每 10 分钟扫描一次
- 字符集：`ABCDEFGHJKLMNPQRSTUVWXYZ23456789`（去除 `0`/`O`/`1`/`I` 等易混字符）

可在 `server/manifest/config/config.yaml` 的 `webclip` 节点调整。

## WebSocket 端到端自测

```bash
cd server
# 启动服务
./webclip &

# 创建一个房间拿到 code 和 token
R=$(curl -s -X POST http://localhost:8080/api/clip -H 'Content-Type: application/json' -d '{"password":""}')
CODE=$(echo "$R" | sed -E 's/.*"code":"([A-Z0-9]+)".*/\1/')
TOKEN=$(echo "$R" | sed -E 's/.*"token":"([^"]+)".*/\1/')

# 运行实时同步测试（A 发送 → B 接收）
go run -tags=wstest ./scripts/wstest.go "$CODE" "$TOKEN"
# OK: B received update from A, content= hello-from-A
```
