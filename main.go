package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"syscall"
	"time"
)

const (
	appName    = "claude-sync"
	appVersion = "1.0.0"
)

// Config 配置
type Config struct {
	ServerURL    string            `json:"server_url"`
	Token        string            `json:"token"`
	MachineID    string            `json:"machine_id"`
	MachineName  string            `json:"machine_name"`
	SyncInterval int               `json:"sync_interval"`
	PathMappings map[string]string `json:"path_mappings"` // remote -> local 路径映射
}

// FileInfo 文件信息
type FileInfo struct {
	Path     string `json:"path"`
	Hash     string `json:"hash"`
	ModTime  int64  `json:"mod_time"`
	Size     int64  `json:"size"`
	Content  []byte `json:"content,omitempty"`
}

// SyncRequest 同步请求
type SyncRequest struct {
	MachineID   string     `json:"machine_id"`
	MachineName string     `json:"machine_name"`
	Files       []FileInfo `json:"files"`
}

// SyncResponse 同步响应
type SyncResponse struct {
	Success bool       `json:"success"`
	Message string     `json:"message"`
	Files   []FileInfo `json:"files"`
}

// Daemon 守护进程
type Daemon struct {
	config     *Config
	claudeDir  string
	fileHashes map[string]string
	mu         sync.RWMutex
	stopChan   chan struct{}
}

func main() {
	if len(os.Args) < 2 {
		printUsage()
		os.Exit(1)
	}

	switch os.Args[1] {
	case "start":
		startCmd := flag.NewFlagSet("start", flag.ExitOnError)
		foreground := startCmd.Bool("f", false, "前台运行")
		startCmd.Parse(os.Args[2:])
		runStart(*foreground)

	case "stop":
		runStop()

	case "config":
		configCmd := flag.NewFlagSet("config", flag.ExitOnError)
		serverURL := configCmd.String("server", "", "服务器地址")
		token := configCmd.String("token", "", "认证令牌")
		machineName := configCmd.String("name", "", "机器名称")
		interval := configCmd.Int("interval", 0, "同步间隔(秒)")
		show := configCmd.Bool("show", false, "显示配置")
		configCmd.Parse(os.Args[2:])
		runConfig(*serverURL, *token, *machineName, *interval, *show)

	case "mapping":
		mappingCmd := flag.NewFlagSet("mapping", flag.ExitOnError)
		add := mappingCmd.String("add", "", "添加映射 (格式: remote_path:local_path)")
		remove := mappingCmd.String("remove", "", "删除映射 (remote_path)")
		list := mappingCmd.Bool("list", false, "列出所有映射")
		mappingCmd.Parse(os.Args[2:])
		runMapping(*add, *remove, *list)

	case "status":
		runStatus()

	case "sync":
		runSyncOnce()

	case "server":
		serverCmd := flag.NewFlagSet("server", flag.ExitOnError)
		port := serverCmd.Int("port", 8080, "监听端口")
		dataDir := serverCmd.String("data", "./claude-sync-data", "数据目录")
		token := serverCmd.String("token", "", "认证令牌 (必填)")
		serverCmd.Parse(os.Args[2:])
		runServer(*port, *dataDir, *token)

	case "version":
		fmt.Printf("%s version %s (%s/%s)\n", appName, appVersion, runtime.GOOS, runtime.GOARCH)

	case "help", "-h", "--help":
		printUsage()

	default:
		fmt.Printf("未知命令: %s\n", os.Args[1])
		printUsage()
		os.Exit(1)
	}
}

