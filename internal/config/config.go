package config

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"time"
)

// Config 应用配置
type Config struct {
	ServerURL    string            `json:"server_url"`
	Token        string            `json:"token"`
	MachineID    string            `json:"machine_id"`
	MachineName  string            `json:"machine_name"`
	SyncInterval int               `json:"sync_interval"` // 秒
	PathMappings map[string]string `json:"path_mappings"` // remote -> local
	AutoStart    bool              `json:"auto_start"`    // 开机自启
	Paused       bool              `json:"paused"`        // 暂停同步
}

// DefaultConfig 默认配置
func DefaultConfig() *Config {
	return &Config{
		MachineID:    generateMachineID(),
		SyncInterval: 30,
		PathMappings: make(map[string]string),
		AutoStart:    true,
		Paused:       false,
	}
}

// GetClaudeDir 获取 Claude 配置目录
func GetClaudeDir() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".claude")
}

// GetConfigPath 获取配置文件路径
func GetConfigPath() string {
	return filepath.Join(GetClaudeDir(), "sync-config.json")
}

// GetLogPath 获取日志文件路径
func GetLogPath() string {
	return filepath.Join(GetClaudeDir(), "sync.log")
}

// Load 加载配置
func Load() (*Config, error) {
	data, err := os.ReadFile(GetConfigPath())
	if err != nil {
		if os.IsNotExist(err) {
			return DefaultConfig(), nil
		}
		return nil, err
	}

	config := DefaultConfig()
	if err := json.Unmarshal(data, config); err != nil {
		return nil, err
	}

	// 确保必要字段
	if config.MachineID == "" {
		config.MachineID = generateMachineID()
	}
	if config.SyncInterval == 0 {
		config.SyncInterval = 30
	}
	if config.PathMappings == nil {
		config.PathMappings = make(map[string]string)
	}

	return config, nil
}

// Save 保存配置
func (c *Config) Save() error {
	// 确保目录存在
	if err := os.MkdirAll(filepath.Dir(GetConfigPath()), 0755); err != nil {
		return err
	}

	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(GetConfigPath(), data, 0600)
}

// IsConfigured 检查是否已配置
func (c *Config) IsConfigured() bool {
	return c.ServerURL != "" && c.Token != ""
}

// generateMachineID 生成机器ID
func generateMachineID() string {
	hostname, _ := os.Hostname()
	data := fmt.Sprintf("%s-%s-%d", hostname, runtime.GOOS, time.Now().UnixNano())
	hash := sha256.Sum256([]byte(data))
	return hex.EncodeToString(hash[:8])
}
