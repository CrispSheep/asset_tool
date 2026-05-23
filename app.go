package main

import (
	"context"
	"crypto/rand"
	"fmt"
	"log"
	"os"
	"path/filepath"

	"asset_tool_go/internal/db"
	"asset_tool_go/internal/model"
	"asset_tool_go/internal/parser"
	"asset_tool_go/internal/scanner"

	wruntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

// App 主应用
type App struct {
	ctx context.Context
}

// NewApp 创建 App
func NewApp() *App {
	return &App{}
}

// startup Wails 启动时调用
func (a *App) startup(ctx context.Context) {
	a.ctx = ctx

	// 数据库放在用户主目录下的 .asset_tool_go/asset_tool.db
	home, err := os.UserHomeDir()
	if err != nil {
		log.Fatalf("get home dir: %v", err)
	}
	dir := filepath.Join(home, ".asset_tool_go")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		log.Fatalf("mkdir data dir: %v", err)
	}
	dbPath := filepath.Join(dir, "asset_tool.db")
	if err := db.Init(dbPath); err != nil {
		log.Fatalf("init db: %v", err)
	}
	log.Printf("数据库已就绪: %s", dbPath)
}

// shutdown Wails 关闭时调用
func (a *App) shutdown(ctx context.Context) {
	_ = db.Close()
}

// ── Project APIs（前端通过 wailsjs 调用）────────────────────────────────

// ListProjects 列出项目（支持搜索）
func (a *App) ListProjects(search string) ([]model.Project, error) {
	return db.ListProjects(search)
}

// CreateProject 新建项目
func (a *App) CreateProject(name string) (int64, error) {
	return db.CreateProject(name)
}

// RenameProject 重命名
func (a *App) RenameProject(id int64, newName string) error {
	return db.RenameProject(id, newName)
}

// DeleteProject 删除项目
func (a *App) DeleteProject(id int64) error {
	return db.DeleteProject(id)
}

// ── Asset / CSV APIs ──────────────────────────────────────────────────────

// ImportCSV 选择 CSV 文件 → 解析 → 入库
func (a *App) ImportCSV(projectID int64, source string) (map[string]any, error) {
	path, err := wruntime.OpenFileDialog(a.ctx, wruntime.OpenDialogOptions{
		Title: "选择 CSV 文件",
		Filters: []wruntime.FileFilter{
			{DisplayName: "CSV 文件", Pattern: "*.csv"},
			{DisplayName: "所有文件", Pattern: "*.*"},
		},
	})
	if err != nil {
		return nil, err
	}
	if path == "" {
		return map[string]any{"cancelled": true}, nil
	}
	entries, stats, err := parser.ParseCSV(path, source)
	if err != nil {
		return nil, err
	}
	res, err := db.UpsertAssets(projectID, entries)
	if err != nil {
		return nil, err
	}
	return map[string]any{
		"path":        path,
		"total_rows":  stats.TotalRows,
		"detected_ip": stats.IP,
		"detected_dm": stats.Domain,
		"new_ip":      res.NewIP,
		"new_domain":  res.NewDomain,
		"skipped":     res.Skipped,
	}, nil
}

// ManualAddAssets 手动添加（多行文本，每行 host[:port]）
func (a *App) ManualAddAssets(projectID int64, lines []string, source string) (model.ImportResult, error) {
	var entries []model.AssetEntry
	seen := map[string]struct{}{}
	for _, line := range lines {
		for _, item := range parser.ExtractFromValue(line) {
			if _, ok := seen[item.Host]; ok {
				continue
			}
			seen[item.Host] = struct{}{}
			item.Source = source
			entries = append(entries, item)
		}
	}
	return db.UpsertAssets(projectID, entries)
}

// ListAssets 列出资产
func (a *App) ListAssets(projectID int64, typeFilter, statusFilter string) ([]model.Asset, error) {
	return db.ListAssets(projectID, typeFilter, statusFilter)
}

// ListAssetsPage 分页列出资产
func (a *App) ListAssetsPage(projectID int64, typeFilter, statusFilter, keyword, networkFilter, sortsJSON string, page, pageSize int) (db.AssetPageResult, error) {
	return db.ListAssetsPage(projectID, typeFilter, statusFilter, keyword, networkFilter, sortsJSON, page, pageSize)
}

// CountAssetStats 统计项目资产各维度数量
func (a *App) CountAssetStats(projectID int64) (map[string]int, error) {
	return db.CountAssetStats(projectID)
}

// DeleteAsset 删除单个
func (a *App) DeleteAsset(id int64) error {
	return db.DeleteAsset(id)
}

// DeleteAssets 批量删除
func (a *App) DeleteAssets(ids []int64) error {
	return db.DeleteAssets(ids)
}

// UpdateTags 更新单个资产标签
func (a *App) UpdateTags(id int64, tags []string) error {
	return db.UpdateTags(id, tags)
}