func printUsage() {
	fmt.Printf(`%s - Claude Code 历史记录自动同步工具

用法:
  %s <command> [options]

客户端命令:
  start     启动同步守护进程
    -f      前台运行 (默认后台)

  stop      停止守护进程

  config    配置同步设置
    -server <url>     服务器地址
    -token <token>    认证令牌
    -name <name>      机器名称
    -interval <sec>   同步间隔(秒), 默认30
    -show             显示当前配置

  mapping   管理路径映射 (用于不同机器目录名不同的情况)
    -add <remote:local>   添加映射
    -remove <remote>      删除映射
    -list                 列出所有映射

  status    查看同步状态
  sync      立即执行一次同步

服务端命令:
  server    启动同步服务器
    -port <port>      监听端口 (默认: 8080)
    -data <dir>       数据目录 (默认: ./claude-sync-data)
    -token <token>    认证令牌 (必填)

示例:
  # 1. 在公网服务器上启动服务
  %s server -port 8080 -token your-secret-token

  # 2. 在本地配置并启动
  %s config -server http://your-server:8080 -token your-secret-token -name "MacBook-Home"
  %s start

  # 3. 路径映射 (当两台机器目录不同时)
  # 假设公司电脑项目在 /Users/work/projects, 家里在 /Users/home/dev
  # 在家里的电脑上配置:
  %s mapping -add "/Users/work/projects:/Users/home/dev"

`, appName, appName, appName, appName, appName, appName)
}

// ==================== 配置相关 ====================

func getClaudeDir() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".claude")
}

func getConfigPath() string {
	return filepath.Join(getClaudeDir(), "sync-config.json")
}

func getPidPath() string {
	return filepath.Join(getClaudeDir(), "sync.pid")
}

func loadConfig() (*Config, error) {
	data, err := os.ReadFile(getConfigPath())
	if err != nil {
		if os.IsNotExist(err) {
			return &Config{
				MachineID:    generateMachineID(),
				SyncInterval: 30,
				PathMappings: make(map[string]string),
			}, nil
		}
		return nil, err
	}

	var config Config
	if err := json.Unmarshal(data, &config); err != nil {
		return nil, err
	}

	if config.MachineID == "" {
		config.MachineID = generateMachineID()
	}
	if config.SyncInterval == 0 {
		config.SyncInterval = 30
	}
	if config.PathMappings == nil {
		config.PathMappings = make(map[string]string)
	}

	return &config, nil
}

func saveConfig(config *Config) error {
	data, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(getConfigPath(), data, 0600)
}

func generateMachineID() string {
	hostname, _ := os.Hostname()
	data := fmt.Sprintf("%s-%s-%d", hostname, runtime.GOOS, time.Now().UnixNano())
	hash := sha256.Sum256([]byte(data))
	return hex.EncodeToString(hash[:8])
}

// ==================== 路径映射 ====================

