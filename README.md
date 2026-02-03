# Claude Sync

Claude Code 历史记录自动同步工具，支持多台机器之间无感同步。

## 特性

- 🔄 **自动同步**: 后台守护进程，定时自动同步
- 🖥️ **多平台**: 支持 macOS / Linux / Windows
- 🗺️ **路径映射**: 支持不同机器目录名不同的情况
- 🔒 **安全**: Token 认证，数据传输安全
- 📁 **增量同步**: 只同步变化的文件，节省带宽

## 快速开始

### 1. 编译

```bash
# 编译当前平台
make build

# 编译所有平台
make all

# 安装到系统
make install
```

### 2. 在公网服务器上启动服务

```bash
claude-sync server -port 8080 -token your-secret-token -data /data/claude-sync
```

建议使用 systemd 或 supervisor 管理服务进程。

### 3. 在本地机器配置并启动

**机器 A (如公司电脑):**
```bash
claude-sync config -server http://your-server:8080 -token your-secret-token -name "Work-Mac"
claude-sync start
```

**机器 B (如家里电脑):**
```bash
claude-sync config -server http://your-server:8080 -token your-secret-token -name "Home-Mac"
claude-sync start
```

### 4. 路径映射 (可选)

如果两台机器的项目目录不同，需要配置路径映射：

```bash
# 假设公司电脑项目在 /Users/work/projects
# 家里电脑项目在 /Users/home/dev

# 在家里电脑上配置:
claude-sync mapping -add "/Users/work/projects:/Users/home/dev"

# 查看所有映射
claude-sync mapping -list
```

## 命令参考

### 客户端命令

```bash
# 启动同步守护进程 (后台运行)
claude-sync start

# 前台运行 (调试用)
claude-sync start -f

# 停止守护进程
claude-sync stop

# 查看状态
claude-sync status

# 立即执行一次同步
claude-sync sync

# 配置
claude-sync config -server <url> -token <token> -name <name> -interval <seconds>
claude-sync config -show

# 路径映射
claude-sync mapping -add "remote_path:local_path"
claude-sync mapping -remove "remote_path"
claude-sync mapping -list
```

### 服务端命令

```bash
claude-sync server -port 8080 -token your-secret-token -data ./data
```

## 配置文件

配置保存在 `~/.claude/sync-config.json`:

```json
{
  "server_url": "http://your-server:8080",
  "token": "your-secret-token",
  "machine_id": "abc12345",
  "machine_name": "MacBook-Home",
  "sync_interval": 30,
  "path_mappings": {
    "/Users/work/projects": "/Users/home/dev"
  }
}
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
ExecStart=/usr/local/bin/claude-sync server -port 8080 -token YOUR_TOKEN -data /data/claude-sync
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

```dockerfile
FROM golang:1.21-alpine AS builder
WORKDIR /app
COPY . .
RUN go build -o claude-sync .

FROM alpine:latest
COPY --from=builder /app/claude-sync /usr/local/bin/
EXPOSE 8080
CMD ["claude-sync", "server", "-port", "8080", "-token", "${TOKEN}", "-data", "/data"]
```

```bash
docker run -d -p 8080:8080 -v /data/claude-sync:/data -e TOKEN=your-secret-token claude-sync
```

## 开机自启动 (客户端)

### macOS

创建 `~/Library/LaunchAgents/com.claude-sync.plist`:

```xml
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
    <key>Label</key>
    <string>com.claude-sync</string>
    <key>ProgramArguments</key>
    <array>
        <string>/usr/local/bin/claude-sync</string>
        <string>start</string>
        <string>-f</string>
    </array>
    <key>RunAtLoad</key>
    <true/>
    <key>KeepAlive</key>
    <true/>
</dict>
</plist>
```

```bash
launchctl load ~/Library/LaunchAgents/com.claude-sync.plist
```

### Linux

创建 `~/.config/systemd/user/claude-sync.service`:

```ini
[Unit]
Description=Claude Sync Client

[Service]
ExecStart=/usr/local/bin/claude-sync start -f
Restart=always

[Install]
WantedBy=default.target
```

```bash
systemctl --user enable claude-sync
systemctl --user start claude-sync
```

## License

MIT
