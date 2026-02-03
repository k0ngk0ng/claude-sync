APP_NAME := claude-sync
SERVER_NAME := claude-sync-server
VERSION := 1.0.0
BUILD_DIR := build

.PHONY: all build server clean dev install

# 默认: 构建所有
all: build-all

# 开发模式运行
dev:
	wails dev

# 构建客户端 (当前平台)
build:
	wails build -clean

# 构建服务端
server:
	go build -buildvcs=false -ldflags "-s -w" -o $(BUILD_DIR)/$(SERVER_NAME) ./cmd/server

# 构建所有平台
build-all: build-darwin build-linux build-windows server-all

# macOS 客户端
build-darwin:
	wails build -platform darwin/amd64 -o $(APP_NAME)-darwin-amd64
	wails build -platform darwin/arm64 -o $(APP_NAME)-darwin-arm64

# Linux 客户端
build-linux:
	wails build -platform linux/amd64 -o $(APP_NAME)-linux-amd64

# Windows 客户端
build-windows:
	wails build -platform windows/amd64 -o $(APP_NAME)-windows-amd64.exe

# 服务端所有平台
server-all:
	GOOS=darwin GOARCH=amd64 go build -buildvcs=false -ldflags "-s -w" -o $(BUILD_DIR)/$(SERVER_NAME)-darwin-amd64 ./cmd/server
	GOOS=darwin GOARCH=arm64 go build -buildvcs=false -ldflags "-s -w" -o $(BUILD_DIR)/$(SERVER_NAME)-darwin-arm64 ./cmd/server
	GOOS=linux GOARCH=amd64 go build -buildvcs=false -ldflags "-s -w" -o $(BUILD_DIR)/$(SERVER_NAME)-linux-amd64 ./cmd/server
	GOOS=linux GOARCH=arm64 go build -buildvcs=false -ldflags "-s -w" -o $(BUILD_DIR)/$(SERVER_NAME)-linux-arm64 ./cmd/server
	GOOS=windows GOARCH=amd64 go build -buildvcs=false -ldflags "-s -w" -o $(BUILD_DIR)/$(SERVER_NAME)-windows-amd64.exe ./cmd/server

# 清理
clean:
	rm -rf $(BUILD_DIR)
	rm -rf frontend/dist/wails*

# 安装依赖
deps:
	go mod tidy
	wails doctor

# 运行服务端 (开发)
run-server:
	go run ./cmd/server -port 8080 -token dev-token -data ./test-data

# 安装到系统
install: build server
	sudo cp $(BUILD_DIR)/bin/$(APP_NAME) /usr/local/bin/
	sudo cp $(BUILD_DIR)/$(SERVER_NAME) /usr/local/bin/
	@echo "✓ 已安装到 /usr/local/bin/"

# 帮助
help:
	@echo "Claude Sync 构建命令"
	@echo ""
	@echo "  make dev          - 开发模式运行 (热重载)"
	@echo "  make build        - 构建客户端 (当前平台)"
	@echo "  make server       - 构建服务端"
	@echo "  make build-all    - 构建所有平台"
	@echo "  make clean        - 清理构建文件"
	@echo "  make deps         - 安装依赖"
	@echo "  make run-server   - 运行服务端 (开发)"
	@echo "  make install      - 安装到系统"