func runMapping(add, remove string, list bool) {
	config, err := loadConfig()
	if err != nil {
		fmt.Printf("错误: %v\n", err)
		os.Exit(1)
	}

	if add != "" {
		parts := strings.SplitN(add, ":", 2)
		if len(parts) != 2 {
			fmt.Println("错误: 格式应为 remote_path:local_path")
			os.Exit(1)
		}
		remotePath := strings.TrimSpace(parts[0])
		localPath := strings.TrimSpace(parts[1])

		// 展开本地路径
		if strings.HasPrefix(localPath, "~") {
			home, _ := os.UserHomeDir()
			localPath = filepath.Join(home, localPath[1:])
		}

		config.PathMappings[remotePath] = localPath
		if err := saveConfig(config); err != nil {
			fmt.Printf("错误: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("✓ 已添加映射:\n  远程: %s\n  本地: %s\n", remotePath, localPath)
		return
	}

	if remove != "" {
		if _, exists := config.PathMappings[remove]; !exists {
			fmt.Printf("映射不存在: %s\n", remove)
			os.Exit(1)
		}
		delete(config.PathMappings, remove)
		if err := saveConfig(config); err != nil {
			fmt.Printf("错误: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("✓ 已删除映射: %s\n", remove)
		return
	}

	// 默认列出所有映射
	fmt.Println("路径映射:")
	if len(config.PathMappings) == 0 {
		fmt.Println("  (无映射)")
	} else {
		for remote, local := range config.PathMappings {
			fmt.Printf("  %s -> %s\n", remote, local)
		}
	}
}

// applyPathMapping 应用路径映射 (远程路径 -> 本地路径)
func applyPathMapping(path string, mappings map[string]string) string {
	for remote, local := range mappings {
		if strings.Contains(path, remote) {
			return strings.Replace(path, remote, local, 1)
		}
	}
	return path
}

// reversePathMapping 反向路径映射 (本地路径 -> 远程路径)
func reversePathMapping(path string, mappings map[string]string) string {
	for remote, local := range mappings {
		if strings.Contains(path, local) {
			return strings.Replace(path, local, remote, 1)
		}
	}
	return path
}

// ==================== 客户端命令 ====================

func runConfig(serverURL, token, machineName string, interval int, show bool) {
	config, err := loadConfig()
	if err != nil {
		fmt.Printf("错误: %v\n", err)
		os.Exit(1)
	}

	if show || (serverURL == "" && token == "" && machineName == "" && interval == 0) {
		fmt.Println("当前配置:")
		fmt.Printf("  服务器:   %s\n", valueOrDefault(config.ServerURL, "(未设置)"))
		fmt.Printf("  令牌:     %s\n", maskToken(config.Token))
		fmt.Printf("  机器名称: %s\n", valueOrDefault(config.MachineName, "(未设置)"))
		fmt.Printf("  机器ID:   %s\n", config.MachineID)
		fmt.Printf("  同步间隔: %d 秒\n", config.SyncInterval)
		fmt.Printf("  路径映射: %d 条\n", len(config.PathMappings))
		return
	}

	if serverURL != "" {
		config.ServerURL = strings.TrimSuffix(serverURL, "/")
		fmt.Printf("✓ 服务器已设置: %s\n", config.ServerURL)
	}
	if token != "" {
		config.Token = token
		fmt.Printf("✓ 令牌已设置\n")
	}
	if machineName != "" {
		config.MachineName = machineName
		fmt.Printf("✓ 机器名称已设置: %s\n", machineName)
	}
	if interval > 0 {
		config.SyncInterval = interval
		fmt.Printf("✓ 同步间隔已设置: %d 秒\n", interval)
	}

	if err := saveConfig(config); err != nil {
		fmt.Printf("错误: %v\n", err)
		os.Exit(1)
	}
}

func runStart(foreground bool) {
	config, err := loadConfig()
	if err != nil {
		fmt.Printf("错误: %v\n", err)
		os.Exit(1)
	}

	if config.ServerURL == "" {
		fmt.Println("错误: 未配置服务器地址，请先运行: claude-sync config -server <url>")
		os.Exit(1)
	}

	if pid := readPid(); pid > 0 {
		if processExists(pid) {
			fmt.Printf("守护进程已在运行 (PID: %d)\n", pid)
			os.Exit(1)
		}
	}

	if !foreground {
		fmt.Println("正在后台启动同步守护进程...")
		daemonize()
		return
	}

	daemon := NewDaemon(config)
	daemon.Run()
}

func runStop() {
	pid := readPid()
	if pid <= 0 {
		fmt.Println("守护进程未运行")
		return
	}

	process, err := os.FindProcess(pid)
	if err != nil {
		fmt.Println("守护进程未运行")
		os.Remove(getPidPath())
		return
	}

	if err := process.Signal(syscall.SIGTERM); err != nil {
		fmt.Printf("停止失败: %v\n", err)
		os.Exit(1)
	}

	os.Remove(getPidPath())
	fmt.Println("✓ 守护进程已停止")
}

func runStatus() {
	config, _ := loadConfig()

	fmt.Println("Claude Sync 状态")
	fmt.Println(strings.Repeat("=", 40))

	pid := readPid()
	if pid > 0 && processExists(pid) {
		fmt.Printf("守护进程: 运行中 (PID: %d)\n", pid)
	} else {
		fmt.Println("守护进程: 未运行")
	}

	fmt.Printf("服务器:   %s\n", valueOrDefault(config.ServerURL, "(未设置)"))
	fmt.Printf("机器名称: %s\n", valueOrDefault(config.MachineName, "(未设置)"))
	fmt.Printf("路径映射: %d 条\n", len(config.PathMappings))

	projectsDir := filepath.Join(getClaudeDir(), "projects")
	if info, err := os.Stat(projectsDir); err == nil {
		var fileCount int
		filepath.Walk(projectsDir, func(path string, info os.FileInfo, err error) error {
			if err == nil && !info.IsDir() {
				fileCount++
			}
			return nil
		})
		fmt.Printf("本地文件: %d 个\n", fileCount)
		fmt.Printf("最后修改: %s\n", info.ModTime().Format("2006-01-02 15:04:05"))
	}

	if config.ServerURL != "" {
		fmt.Print("服务器连接: ")
		client := &http.Client{Timeout: 5 * time.Second}
		resp, err := client.Get(config.ServerURL + "/health")
		if err != nil {
			fmt.Println("失败 -", err)
		} else {
			resp.Body.Close()
			if resp.StatusCode == 200 {
				fmt.Println("正常")
			} else {
				fmt.Printf("异常 (HTTP %d)\n", resp.StatusCode)
			}
		}
	}
}

func runSyncOnce() {
	config, err := loadConfig()
	if err != nil {
		fmt.Printf("错误: %v\n", err)
		os.Exit(1)
	}

	if config.ServerURL == "" {
		fmt.Println("错误: 未配置服务器地址")
		os.Exit(1)
	}

	daemon := NewDaemon(config)
	fmt.Println("正在同步...")
	if err := daemon.syncOnce(); err != nil {
		fmt.Printf("同步失败: %v\n", err)
		os.Exit(1)
	}
	fmt.Println("✓ 同步完成")
}

// ==================== 守护进程 ====================

func NewDaemon(config *Config) *Daemon {
	return &Daemon{
		config:     config,
		claudeDir:  getClaudeDir(),
		fileHashes: make(map[string]string),
		stopChan:   make(chan struct{}),
	}
}

func (d *Daemon) Run() {
	writePid(os.Getpid())
	defer os.Remove(getPidPath())

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	fmt.Printf("Claude Sync 守护进程已启动 (PID: %d)\n", os.Getpid())
	fmt.Printf("服务器: %s\n", d.config.ServerURL)
	fmt.Printf("同步间隔: %d 秒\n", d.config.SyncInterval)

	d.syncOnce()

	ticker := time.NewTicker(time.Duration(d.config.SyncInterval) * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			if err := d.syncOnce(); err != nil {
				fmt.Printf("[%s] 同步错误: %v\n", time.Now().Format("15:04:05"), err)
			}
		case <-sigChan:
			fmt.Println("\n正在停止...")
			return
		case <-d.stopChan:
			return
		}
	}
}

func (d *Daemon) syncOnce() error {
	localFiles, err := d.scanLocalFiles()
	if err != nil {
		return fmt.Errorf("扫描本地文件失败: %w", err)
	}

	req := SyncRequest{
		MachineID:   d.config.MachineID,
		MachineName: d.config.MachineName,
		Files:       localFiles,
	}

	respFiles, err := d.sendSyncRequest(req)
	if err != nil {
		return fmt.Errorf("同步请求失败: %w", err)
	}

	var updated int
	for _, f := range respFiles {
		if len(f.Content) > 0 {
			// 应用路径映射
			localPath := applyPathMapping(f.Path, d.config.PathMappings)
			destPath := filepath.Join(d.claudeDir, localPath)

			// 同时替换文件内容中的路径
			content := d.applyContentPathMapping(f.Content)

			if err := os.MkdirAll(filepath.Dir(destPath), 0755); err != nil {
				continue
			}
			if err := os.WriteFile(destPath, content, 0644); err != nil {
				continue
			}
			updated++
		}
	}

	if updated > 0 {
		fmt.Printf("[%s] 同步完成: 更新了 %d 个文件\n", time.Now().Format("15:04:05"), updated)
	}

	return nil
}

// applyContentPathMapping 替换文件内容中的路径
func (d *Daemon) applyContentPathMapping(content []byte) []byte {
	result := string(content)
	for remote, local := range d.config.PathMappings {
		result = strings.ReplaceAll(result, remote, local)
	}
	return []byte(result)
}

// reverseContentPathMapping 反向替换文件内容中的路径 (上传时用)
func (d *Daemon) reverseContentPathMapping(content []byte) []byte {
	result := string(content)
	for remote, local := range d.config.PathMappings {
		result = strings.ReplaceAll(result, local, remote)
	}
	return []byte(result)
}

func (d *Daemon) scanLocalFiles() ([]FileInfo, error) {
	var files []FileInfo
	projectsDir := filepath.Join(d.claudeDir, "projects")

	err := filepath.Walk(projectsDir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return nil
		}

		relPath, _ := filepath.Rel(d.claudeDir, path)

		data, err := os.ReadFile(path)
		if err != nil {
			return nil
		}

		hash := sha256.Sum256(data)
		hashStr := hex.EncodeToString(hash[:])

		d.mu.RLock()
		oldHash := d.fileHashes[relPath]
		d.mu.RUnlock()

		// 上传时使用反向映射的路径
		remotePath := reversePathMapping(relPath, d.config.PathMappings)

		fileInfo := FileInfo{
			Path:    remotePath,
			Hash:    hashStr,
			ModTime: info.ModTime().Unix(),
			Size:    info.Size(),
		}

		if oldHash != hashStr {
			// 上传时反向替换内容中的路径
			fileInfo.Content = d.reverseContentPathMapping(data)
			d.mu.Lock()
			d.fileHashes[relPath] = hashStr
			d.mu.Unlock()
		}

		files = append(files, fileInfo)
		return nil
	})

	return files, err
}

func (d *Daemon) sendSyncRequest(req SyncRequest) ([]FileInfo, error) {
	data, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}

	httpReq, err := http.NewRequest("POST", d.config.ServerURL+"/sync", bytes.NewReader(data))
	if err != nil {
		return nil, err
	}

	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+d.config.Token)

	client := &http.Client{Timeout: 60 * time.Second}
	resp, err := client.Do(httpReq)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(body))
	}

	var syncResp SyncResponse
	if err := json.NewDecoder(resp.Body).Decode(&syncResp); err != nil {
		return nil, err
	}

	if !syncResp.Success {
		return nil, fmt.Errorf(syncResp.Message)
	}

	return syncResp.Files, nil
}

