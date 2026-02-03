package service

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/k0ngk0ng/claude-sync/internal/config"
)

// SyncStatus 同步状态
type SyncStatus int

const (
	StatusIdle SyncStatus = iota
	StatusSyncing
	StatusError
	StatusOffline
)

func (s SyncStatus) String() string {
	switch s {
	case StatusIdle:
		return "已同步"
	case StatusSyncing:
		return "同步中..."
	case StatusError:
		return "同步失败"
	case StatusOffline:
		return "未连接"
	default:
		return "未知"
	}
}

// FileInfo 文件信息
type FileInfo struct {
	Path    string `json:"path"`
	Hash    string `json:"hash"`
	ModTime int64  `json:"mod_time"`
	Size    int64  `json:"size"`
	Content []byte `json:"content,omitempty"`
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

// SyncStats 同步统计
type SyncStats struct {
	TotalFiles   int       `json:"total_files"`
	TotalSize    int64     `json:"total_size"`
	LastSync     time.Time `json:"last_sync"`
	LastError    string    `json:"last_error"`
	Uploaded     int       `json:"uploaded"`
	Downloaded   int       `json:"downloaded"`
}

// StatusCallback 状态回调
type StatusCallback func(status SyncStatus, stats *SyncStats)

// SyncService 同步服务
type SyncService struct {
	config     *config.Config
	claudeDir  string
	fileHashes map[string]string
	mu         sync.RWMutex
	stopChan   chan struct{}
	status     SyncStatus
	stats      SyncStats
	callback   StatusCallback
	running    bool
}

// NewSyncService 创建同步服务
func NewSyncService(cfg *config.Config) *SyncService {
	return &SyncService{
		config:     cfg,
		claudeDir:  config.GetClaudeDir(),
		fileHashes: make(map[string]string),
		stopChan:   make(chan struct{}),
		status:     StatusOffline,
	}
}

// SetCallback 设置状态回调
func (s *SyncService) SetCallback(cb StatusCallback) {
	s.callback = cb
}

// GetStatus 获取当前状态
func (s *SyncService) GetStatus() SyncStatus {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.status
}

// GetStats 获取统计信息
func (s *SyncService) GetStats() SyncStats {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.stats
}

// Start 启动同步服务
func (s *SyncService) Start() {
	if s.running {
		return
	}
	s.running = true
	s.stopChan = make(chan struct{})

	go s.run()
}

// Stop 停止同步服务
func (s *SyncService) Stop() {
	if !s.running {
		return
	}
	s.running = false
	close(s.stopChan)
}

// IsRunning 是否运行中
func (s *SyncService) IsRunning() bool {
	return s.running
}

// SyncNow 立即同步
func (s *SyncService) SyncNow() error {
	return s.syncOnce()
}

// UpdateConfig 更新配置
func (s *SyncService) UpdateConfig(cfg *config.Config) {
	s.mu.Lock()
	s.config = cfg
	s.mu.Unlock()
}

func (s *SyncService) run() {
	// 立即执行一次
	s.syncOnce()

	ticker := time.NewTicker(time.Duration(s.config.SyncInterval) * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			if !s.config.Paused {
				s.syncOnce()
			}
		case <-s.stopChan:
			return
		}
	}
}

func (s *SyncService) setStatus(status SyncStatus) {
	s.mu.Lock()
	s.status = status
	s.mu.Unlock()

	if s.callback != nil {
		s.callback(status, &s.stats)
	}
}

