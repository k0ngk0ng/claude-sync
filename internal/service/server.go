package service

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// Server 同步服务器 (多租户)
type Server struct {
	dataDir  string
	port     int
	mu       sync.RWMutex
	tenants  map[string]*Tenant // token -> Tenant
	configPath string
}

// Tenant 租户
type Tenant struct {
	ID          string                 `json:"id"`
	Name        string                 `json:"name"`
	Token       string                 `json:"token"`
	CreatedAt   time.Time              `json:"created_at"`
	LastActive  time.Time              `json:"last_active"`
	Files       map[string]FileInfo    `json:"-"` // 内存中的文件索引
	Clients     map[string]*ClientInfo `json:"-"` // 连接的客户端
}

// ClientInfo 客户端信息
type ClientInfo struct {
	MachineID   string    `json:"machine_id"`
	MachineName string    `json:"machine_name"`
	LastSeen    time.Time `json:"last_seen"`
	FileCount   int       `json:"file_count"`
	IP          string    `json:"ip"`
}

// ServerConfig 服务器配置
type ServerConfig struct {
	AdminToken string    `json:"admin_token"`
	Tenants    []*Tenant `json:"tenants"`
}

// ServerStats 服务器统计
type ServerStats struct {
	TotalTenants int            `json:"total_tenants"`
	TotalFiles   int            `json:"total_files"`
	TotalSize    int64          `json:"total_size"`
	Tenants      []*TenantStats `json:"tenants"`
}

// TenantStats 租户统计
type TenantStats struct {
	ID          string        `json:"id"`
	Name        string        `json:"name"`
	FileCount   int           `json:"file_count"`
	TotalSize   int64         `json:"total_size"`
	ClientCount int           `json:"client_count"`
	Clients     []*ClientInfo `json:"clients"`
	LastActive  time.Time     `json:"last_active"`
}

// NewServer 创建服务器
func NewServer(port int, dataDir, adminToken string) *Server {
	s := &Server{
		dataDir:    dataDir,
		port:       port,
		tenants:    make(map[string]*Tenant),
		configPath: filepath.Join(dataDir, "config.json"),
	}

	// 加载或创建配置
	s.loadConfig(adminToken)

	return s
}

// loadConfig 加载配置
func (s *Server) loadConfig(adminToken string) {
	// 确保数据目录存在
	os.MkdirAll(s.dataDir, 0755)

	data, err := os.ReadFile(s.configPath)
	if err == nil {
		var config ServerConfig
		if json.Unmarshal(data, &config) == nil {
			for _, t := range config.Tenants {
				t.Files = make(map[string]FileInfo)
				t.Clients = make(map[string]*ClientInfo)
				s.tenants[t.Token] = t
				// 加载租户数据
				s.loadTenantData(t)
			}
		}
	}

	// 如果提供了 adminToken 且没有租户，创建默认租户
	if adminToken != "" && len(s.tenants) == 0 {
		s.CreateTenant("default", "Default User", adminToken)
	}
}

// saveConfig 保存配置
func (s *Server) saveConfig() error {
	s.mu.RLock()
	tenants := make([]*Tenant, 0, len(s.tenants))
	for _, t := range s.tenants {
		tenants = append(tenants, t)
	}
	s.mu.RUnlock()

	config := ServerConfig{
		Tenants: tenants,
	}

	data, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(s.configPath, data, 0600)
}

// CreateTenant 创建租户
func (s *Server) CreateTenant(id, name, token string) (*Tenant, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	// 检查 token 是否已存在
	if _, exists := s.tenants[token]; exists {
		return nil, fmt.Errorf("token already exists")
	}

	// 检查 ID 是否已存在
	for _, t := range s.tenants {
		if t.ID == id {
			return nil, fmt.Errorf("tenant ID already exists")
		}
	}

	tenant := &Tenant{
		ID:        id,
		Name:      name,
		Token:     token,
		CreatedAt: time.Now(),
		Files:     make(map[string]FileInfo),
		Clients:   make(map[string]*ClientInfo),
	}

	// 创建租户数据目录
	tenantDir := filepath.Join(s.dataDir, "tenants", id)
	os.MkdirAll(tenantDir, 0755)

	s.tenants[token] = tenant
	s.saveConfig()

	fmt.Printf("[%s] 创建租户: %s (%s)\n", time.Now().Format("15:04:05"), name, id)

	return tenant, nil
}

