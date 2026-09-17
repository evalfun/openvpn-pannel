BINARY       := openvpn-pannel
CMD          := ./cmd/openvpn-pannel
FRONT_DIR    := front
DIST_DIR     := $(FRONT_DIR)/dist
ASSETS_GO    := internal/assets/assets.go
OVPN_ASSETS  := internal/ovpn_server/assets.go
OVPN_RES     := internal/ovpn_server/default_resource

BUILD_DATE   := $(shell date "+%Y-%m-%d %H:%M:%S")
LDFLAGS      := -X 'main.buildDate=$(BUILD_DATE)'

.PHONY: all build static dynamic front assets assets-ovpn clean distclean run lint fmt vet

all: build

## build: 静态编译（musl, 适用于标准 Linux 发行版和 OpenWrt）
build: static

## static: 静态编译
static: front assets
	CC=musl-gcc CGO_ENABLED=1 GOOS=linux GOARCH=amd64 \
		go build -ldflags="-linkmode external -extldflags -static $(LDFLAGS)" \
		-o $(BINARY) $(CMD)

## dynamic: 动态编译（适用于标准 Linux 发行版）
dynamic: front assets
	go build -ldflags "$(LDFLAGS)" -o $(BINARY) $(CMD)

## front: 编译前端，输出到 front/dist
front:
	cd $(FRONT_DIR) && npm install && npm run build

## assets: 将前端产物和 OpenVPN 默认资源嵌入为 Go 代码
assets: $(ASSETS_GO) $(OVPN_ASSETS)

$(ASSETS_GO): $(DIST_DIR)
	go-bindata -o $(ASSETS_GO) -pkg assets -prefix "$(DIST_DIR)" $(DIST_DIR)/...

$(OVPN_ASSETS): $(OVPN_RES)
	go-bindata -o $(OVPN_ASSETS) -pkg ovpnserver -prefix "$(OVPN_RES)" $(OVPN_RES)/...

## run: 编译并运行
run: build
	./$(BINARY)

## lint: 运行前端 ESLint 和 Go vet
lint:
	cd $(FRONT_DIR) && npm run lint
	go vet ./...

## vet: Go 静态检查
vet:
	go vet ./...

## fmt: 格式化 Go 代码
fmt:
	go fmt ./...

## clean: 删除编译产物
clean:
	rm -f $(BINARY)

## distclean: 删除编译产物、嵌入资源和前端产物
distclean: clean
	rm -f $(ASSETS_GO) $(OVPN_ASSETS)
	rm -rf $(DIST_DIR)