func (s *SyncService) syncOnce() error {
	if !s.config.IsConfigured() {
		s.setStatus(StatusOffline)
		return fmt.Errorf("未配置服务器")
	}

	s.setStatus(StatusSyncing)

	// 扫描本地文件
	localFiles, totalSize, err := s.scanLocalFiles()
	if err != nil {
		s.mu.Lock()
		s.stats.LastError = err.Error()
		s.mu.Unlock()
		s.setStatus(StatusError)
		return err
	}

	s.mu.Lock()
	s.stats.TotalFiles = len(localFiles)
	s.stats.TotalSize = totalSize
	s.mu.Unlock()

	// 发送同步请求
	req := SyncRequest{
		MachineID:   s.config.MachineID,
		MachineName: s.config.MachineName,
		Files:       localFiles,
	}

	respFiles, err := s.sendSyncRequest(req)
	if err != nil {
		s.mu.Lock()
		s.stats.LastError = err.Error()
		s.mu.Unlock()
		s.setStatus(StatusError)
		return err
	}

	// 应用远程更新
	downloaded := 0
	for _, f := range respFiles {
		if len(f.Content) > 0 {
			localPath := s.applyPathMapping(f.Path)
			destPath := filepath.Join(s.claudeDir, localPath)
			content := s.applyContentPathMapping(f.Content)

			if err := os.MkdirAll(filepath.Dir(destPath), 0755); err != nil {
				continue
			}
			if err := os.WriteFile(destPath, content, 0644); err != nil {
				continue
			}
			downloaded++
		}
	}

	s.mu.Lock()
	s.stats.LastSync = time.Now()
	s.stats.Downloaded = downloaded
	s.stats.LastError = ""
	s.mu.Unlock()

	s.setStatus(StatusIdle)
	return nil
}

func (s *SyncService) scanLocalFiles() ([]FileInfo, int64, error) {
	var files []FileInfo
	var totalSize int64
	projectsDir := filepath.Join(s.claudeDir, "projects")

	err := filepath.Walk(projectsDir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return nil
		}

		relPath, _ := filepath.Rel(s.claudeDir, path)
		data, err := os.ReadFile(path)
		if err != nil {
			return nil
		}

		hash := sha256.Sum256(data)
		hashStr := hex.EncodeToString(hash[:])

		s.mu.RLock()
		oldHash := s.fileHashes[relPath]
		s.mu.RUnlock()

		remotePath := s.reversePathMapping(relPath)

		fileInfo := FileInfo{
			Path:    remotePath,
			Hash:    hashStr,
			ModTime: info.ModTime().Unix(),
			Size:    info.Size(),
		}

		if oldHash != hashStr {
			fileInfo.Content = s.reverseContentPathMapping(data)
			s.mu.Lock()
			s.fileHashes[relPath] = hashStr
			s.mu.Unlock()
		}

		files = append(files, fileInfo)
		totalSize += info.Size()
		return nil
	})

	return files, totalSize, err
}

func (s *SyncService) sendSyncRequest(req SyncRequest) ([]FileInfo, error) {
	data, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}

	httpReq, err := http.NewRequest("POST", s.config.ServerURL+"/sync", bytes.NewReader(data))
	if err != nil {
		return nil, err
	}

	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+s.config.Token)

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

// 路径映射相关
func (s *SyncService) applyPathMapping(path string) string {
	for remote, local := range s.config.PathMappings {
		if strings.Contains(path, remote) {
			return strings.Replace(path, remote, local, 1)
		}
	}
	return path
}

func (s *SyncService) reversePathMapping(path string) string {
	for remote, local := range s.config.PathMappings {
		if strings.Contains(path, local) {
			return strings.Replace(path, local, remote, 1)
		}
	}
	return path
}

func (s *SyncService) applyContentPathMapping(content []byte) []byte {
	result := string(content)
	for remote, local := range s.config.PathMappings {
		result = strings.ReplaceAll(result, remote, local)
	}
	return []byte(result)
}

func (s *SyncService) reverseContentPathMapping(content []byte) []byte {
	result := string(content)
	for remote, local := range s.config.PathMappings {
		result = strings.ReplaceAll(result, local, remote)
	}
	return []byte(result)
}

// CheckConnection 检查服务器连接
func (s *SyncService) CheckConnection() bool {
	if s.config.ServerURL == "" {
		return false
	}

	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Get(s.config.ServerURL + "/health")
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	return resp.StatusCode == 200
}
