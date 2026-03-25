# 构建阶段
FROM golang:1.21-alpine AS builder

# 设置代理 (如果在国内环境，建议开启)
# ENV GOPROXY=https://goproxy.cn,direct

ARG TARGETARCH
ARG TARGETOS

WORKDIR /tgfilebot

# 复制依赖文件并下载，利用 Docker 缓存镜像层
COPY go.mod go.sum ./
RUN go mod download

# 复制源代码
COPY . .

RUN CGO_ENABLED=0 \
    GOOS=${TARGETOS} \
    GOARCH=${TARGETARCH} \
    go build -ldflags "-s -w" \
    -tags netgo \
    -installsuffix netgo \
    -o TGBot .

# 运行阶段
FROM alpine:3.20

WORKDIR /root/

RUN apk --no-cache add ca-certificates tzdata

# 复制编译产物
COPY --from=builder /tgfilebot/TGBot .

# 确保配置文件和目录存在
RUN mkdir -p files

EXPOSE 8080

CMD ["./TGBot", "-files", "./files"]

ENV TZ=Asia/Shanghai