// BatchAddTag 批量加标签
func (a *App) BatchAddTag(ids []int64, tag string) error {
	return db.BatchAddTag(ids, tag)
}

// BatchRemoveTag 批量移除标签
func (a *App) BatchRemoveTag(ids []int64, tag string) error {
	return db.BatchRemoveTag(ids, tag)
}

// ── Settings ──────────────────────────────────────────────────────────────

// GetSetting 读取
func (a *App) GetSetting(key string) string {
	return db.GetSetting(key)
}

// SetSetting 写入
func (a *App) SetSetting(key, value string) error {
	return db.SetSetting(key, value)
}

// ── httpx 探活 APIs ──────────────────────────────────────────────────────

// RunHttpx 启动 httpx 探活（异步执行，进度通过事件推送）
func (a *App) RunHttpx(projectID int64, cfg scanner.HttpxConfig) (string, error) {
	jobID := fmt.Sprintf("httpx-%d-%d", projectID, randInt())
	go func() {
		defer func() {
			if r := recover(); r != nil {
				wruntime.EventsEmit(a.ctx, "httpx:error", fmt.Sprintf("内部错误: %v", r))
			}
		}()
		if err := scanner.RunHttpx(a.ctx, jobID, projectID, cfg); err != nil {
			wruntime.EventsEmit(a.ctx, "httpx:error", err.Error())
		}
	}()
	return jobID, nil
}

// PauseJob 暂停任务
func (a *App) PauseJob(jobID string) {
	if j := scanner.GetJob(jobID); j != nil {
		j.Pause()
	}
}

// ResumeJob 继续任务
func (a *App) ResumeJob(jobID string) {
	if j := scanner.GetJob(jobID); j != nil {
		j.Resume()
	}
}

// CancelJob 取消任务
func (a *App) CancelJob(jobID string) {
	if j := scanner.GetJob(jobID); j != nil {
		j.Cancel()
	}
}

// ── rustscan 端口扫描 APIs ──────────────────────────────────────────────

// RunRustscan 启动端口扫描
func (a *App) RunRustscan(projectID int64, cfg scanner.RustscanConfig) (string, error) {
	jobID := fmt.Sprintf("rustscan-%d-%d", projectID, randInt())
	go func() {
		defer func() {
			if r := recover(); r != nil {
				wruntime.EventsEmit(a.ctx, "rustscan:error", fmt.Sprintf("内部错误: %v", r))
			}
		}()
		if err := scanner.RunRustscan(a.ctx, jobID, projectID, cfg); err != nil {
			wruntime.EventsEmit(a.ctx, "rustscan:error", err.Error())
		}
	}()
	return jobID, nil
}

// RunSubdomain 启动子域名探测
func (a *App) RunSubdomain(projectID int64, cfg scanner.SubdomainConfig) (string, error) {
	jobID := fmt.Sprintf("subdomain-%d-%d", projectID, randInt())
	go func() {
		defer func() {
			if r := recover(); r != nil {
				wruntime.EventsEmit(a.ctx, "subdomain:error", fmt.Sprintf("内部错误: %v", r))
			}
		}()
		if err := scanner.RunSubdomain(a.ctx, jobID, projectID, cfg); err != nil {
			wruntime.EventsEmit(a.ctx, "subdomain:error", err.Error())
		}
	}()
	return jobID, nil
}

// RunDns 启动批量 DNS 解析
func (a *App) RunDns(projectID int64, cfg scanner.DnsConfig) (string, error) {
	jobID := fmt.Sprintf("dns-%d-%d", projectID, randInt())
	go func() {
		defer func() {
			if r := recover(); r != nil {
				wruntime.EventsEmit(a.ctx, "dns:error", fmt.Sprintf("内部错误: %v", r))
			}
		}()
		if err := scanner.RunDns(a.ctx, jobID, projectID, cfg); err != nil {
			wruntime.EventsEmit(a.ctx, "dns:error", err.Error())
		}
	}()
	return jobID, nil
}

// RunNaabu 启动 naabu 端口扫描
func (a *App) RunNaabu(projectID int64, cfg scanner.NaabuConfig) (string, error) {
	jobID := fmt.Sprintf("naabu-%d-%d", projectID, randInt())
	go func() {
		defer func() {
			if r := recover(); r != nil {
				wruntime.EventsEmit(a.ctx, "naabu:error", fmt.Sprintf("内部错误: %v", r))
			}
		}()
		if err := scanner.RunNaabu(a.ctx, jobID, projectID, cfg); err != nil {
			wruntime.EventsEmit(a.ctx, "naabu:error", err.Error())
		}
	}()
	return jobID, nil
}

func randInt() int64 {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	return int64(uint64(b[0])<<56 | uint64(b[1])<<48 | uint64(b[2])<<40 | uint64(b[3])<<32 |
		uint64(b[4])<<24 | uint64(b[5])<<16 | uint64(b[6])<<8 | uint64(b[7]))
}
