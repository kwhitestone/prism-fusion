# ==================================================
# Prism Fusion - 生产环境多阶段构建
# 集成 Go Gateway + Admin 前端
# ==================================================
# 
# 构建命令: docker build -t prism-fusion .
# 
# ==================================================

# 镜像注册表（可通过构建参数覆盖）
ARG REGISTRY=swr.cn-north-4.myhuaweicloud.com/ddn-k8s/docker.io

# ========== 阶段1: 前端构建阶段 ==========
FROM ${REGISTRY}/node:22.22.0-alpine3.23 AS frontend-builder

WORKDIR /app/admin

ENV ELECTRON_MIRROR=https://npmmirror.com/mirrors/electron/ \
    SASS_BINARY_SITE=https://npmmirror.com/mirrors/node-sass/ \
    PHANTOMJS_CDNURL=https://npmmirror.com/mirrors/phantomjs/ \
    CHROMEDRIVER_CDNURL=https://npmmirror.com/mirrors/chromedriver/ \
    PYTHON_MIRROR=https://npmmirror.com/mirrors/python/

RUN npm config set registry https://registry.npmmirror.com/ && \
    npm config set cache /tmp/.npm && \
    npm install -g pnpm && \
    pnpm config set registry https://registry.npmmirror.com/

COPY src/admin/package.json src/admin/pnpm-lock.yaml ./
RUN pnpm install --frozen-lockfile

COPY src/admin/ .

RUN pnpm run build

# ========== 阶段2: Go后端构建阶段 ==========
FROM ${REGISTRY}/golang:1.25.5 AS backend-builder

ENV GO111MODULE=on \
    GOPROXY=https://goproxy.cn,direct \
    GOSUMDB=sum.golang.google.cn \
    CGO_ENABLED=0 \
    GOOS=linux \
    GOARCH=amd64

# Set non-interactive mode to avoid tzdata blocking
ENV DEBIAN_FRONTEND=noninteractive

# 使用中国镜像加速 apt
RUN sed -i 's|deb.debian.org|mirrors.aliyun.com|g' /etc/apt/sources.list.d/debian.sources

RUN apt-get update && apt-get install -y \
    ca-certificates \
    tzdata && \
    rm -rf /var/lib/apt/lists/*

WORKDIR /app/server

COPY src/server/go.mod src/server/go.sum ./
RUN go mod download && go mod verify

COPY src/server/ .

RUN go build -ldflags="-s -w -extldflags '-static'" -a -installsuffix cgo -o prism-fusion ./main.go

# ========== 阶段3: 最终运行阶段 ==========
FROM ${REGISTRY}/library/alpine:3.23 AS final

# 设置时区
ENV TZ=Asia/Shanghai

# 更新镜像源并安装依赖
RUN sed -i 's/dl-cdn.alpinelinux.org/mirrors.aliyun.com/g' /etc/apk/repositories && \
    apk update && \
    apk add --no-cache \
        ca-certificates \
        tzdata \
        bash \
        supervisor \
        wget \
        netcat-openbsd \
        jq \
        curl && \
    ln -sf /usr/share/zoneinfo/${TZ} /etc/localtime && \
    echo ${TZ} > /etc/timezone && \
    rm -rf /var/cache/apk/*

# 创建应用用户和目录
RUN addgroup -S app && adduser -S app -G app && \
    mkdir -p /app/web /app/data && \
    chown -R app:app /app

# ========== 复制 Go 应用和前端资源 ==========
COPY --from=backend-builder --chown=app:app /app/server/prism-fusion /app/
COPY --from=backend-builder --chown=app:app /app/server/config.yaml /app/
COPY --from=frontend-builder --chown=app:app /app/admin/dist/ /app/web/

# ========== 配置 Supervisor ==========
COPY scripts/supervisord.conf /etc/supervisor/conf.d/supervisord.conf

# ========== 复制启动脚本 ==========
COPY --chown=app:app scripts/entrypoint.sh /app/entrypoint.sh

# 设置工作目录
WORKDIR /app

# 设置默认端口环境变量
ENV GATEWAY_PORT=8080

# 暴露端口 (由 GATEWAY_PORT 控制)
EXPOSE 8080

# 健康检查 - 使用 GATEWAY_PORT 环境变量
HEALTHCHECK --interval=30s --timeout=10s --start-period=30s --retries=3 \
    CMD wget -qO- http://localhost:${GATEWAY_PORT}/health | grep -q '"status"' || exit 1

# 使用 entrypoint 脚本启动
ENTRYPOINT ["/app/entrypoint.sh"]
