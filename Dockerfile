# syntax=docker/dockerfile:1.6

# ---------- Stage 1: 构建前端 ----------
FROM node:20-alpine AS web-builder

WORKDIR /app

# 先复制依赖清单，利用层缓存
COPY web/package.json web/package-lock.json ./web/
RUN cd web && npm ci

# 复制前端源码并构建
# vite 配置中 outDir 指向 ../server/resource/public，因此输出会落在 /app/server/resource/public
COPY web ./web
RUN cd web && npm run build


# ---------- Stage 2: 构建后端二进制 ----------
FROM golang:1.25-alpine AS server-builder

WORKDIR /app/server

# 纯 Go 编译，无需 CGO（sqlite 使用 modernc.org/sqlite）
ENV CGO_ENABLED=0 \
    GOOS=linux \
    GOFLAGS=-trimpath

# 先下载依赖，利用层缓存
COPY server/go.mod server/go.sum ./
RUN go mod download

# 复制后端源码
COPY server ./

# 注入前端构建产物
COPY --from=web-builder /app/server/resource/public ./resource/public

# 构建静态二进制
RUN go build -ldflags="-s -w" -o /out/webclip .


# ---------- Stage 3: 运行镜像 ----------
FROM alpine:3.20

# 时区与 CA 证书
RUN apk add --no-cache ca-certificates tzdata && \
    addgroup -S app && adduser -S -G app app

ENV TZ=Asia/Shanghai

WORKDIR /app

# 拷贝二进制与运行所需的资源
COPY --from=server-builder /out/webclip          /app/webclip
COPY --from=server-builder /app/server/manifest  /app/manifest
COPY --from=server-builder /app/server/resource  /app/resource

# 数据目录（SQLite / 日志 / sessions）
RUN mkdir -p /app/data && chown -R app:app /app

USER app

EXPOSE 8080
VOLUME ["/app/data"]

ENTRYPOINT ["/app/webclip"]