// ==================== 服务端 ====================

type Server struct {
	dataDir string
	token   string
	mu      sync.RWMutex
	files   map[string]FileInfo
}

func runServer(port int, dataDir, token string) {
	if token == "" {
		fmt.Println("错误: 必须指定认证令牌 (-token)")
		os.Exit(1)
	}

	if err := os.MkdirAll(dataDir, 0755); err != nil {
		fmt.Printf("错误: 无法创建数据目录: %v\n", err)
		os.Exit(1)
	}

	server := &Server{
		dataDir: dataDir,
		token:   token,
		files:   make(map[string]FileInfo),
	}

	server.loadData()

	http.HandleFunc("/health", server.handleHealth)
	http.HandleFunc("/sync", server.authMiddleware(server.handleSync))

	fmt.Printf("Claude Sync 服务器启动\n")
	fmt.Printf("监听端口: %d\n", port)
	fmt.Printf("数据目录: %s\n", dataDir)
	fmt.Println("等待客户端连接...")

	if err := http.ListenAndServe(fmt.Sprintf(":%d", port), nil); err != nil {
		fmt.Printf("服务器错误: %v\n", err)
		os.Exit(1)
	}
}

func (s *Server) authMiddleware(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		auth := r.Header.Get("Authorization")
		if auth != "Bearer "+s.token {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}
		next(w, r)
	}
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("OK"))
}