// DeleteTenant 删除租户
func (s *Server) DeleteTenant(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	var tokenToDelete string
	for token, t := range s.tenants {
		if t.ID == id {
			tokenToDelete = token
			break
		}
	}

	if tokenToDelete == "" {
		return fmt.Errorf("tenant not found")
	}

	delete(s.tenants, tokenToDelete)

	// 删除数据目录
	tenantDir := filepath.Join(s.dataDir, "tenants", id)
	os.RemoveAll(tenantDir)

	s.saveConfig()

	return nil
}

// getTenantByToken 根据 token 获取租户
func (s *Server) getTenantByToken(token string) *Tenant {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.tenants[token]
}

// getTenantDataDir 获取租户数据目录
func (s *Server) getTenantDataDir(tenant *Tenant) string {
	return filepath.Join(s.dataDir, "tenants", tenant.ID)
}

// loadTenantData 加载租户数据
func (s *Server) loadTenantData(tenant *Tenant) {
	tenantDir := s.getTenantDataDir(tenant)

	filepath.Walk(tenantDir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return nil
		}

		relPath, _ := filepath.Rel(tenantDir, path)
		data, err := os.ReadFile(path)
		if err != nil {
			return nil
		}

		hash := sha256.Sum256(data)
		tenant.Files[relPath] = FileInfo{
			Path:    relPath,
			Hash:    hex.EncodeToString(hash[:]),
			ModTime: info.ModTime().Unix(),
			Size:    info.Size(),
		}
		return nil
	})
}

// Start 启动服务器
func (s *Server) Start() error {
	mux := http.NewServeMux()

	// 公开接口
	mux.HandleFunc("/health", s.handleHealth)

	// 租户接口 (需要租户 token)
	mux.HandleFunc("/sync", s.tenantAuth(s.handleSync))
	mux.HandleFunc("/stats", s.tenantAuth(s.handleTenantStats))

	// 管理接口 (需要 admin token，暂时用第一个租户的 token)
	mux.HandleFunc("/admin/tenants", s.handleAdminTenants)
	mux.HandleFunc("/admin/stats", s.handleAdminStats)

	fmt.Printf("Claude Sync 服务器启动 (多租户模式)\n")
	fmt.Printf("监听端口: %d\n", s.port)
	fmt.Printf("数据目录: %s\n", s.dataDir)
	fmt.Printf("租户数量: %d\n", len(s.tenants))
	fmt.Println("等待客户端连接...")

	return http.ListenAndServe(fmt.Sprintf(":%d", s.port), mux)
}

// tenantAuth 租户认证中间件
func (s *Server) tenantAuth(next func(http.ResponseWriter, *http.Request, *Tenant)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		auth := r.Header.Get("Authorization")
		if !strings.HasPrefix(auth, "Bearer ") {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}

		token := strings.TrimPrefix(auth, "Bearer ")
		tenant := s.getTenantByToken(token)
		if tenant == nil {
			http.Error(w, "Invalid token", http.StatusUnauthorized)
			return
		}

		// 更新最后活跃时间
		s.mu.Lock()
		tenant.LastActive = time.Now()
		s.mu.Unlock()

		next(w, r, tenant)
	}
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"status":  "ok",
		"version": "1.0.0",
		"time":    time.Now(),
		"tenants": len(s.tenants),
	})
}

