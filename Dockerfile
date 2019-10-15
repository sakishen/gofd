FROM golang:1.13.0 AS builder
ENV GOPROXY https://goproxy.cn
ENV GO111MODULE on

WORKDIR /go/cache

ADD go.mod .
ADD go.sum .
# 执行的时候，会构建一层缓存，包含了该项所有的依赖
RUN go mod download

WORKDIR /opt/
ADD . .

# -s: 省略符号表和调试信息
# -w: 省略DWARF符号表
RUN GOOS=linux CGO_ENABLED=0 go build -ldflags="-s -w" -installsuffix cgo -o app cmd/main.go
RUN rm /etc/localtime && ln -s /usr/share/zoneinfo/Asia/Shanghai /etc/localtime

FROM alpine:latest
RUN apk --no-cache add ca-certificates bash && mkdir -p /opt/download && mkdir -p /opt/config
WORKDIR /opt/
COPY --from=builder /usr/share/zoneinfo /usr/share/zoneinfo
COPY --from=builder /opt/app .
#COPY --from=builder /opt/config ./config
COPY --from=builder /opt/misc ./misc
VOLUME ["/opt/area_game_data/"]
VOLUME ["/opt/download/"]
VOLUME ["/opt/config/"]
# docker build  -t vrviu/gofd:latest .
# docker run -p 45010:45010 -p 45011:45011 -p 45000:45000 -p 45001:45001 -it --rm -v /Users/sakishum/data/saki/script/go/gofd/download/:/opt/download -v /Users/sakishum/data/saki/script/go/gofd/config:/opt/config vrviu/gofd sh
ENTRYPOINT ["/opt/app", "-s", "/opt/config/server.yml"]
# docker build  -t vrviu/gofd_agent:latest .
# docker image save -o gofd_agent_img.tar vrviu/gofd_agent
# scp gofd_agent_img.tar user_00@10.86.0.108:~
# docker image load -i gofd_agent_img.tar
# docker run -p 45010:45010 -p 45011:45011 -it --rm -v /data/saki/download/:/opt/download -v /data/saki/conf/:/opt/config vrviu/gofd_agent sh
#ENTRYPOINT ["/opt/app", "-a", "/opt/config/agent.yml"]

# curl  -l --insecure --basic -u "vrviu@sz:vrviu" -H "Content-type: application/json" -X POST -d '{"id":"1","dispatchFiles":["/opt/download/gofd_agent_img.tar"],"destIPs":["10.86.0.108"]}' https://10.86.1.21:45000/api/v1/server/tasks
# curl  -l --insecure --basic -u "vrviu@sz:vrviu" -H "Content-type: application/json" -X GET https://10.86.1.21:45000/api/v1/server/tasks/1