func (s *Server) handleSync(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req SyncRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	fmt.Printf("[%s] 同步请求来自: %s (%s), 文件数: %d\n",
		time.Now().Format("15:04:05"),
		req.MachineName, req.MachineID, len(req.Files))

	s.mu.Lock()
	defer s.mu.Unlock()

	var filesToSend []FileInfo

	for _, f := range req.Files {
		existing, exists := s.files[f.Path]

		if len(f.Content) > 0 {
			if !exists || f.ModTime > existing.ModTime {
				s.files[f.Path] = f
				s.saveFile(f)
			}
		}

		if exists && existing.Hash != f.Hash && existing.ModTime > f.ModTime {
			content, err := s.readFile(existing.Path)
			if err == nil {
				filesToSend = append(filesToSend, FileInfo{
					Path:    existing.Path,
					Hash:    existing.Hash,
					ModTime: existing.ModTime,
					Content: content,
				})
			}
		}
	}

	clientFiles := make(map[string]bool)
	for _, f := range req.Files {
		clientFiles[f.Path] = true
	}

	for path, f := range s.files {
		if !clientFiles[path] {
			content, err := s.readFile(path)
			if err == nil {
				filesToSend = append(filesToSend, FileInfo{
					Path:    f.Path,
					Hash:    f.Hash,
					ModTime: f.ModTime,
					Content: content,
				})
			}
		}
	}

	resp := SyncResponse{
		Success: true,
		Message: "OK",
		Files:   filesToSend,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

func (s *Server) saveFile(f FileInfo) error {
	path := filepath.Join(s.dataDir, f.Path)
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	return os.WriteFile(path, f.Content, 0644)
}

func (s *Server) readFile(relPath string) ([]byte, error) {
	return os.ReadFile(filepath.Join(s.dataDir, relPath))
}

func (s *Server) loadData() {
	filepath.Walk(s.dataDir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return nil
		}

		relPath, _ := filepath.Rel(s.dataDir, path)
		data, err := os.ReadFile(path)
		if err != nil {
			return nil
		}

		hash := sha256.Sum256(data)
		s.files[relPath] = FileInfo{
			Path:    relPath,
			Hash:    hex.EncodeToString(hash[:]),
			ModTime: info.ModTime().Unix(),
			Size:    info.Size(),
		}
		return nil
	})

	fmt.Printf("已加载 %d 个文件\n", len(s.files))
}

