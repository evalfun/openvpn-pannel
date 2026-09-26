#!/bin/bash
go-bindata -o internal/assets/assets.go -pkg  assets -prefix "front/dist" front/dist/...
# 资源集内置资源（4 套），符号统一加 ResourceSet 前缀以避免与其它 bindata 冲突
go-bindata -o internal/ovpn_server/resource_sets_assets.go -pkg ovpnserver -prefix "internal/ovpn_server/resource_sets" internal/ovpn_server/resource_sets/...
bash scripts/rename_resource_set_assets.sh internal/ovpn_server/resource_sets_assets.go

# 动态编译 适用标准Linux发行版
# go build -ldflags "-X 'main.buildDate=$(date "+%Y-%m-%d %H:%M:%S")'" ./cmd/openvpn-pannel/ 

# 静态编译 使用标准Linux发行版和OpenWrt
CC=musl-gcc CGO_ENABLED=1 GOOS=linux GOARCH=amd64 go build -ldflags="-linkmode external -extldflags -static -X 'main.buildDate=$(date "+%Y-%m-%d %H:%M:%S")'"  -o openvpn-pannel ./cmd/openvpn-pannel