func (s *Server) handleSync(w http.ResponseWriter, r *http.Request, tenant *Tenant) {
	if r.Method != "POST" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req SyncRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	clientIP := r.RemoteAddr
	if forwarded := r.Header.Get("X-Forwarded-For"); forwarded != "" {
		clientIP = forwarded
	}

	fmt.Printf("[%s] [%s] 同步请求: %s (%s) @ %s, 文件数: %d\n",
		time.Now().Format("15:04:05"),
		tenant.Name,
		req.MachineName, req.MachineID, clientIP, len(req.Files))

	s.mu.Lock()

	// 更新客户端信息
	if tenant.Clients == nil {
		tenant.Clients = make(map[string]*ClientInfo)
	}
	tenant.Clients[req.MachineID] = &ClientInfo{
		MachineID:   req.MachineID,
		MachineName: req.MachineName,
		LastSeen:    time.Now(),
		FileCount:   len(req.Files),
		IP:          clientIP,
	}

	tenantDir := s.getTenantDataDir(tenant)
	var filesToSend []FileInfo

	// 处理客户端发来的文件
	for _, f := range req.Files {
		existing, exists := tenant.Files[f.Path]

		if len(f.Content) > 0 {
			if !exists || f.ModTime > existing.ModTime {
				tenant.Files[f.Path] = f
				s.saveTenantFile(tenant, f)
			}
		}

		if exists && existing.Hash != f.Hash && existing.ModTime > f.ModTime {
			content, err := os.ReadFile(filepath.Join(tenantDir, existing.Path))
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

	// 检查服务器上有但客户端没有的文件
	clientFiles := make(map[string]bool)
	for _, f := range req.Files {
		clientFiles[f.Path] = true
	}

	for path, f := range tenant.Files {
		if !clientFiles[path] {
			content, err := os.ReadFile(filepath.Join(tenantDir, path))
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

	s.mu.Unlock()

	if len(filesToSend) > 0 {
		fmt.Printf("[%s] [%s] 发送 %d 个文件到 %s\n",
			time.Now().Format("15:04:05"), tenant.Name, len(filesToSend), req.MachineName)
	}

	resp := SyncResponse{
		Success: true,
		Message: "OK",
		Files:   filesToSend,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

func (s *Server) saveTenantFile(tenant *Tenant, f FileInfo) error {
	path := filepath.Join(s.getTenantDataDir(tenant), f.Path)
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	return os.WriteFile(path, f.Content, 0644)
}

func (s *Server) handleTenantStats(w http.ResponseWriter, r *http.Request, tenant *Tenant) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var totalSize int64
	for _, f := range tenant.Files {
		totalSize += f.Size
	}

	clients := make([]*ClientInfo, 0, len(tenant.Clients))
	for _, c := range tenant.Clients {
		clients = append(clients, c)
	}

	stats := TenantStats{
		ID:          tenant.ID,
		Name:        tenant.Name,
		FileCount:   len(tenant.Files),
		TotalSize:   totalSize,
		ClientCount: len(tenant.Clients),
		Clients:     clients,
		LastActive:  tenant.LastActive,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(stats)
}

func (s *Server) handleAdminTenants(w http.ResponseWriter, r *http.Request) {
	// 简单的 admin 认证 (使用 query param)
	adminToken := r.URL.Query().Get("admin_token")
	if adminToken == "" {
		http.Error(w, "Admin token required", http.StatusUnauthorized)
		return
	}

	// 验证 admin token (这里简单处理，实际应该有独立的 admin token)
	s.mu.RLock()
	validAdmin := false
	for _, t := range s.tenants {
		if t.Token == adminToken {
			validAdmin = true
			break
		}
	}
	s.mu.RUnlock()

	if !validAdmin {
		http.Error(w, "Invalid admin token", http.StatusUnauthorized)
		return
	}

	switch r.Method {
	case "GET":
		// 列出所有租户
		s.mu.RLock()
		tenants := make([]*TenantStats, 0, len(s.tenants))
		for _, t := range s.tenants {
			var totalSize int64
			for _, f := range t.Files {
				totalSize += f.Size
			}
			tenants = append(tenants, &TenantStats{
				ID:          t.ID,
				Name:        t.Name,
				FileCount:   len(t.Files),
				TotalSize:   totalSize,
				ClientCount: len(t.Clients),
				LastActive:  t.LastActive,
			})
		}
		s.mu.RUnlock()

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(tenants)

	case "POST":
		// 创建租户
		var req struct {
			ID    string `json:"id"`
			Name  string `json:"name"`
			Token string `json:"token"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		tenant, err := s.CreateTenant(req.ID, req.Name, req.Token)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": true,
			"tenant": map[string]string{
				"id":   tenant.ID,
				"name": tenant.Name,
			},
		})

	case "DELETE":
		// 删除租户
		id := r.URL.Query().Get("id")
		if id == "" {
			http.Error(w, "Tenant ID required", http.StatusBadRequest)
			return
		}

		if err := s.DeleteTenant(id); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]bool{"success": true})

	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleAdminStats(w http.ResponseWriter, r *http.Request) {
	adminToken := r.URL.Query().Get("admin_token")
	if adminToken == "" {
		http.Error(w, "Admin token required", http.StatusUnauthorized)
		return
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	var totalFiles int
	var totalSize int64
	tenantStats := make([]*TenantStats, 0, len(s.tenants))

	for _, t := range s.tenants {
		var tSize int64
		for _, f := range t.Files {
			tSize += f.Size
		}
		totalFiles += len(t.Files)
		totalSize += tSize

		clients := make([]*ClientInfo, 0, len(t.Clients))
		for _, c := range t.Clients {
			clients = append(clients, c)
		}

		tenantStats = append(tenantStats, &TenantStats{
			ID:          t.ID,
			Name:        t.Name,
			FileCount:   len(t.Files),
			TotalSize:   tSize,
			ClientCount: len(t.Clients),
			Clients:     clients,
			LastActive:  t.LastActive,
		})
	}

	stats := ServerStats{
		TotalTenants: len(s.tenants),
		TotalFiles:   totalFiles,
		TotalSize:    totalSize,
		Tenants:      tenantStats,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(stats)
}