// ==================== 辅助函数 ====================

func readPid() int {
	data, err := os.ReadFile(getPidPath())
	if err != nil {
		return 0
	}
	var pid int
	fmt.Sscanf(string(data), "%d", &pid)
	return pid
}

func writePid(pid int) {
	os.WriteFile(getPidPath(), []byte(fmt.Sprintf("%d", pid)), 0644)
}

func processExists(pid int) bool {
	process, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	err = process.Signal(syscall.Signal(0))
	return err == nil
}

func valueOrDefault(value, defaultValue string) string {
	if value == "" {
		return defaultValue
	}
	return value
}

func maskToken(token string) string {
	if token == "" {
		return "(未设置)"
	}
	if len(token) <= 8 {
		return "****"
	}
	return token[:4] + "****" + token[len(token)-4:]
}

func daemonize() {
	executable, _ := os.Executable()
	args := append([]string{executable, "start", "-f"}, os.Args[2:]...)

	attr := &os.ProcAttr{
		Dir:   ".",
		Env:   os.Environ(),
		Files: []*os.File{nil, nil, nil},
	}

	process, err := os.StartProcess(executable, args, attr)
	if err != nil {
		fmt.Printf("启动失败: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("✓ 守护进程已启动 (PID: %d)\n", process.Pid)
	process.Release()
}
