# Claude Sync

Claude Code 历史记录自动同步工具，像 Google Drive 一样无感同步。

![screenshot](docs/screenshot.png)

## 特性

- 🖥️ **桌面应用** - 系统托盘运行，类似 Google Drive / Dropbox
- 🔄 **自动同步** - 后台定时同步，无需手动操作
- 🗺️ **路径映射** - 支持不同机器目录名不同的情况
- 🔒 **安全** - Token 认证，数据传输安全
- 📁 **增量同步** - 只同步变化的文件，节省带宽
- 💻 **跨平台** - 支持 macOS / Linux / Windows

## 架构

```
┌─────────────────┐     ┌─────────────────┐     ┌─────────────────┐
│  Mac (公司)      │     │   公网服务器     │     │  Mac (家里)      │
│  Claude Sync    │────▶│  claude-sync    │◀────│  Claude Sync    │
│  (桌面应用)      │     │  server         │     │  (桌面应用)      │
└─────────────────┘     └─────────────────┘     └─────────────────┘
```

## 快速开始

### 1. 部署服务端

在公网服务器上运行：

```bash
# 下载服务端
wget https://github.com/k0ngk0ng/claude-sync/releases/latest/download/claude-sync-server-linux-amd64
chmod +x claude-sync-server-linux-amd64

# 启动服务
./claude-sync-server-linux-amd64 -port 8080 -token your-secret-token -data /data/claude-sync
```

### 2. 安装客户端

下载对应平台的客户端：

- **macOS**: `claude-sync-darwin-arm64.app` (Apple Silicon) / `claude-sync-darwin-amd64.app` (Intel)
- **Windows**: `claude-sync-windows-amd64.exe`
- **Linux**: `claude-sync-linux-amd64`

### 3. 配置

打开应用，点击设置，填写：

- **服务器地址**: `http://your-server:8080`
- **认证令牌**: `your-secret-token`
- **机器名称**: `MacBook-Home` (用于区分不同机器)

### 4. 路径映射 (可选)

如果两台机器的项目目录不同：

```
公司电脑: /Users/work/projects
家里电脑: /Users/home/dev
```

在家里电脑的设置中添加路径映射：
- 远程路径: `/Users/work/projects`
- 本地路径: `/Users/home/dev`

## 从源码构建

### 依赖

- Go 1.21+
- [Wails](https://wails.io/) v2

```bash
# 安装 Wails
go install github.com/wailsapp/wails/v2/cmd/wails@latest

# 检查环境
wails doctor
```

### 构建

```bash
# 克隆仓库
git clone https://github.com/k0ngk0ng/claude-sync.git
cd claude-sync

# 构建客户端 (当前平台)
make build

# 构建服务端
make server

# 构建所有平台
make build-all
```

### 开发

```bash
# 开发模式 (热重载)
make dev

# 运行服务端 (开发)
make run-server
```

## 服务端部署

### 使用 systemd

创建 `/etc/systemd/system/claude-sync.service`:

```ini
[Unit]
Description=Claude Sync Server
After=network.target

[Service]
Type=simple
User=claude-sync
ExecStart=/usr/local/bin/claude-sync-server -port 8080 -token YOUR_TOKEN -data /data/claude-sync
Restart=always
RestartSec=5

[Install]
WantedBy=multi-user.target
```

```bash
sudo systemctl enable claude-sync
sudo systemctl start claude-sync
```

### 使用 Docker

```bash
docker run -d \
  --name claude-sync \
  -p 8080:8080 \
  -v /data/claude-sync:/data \
  -e TOKEN=your-secret-token \
  ghcr.io/k0ngk0ng/claude-sync-server
```

## 项目结构

```
claude-sync/
├── main.go                 # 客户端入口 (Wails)
├── cmd/server/             # 服务端入口
├── internal/
│   ├── config/             # 配置管理
│   ├── service/            # 同步服务 & 服务端
│   └── tray/               # 系统托盘 (备用)
├── frontend/               # Web UI
│   └── dist/
├── build/                  # 构建产物
├── wails.json              # Wails 配置
└── Makefile
```

## License

MIT
