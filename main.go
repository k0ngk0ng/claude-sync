package main

import (
	"context"
	"embed"
	"fmt"

	"github.com/k0ngk0ng/claude-sync/internal/config"
	"github.com/k0ngk0ng/claude-sync/internal/service"
	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	"github.com/wailsapp/wails/v2/pkg/options/mac"
	"github.com/wailsapp/wails/v2/pkg/options/windows"
)

//go:embed all:frontend/dist
var assets embed.FS

func main() {
	// 加载配置
	cfg, err := config.Load()
	if err != nil {
		fmt.Printf("加载配置失败: %v\n", err)
		return
	}

	// 创建应用实例
	app := NewApp(cfg)

	// 创建 Wails 应用
	err = wails.Run(&options.App{
		Title:     "Claude Sync",
		Width:     420,
		Height:    520,
		MinWidth:  400,
		MinHeight: 500,
		AssetServer: &assetserver.Options{
			Assets: assets,
		},
		BackgroundColour: &options.RGBA{R: 255, G: 255, B: 255, A: 255},
		OnStartup:        app.startup,
		OnShutdown:       app.shutdown,
		Bind: []interface{}{
			app,
		},
		Mac: &mac.Options{
			TitleBar:             mac.TitleBarHiddenInset(),
			WebviewIsTransparent: true,
			WindowIsTranslucent:  true,
			About: &mac.AboutInfo{
				Title:   "Claude Sync",
				Message: "Claude Code 历史记录同步工具\n版本 1.0.0",
			},
		},
		Windows: &windows.Options{
			WebviewIsTransparent: false,
			WindowIsTranslucent:  false,
			DisableWindowIcon:    false,
		},
		// 启用系统托盘
		StartHidden: false,
	})

	if err != nil {
		fmt.Printf("启动失败: %v\n", err)
	}
}

// App 应用结构
type App struct {
	config      *config.Config
	syncService *service.SyncService
}

// NewApp 创建应用
func NewApp(cfg *config.Config) *App {
	return &App{
		config: cfg,
	}
}

func (a *App) startup(ctx context.Context) {
	// 启动同步服务
	a.syncService = service.NewSyncService(a.config)
	a.syncService.Start()
}

func (a *App) shutdown(ctx context.Context) {
	if a.syncService != nil {
		a.syncService.Stop()
	}
}

// ==================== 暴露给前端的方法 ====================

// GetConfig 获取配置
func (a *App) GetConfig() *config.Config {
	return a.config
}

// SaveConfig 保存配置
func (a *App) SaveConfig(serverURL, token, machineName string, syncInterval int) error {
	a.config.ServerURL = serverURL
	a.config.Token = token
	a.config.MachineName = machineName
	if syncInterval > 0 {
		a.config.SyncInterval = syncInterval
	}

	if err := a.config.Save(); err != nil {
		return err
	}

	// 更新同步服务配置
	if a.syncService != nil {
		a.syncService.UpdateConfig(a.config)
	}

	return nil
}

// GetStatus 获取同步状态
func (a *App) GetStatus() map[string]interface{} {
	if a.syncService == nil {
		return map[string]interface{}{
			"status":     "offline",
			"statusText": "未启动",
		}
	}

	status := a.syncService.GetStatus()
	stats := a.syncService.GetStats()

	return map[string]interface{}{
		"status":      status.String(),
		"statusCode":  int(status),
		"totalFiles":  stats.TotalFiles,
		"totalSize":   stats.TotalSize,
		"lastSync":    stats.LastSync.Unix(),
		"lastError":   stats.LastError,
		"uploaded":    stats.Uploaded,
		"downloaded":  stats.Downloaded,
		"isConnected": a.syncService.CheckConnection(),
	}
}

// SyncNow 立即同步
func (a *App) SyncNow() error {
	if a.syncService == nil {
		return fmt.Errorf("同步服务未启动")
	}
	return a.syncService.SyncNow()
}

// TogglePause 切换暂停状态
func (a *App) TogglePause() bool {
	a.config.Paused = !a.config.Paused
	a.config.Save()
	if a.syncService != nil {
		a.syncService.UpdateConfig(a.config)
	}
	return a.config.Paused
}

// GetPathMappings 获取路径映射
func (a *App) GetPathMappings() map[string]string {
	return a.config.PathMappings
}

// AddPathMapping 添加路径映射
func (a *App) AddPathMapping(remotePath, localPath string) error {
	if a.config.PathMappings == nil {
		a.config.PathMappings = make(map[string]string)
	}
	a.config.PathMappings[remotePath] = localPath
	return a.config.Save()
}

// RemovePathMapping 删除路径映射
func (a *App) RemovePathMapping(remotePath string) error {
	delete(a.config.PathMappings, remotePath)
	return a.config.Save()
}

// CheckConnection 检查服务器连接
func (a *App) CheckConnection() bool {
	if a.syncService == nil {
		return false
	}
	return a.syncService.CheckConnection()
}
