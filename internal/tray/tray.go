package tray

import (
	"fmt"
	"time"

	"github.com/getlantern/systray"
	"github.com/k0ngk0ng/claude-sync/internal/config"
	"github.com/k0ngk0ng/claude-sync/internal/service"
)

// App 托盘应用
type App struct {
	config      *config.Config
	syncService *service.SyncService
	onSettings  func()
	onQuit      func()

	// 菜单项
	mStatus    *systray.MenuItem
	mLastSync  *systray.MenuItem
	mFiles     *systray.MenuItem
	mSyncNow   *systray.MenuItem
	mPause     *systray.MenuItem
	mSettings  *systray.MenuItem
	mQuit      *systray.MenuItem
}

// NewApp 创建托盘应用
func NewApp(cfg *config.Config, onSettings, onQuit func()) *App {
	return &App{
		config:     cfg,
		onSettings: onSettings,
		onQuit:     onQuit,
	}
}

// Run 运行托盘应用
func (a *App) Run() {
	systray.Run(a.onReady, a.onExit)
}

func (a *App) onReady() {
	// 设置图标和标题
	systray.SetIcon(iconIdle)
	systray.SetTitle("Claude Sync")
	systray.SetTooltip("Claude Sync - 历史记录同步")

	// 创建菜单
	a.mStatus = systray.AddMenuItem("⚪ 未连接", "同步状态")
	a.mStatus.Disable()

	a.mLastSync = systray.AddMenuItem("上次同步: 从未", "上次同步时间")
	a.mLastSync.Disable()

	a.mFiles = systray.AddMenuItem("📁 0 个文件", "文件统计")
	a.mFiles.Disable()

	systray.AddSeparator()

	a.mSyncNow = systray.AddMenuItem("🔄 立即同步", "立即执行同步")
	a.mPause = systray.AddMenuItem("⏸️ 暂停同步", "暂停/恢复同步")

	systray.AddSeparator()

	a.mSettings = systray.AddMenuItem("⚙️ 设置...", "打开设置")
	// mLogs := systray.AddMenuItem("📋 查看日志", "查看同步日志")

	systray.AddSeparator()

	a.mQuit = systray.AddMenuItem("退出", "退出 Claude Sync")

	// 启动同步服务
	a.syncService = service.NewSyncService(a.config)
	a.syncService.SetCallback(a.onStatusChange)
	a.syncService.Start()

	// 处理菜单事件
	go a.handleEvents()
}

func (a *App) onExit() {
	if a.syncService != nil {
		a.syncService.Stop()
	}
}

func (a *App) handleEvents() {
	for {
		select {
		case <-a.mSyncNow.ClickedCh:
			go a.syncService.SyncNow()

		case <-a.mPause.ClickedCh:
			a.config.Paused = !a.config.Paused
			a.config.Save()
			a.syncService.UpdateConfig(a.config)
			if a.config.Paused {
				a.mPause.SetTitle("▶️ 恢复同步")
				systray.SetIcon(iconPaused)
			} else {
				a.mPause.SetTitle("⏸️ 暂停同步")
				systray.SetIcon(iconIdle)
			}

		case <-a.mSettings.ClickedCh:
			if a.onSettings != nil {
				a.onSettings()
			}

		case <-a.mQuit.ClickedCh:
			if a.onQuit != nil {
				a.onQuit()
			}
			systray.Quit()
			return
		}
	}
}

func (a *App) onStatusChange(status service.SyncStatus, stats *service.SyncStats) {
	// 更新图标
	switch status {
	case service.StatusIdle:
		systray.SetIcon(iconIdle)
		a.mStatus.SetTitle("✅ 已同步")
	case service.StatusSyncing:
		systray.SetIcon(iconSyncing)
		a.mStatus.SetTitle("🔄 同步中...")
	case service.StatusError:
		systray.SetIcon(iconError)
		a.mStatus.SetTitle("❌ 同步失败")
	case service.StatusOffline:
		systray.SetIcon(iconOffline)
		a.mStatus.SetTitle("⚪ 未连接")
	}

	// 更新统计
	if !stats.LastSync.IsZero() {
		a.mLastSync.SetTitle(fmt.Sprintf("上次同步: %s", formatTime(stats.LastSync)))
	}
	a.mFiles.SetTitle(fmt.Sprintf("📁 %d 个文件 · %s", stats.TotalFiles, formatSize(stats.TotalSize)))
}

func formatTime(t time.Time) string {
	diff := time.Since(t)
	if diff < time.Minute {
		return "刚刚"
	} else if diff < time.Hour {
		return fmt.Sprintf("%d 分钟前", int(diff.Minutes()))
	} else if diff < 24*time.Hour {
		return fmt.Sprintf("%d 小时前", int(diff.Hours()))
	}
	return t.Format("01-02 15:04")
}

func formatSize(size int64) string {
	const (
		KB = 1024
		MB = KB * 1024
		GB = MB * 1024
	)
	switch {
	case size >= GB:
		return fmt.Sprintf("%.1f GB", float64(size)/GB)
	case size >= MB:
		return fmt.Sprintf("%.1f MB", float64(size)/MB)
	case size >= KB:
		return fmt.Sprintf("%.1f KB", float64(size)/KB)
	default:
		return fmt.Sprintf("%d B", size)
	}
}

// UpdateConfig 更新配置
func (a *App) UpdateConfig(cfg *config.Config) {
	a.config = cfg
	if a.syncService != nil {
		a.syncService.UpdateConfig(cfg)
	}
}
