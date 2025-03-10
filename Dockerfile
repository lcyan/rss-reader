FROM golang:1.24.0-alpine3.21 AS builder

COPY . /src
WORKDIR /src

#国内服务器可以取消以下注释
#RUN go env -w GO111MODULE=on && \
#    go env -w GOPROXY=https://goproxy.cn,direct

RUN go build -ldflags "-s -w" -o ./bin/ .

FROM alpine

COPY --from=builder /src/bin /app
COPY --from=builder /src/config.json /app/config.json

WORKDIR /app

EXPOSE 8080

# 设置时区
RUN apk add --no-cache tzdata
ENV TZ=Asia/Shanghai

ENTRYPOINT ["./rss-reader"]
