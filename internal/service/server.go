package service

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// Server 同步服务器
type Server struct {
	dataDir string
	token   string
	port    int
	mu      sync.RWMutex
	files   map[string]FileInfo
	clients map[string]*ClientInfo // machineID -> ClientInfo
}

// ClientInfo 客户端信息
type ClientInfo struct {
	MachineID   string    `json:"machine_id"`
	MachineName string    `json:"machine_name"`
	LastSeen    time.Time `json:"last_seen"`
	FileCount   int       `json:"file_count"`
	IP          string    `json:"ip"`
}

// ServerStats 服务器统计
type ServerStats struct {
	TotalFiles   int           `json:"total_files"`
	TotalSize    int64         `json:"total_size"`
	ClientCount  int           `json:"client_count"`
	Clients      []*ClientInfo `json:"clients"`
	StartTime    time.Time     `json:"start_time"`
}

// NewServer 创建服务器
func NewServer(port int, dataDir, token string) *Server {
	return &Server{
		dataDir: dataDir,
		token:   token,
		port:    port,
		files:   make(map[string]FileInfo),
		clients: make(map[string]*ClientInfo),
	}
}

// Start 启动服务器
func (s *Server) Start() error {
	// 创建数据目录
	if err := os.MkdirAll(s.dataDir, 0755); err != nil {
		return fmt.Errorf("无法创建数据目录: %w", err)
	}

	// 加载已有数据
	s.loadData()

	// 路由
	mux := http.NewServeMux()
	mux.HandleFunc("/health", s.handleHealth)
	mux.HandleFunc("/sync", s.authMiddleware(s.handleSync))
	mux.HandleFunc("/stats", s.authMiddleware(s.handleStats))

	fmt.Printf("Claude Sync 服务器启动\n")
	fmt.Printf("监听端口: %d\n", s.port)
	fmt.Printf("数据目录: %s\n", s.dataDir)
	fmt.Printf("已加载 %d 个文件\n", len(s.files))
	fmt.Println("等待客户端连接...")

	return http.ListenAndServe(fmt.Sprintf(":%d", s.port), mux)
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
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"status":  "ok",
		"version": "1.0.0",
		"time":    time.Now(),
	})
}

func (s *Server) handleStats(w http.ResponseWriter, r *http.Request) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var totalSize int64
	for _, f := range s.files {
		totalSize += f.Size
	}

	clients := make([]*ClientInfo, 0, len(s.clients))
	for _, c := range s.clients {
		clients = append(clients, c)
	}

	stats := ServerStats{
		TotalFiles:  len(s.files),
		TotalSize:   totalSize,
		ClientCount: len(s.clients),
		Clients:     clients,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(stats)
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

	// 获取客户端 IP
	clientIP := r.RemoteAddr
	if forwarded := r.Header.Get("X-Forwarded-For"); forwarded != "" {
		clientIP = forwarded
	}

	fmt.Printf("[%s] 同步请求来自: %s (%s) @ %s, 文件数: %d\n",
		time.Now().Format("15:04:05"),
		req.MachineName, req.MachineID, clientIP, len(req.Files))

	s.mu.Lock()
	defer s.mu.Unlock()

	// 更新客户端信息
	s.clients[req.MachineID] = &ClientInfo{
		MachineID:   req.MachineID,
		MachineName: req.MachineName,
		LastSeen:    time.Now(),
		FileCount:   len(req.Files),
		IP:          clientIP,
	}

	var filesToSend []FileInfo

	// 处理客户端发来的文件
	for _, f := range req.Files {
		existing, exists := s.files[f.Path]

		// 如果客户端发来了内容（有更新）
		if len(f.Content) > 0 {
			// 检查是否比服务器的更新
			if !exists || f.ModTime > existing.ModTime {
				// 保存到服务器
				s.files[f.Path] = f
				s.saveFile(f)
			}
		}

		// 检查服务器是否有更新的版本需要发给客户端
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

	// 检查服务器上有但客户端没有的文件
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

	if len(filesToSend) > 0 {
		fmt.Printf("[%s] 发送 %d 个文件到 %s\n",
			time.Now().Format("15:04:05"), len(filesToSend), req.MachineName)
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
}
