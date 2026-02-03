APP_NAME := claude-sync
VERSION := 1.0.0
BUILD_DIR := build

# Go 参数
GOCMD := go
GOBUILD := $(GOCMD) build
GOCLEAN := $(GOCMD) clean
GOTEST := $(GOCMD) test
GOGET := $(GOCMD) get

# 编译标志
LDFLAGS := -buildvcs=false -ldflags "-s -w -X main.appVersion=$(VERSION)"

.PHONY: all build clean test install uninstall darwin linux windows

# 默认目标
all: darwin linux windows

# 当前平台编译
build:
	$(GOBUILD) $(LDFLAGS) -o $(BUILD_DIR)/$(APP_NAME) .

# macOS (Intel + Apple Silicon)
darwin:
	GOOS=darwin GOARCH=amd64 $(GOBUILD) $(LDFLAGS) -o $(BUILD_DIR)/$(APP_NAME)-darwin-amd64 .
	GOOS=darwin GOARCH=arm64 $(GOBUILD) $(LDFLAGS) -o $(BUILD_DIR)/$(APP_NAME)-darwin-arm64 .

# Linux
linux:
	GOOS=linux GOARCH=amd64 $(GOBUILD) $(LDFLAGS) -o $(BUILD_DIR)/$(APP_NAME)-linux-amd64 .
	GOOS=linux GOARCH=arm64 $(GOBUILD) $(LDFLAGS) -o $(BUILD_DIR)/$(APP_NAME)-linux-arm64 .

# Windows
windows:
	GOOS=windows GOARCH=amd64 $(GOBUILD) $(LDFLAGS) -o $(BUILD_DIR)/$(APP_NAME)-windows-amd64.exe .

# 清理
clean:
	$(GOCLEAN)
	rm -rf $(BUILD_DIR)

# 测试
test:
	$(GOTEST) -v ./...

# 安装到系统 (macOS/Linux)
install: build
	sudo cp $(BUILD_DIR)/$(APP_NAME) /usr/local/bin/
	@echo "✓ 已安装到 /usr/local/bin/$(APP_NAME)"

# 卸载
uninstall:
	sudo rm -f /usr/local/bin/$(APP_NAME)
	@echo "✓ 已卸载"

# 运行服务端 (开发用)
run-server:
	$(GOBUILD) -o $(BUILD_DIR)/$(APP_NAME) . && $(BUILD_DIR)/$(APP_NAME) server -port 8080 -token dev-token

# 运行客户端 (开发用)
run-client:
	$(GOBUILD) -o $(BUILD_DIR)/$(APP_NAME) . && $(BUILD_DIR)/$(APP_NAME) start -f
