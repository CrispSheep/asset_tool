package scanner

import (
	"asset_tool_go/internal/db"
	"asset_tool_go/internal/model"
	"bufio"
	"context"
	"encoding/csv"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	wruntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

// SubdomainConfig 子域名探测配置
type SubdomainConfig struct {
	Tool       string   `json:"tool"`        // "ksubdomain" | "oneforall" | "subfinder"
	ToolPath   string   `json:"tool_path"`
	PythonPath string   `json:"python_path"` // 仅 oneforall 用，留空走系统 python
	Domains    []string `json:"domains"`
	Threads    int      `json:"threads"`     // subfinder 并发数，默认 10
	Timeout    int      `json:"timeout"`     // subfinder 超时秒数，默认 30
	All        bool     `json:"all"`         // subfinder -all 使用所有源
}

// RunSubdomain 执行子域名探测
func RunSubdomain(appCtx context.Context, jobID string, projectID int64, cfg SubdomainConfig) error {
	if len(cfg.Domains) == 0 {
		wruntime.EventsEmit(appCtx, "subdomain:done", map[string]any{"new": 0})
		return nil
	}

	ctx, cancel := context.WithCancel(appCtx)
	defer cancel()
	job := NewJob(jobID, cancel)
	RegisterJob(job)
	defer UnregisterJob(jobID)

	wruntime.EventsEmit(appCtx, "subdomain:start", map[string]any{
		"total":  len(cfg.Domains),
		"job_id": jobID,
	})

	totalNew := 0
	source := cfg.Tool

	for i, domain := range cfg.Domains {
		job.WaitIfPaused(ctx)
		if ctx.Err() != nil {
			break
		}

		wruntime.EventsEmit(appCtx, "subdomain:log", fmt.Sprintf("[*] 开始探测: %s", domain))

		var subs []string
		var err error
		switch cfg.Tool {
		case "ksubdomain":
			subs, err = runKsubdomain(ctx, cfg, domain, func(line string) {
				wruntime.EventsEmit(appCtx, "subdomain:log", "    "+line)
			})
		case "oneforall":
			subs, err = runOneforall(ctx, cfg, domain, func(line string) {
				wruntime.EventsEmit(appCtx, "subdomain:log", "    "+line)
			})
		case "subfinder":
			subs, err = runSubfinder(ctx, cfg, domain, func(line string) {
				wruntime.EventsEmit(appCtx, "subdomain:log", "    "+line)
			})
		default:
			err = fmt.Errorf("未知工具: %s", cfg.Tool)
		}

		if err != nil {
			wruntime.EventsEmit(appCtx, "subdomain:log", fmt.Sprintf("[!] %s 失败: %v", domain, err))
		} else {
			// 逐条发射 found 事件，让前端实时展示
			for _, s := range subs {
				wruntime.EventsEmit(appCtx, "subdomain:found", s)
			}
			added := saveSubdomains(projectID, subs, source)
			totalNew += added
			wruntime.EventsEmit(appCtx, "subdomain:log",
				fmt.Sprintf("[+] %s 完成，发现 %d 条，新增 %d 条", domain, len(subs), added))
		}

		wruntime.EventsEmit(appCtx, "subdomain:progress", map[string]any{
			"done":  i + 1,
			"total": len(cfg.Domains),
			"new":   totalNew,
		})
	}

	cancelled := ctx.Err() != nil
	wruntime.EventsEmit(appCtx, "subdomain:done", map[string]any{
		"new":       totalNew,
		"cancelled": cancelled,
	})
	return nil
}

// ── ksubdomain ────────────────────────────────────────────────────

func runKsubdomain(ctx context.Context, cfg SubdomainConfig, domain string, logFn func(string)) ([]string, error) {
	tmp, err := os.CreateTemp("", "ksubdomain_*.txt")
	if err != nil {
		return nil, err
	}
	outPath := tmp.Name()
	tmp.Close()
	defer os.Remove(outPath)

	args := []string{"e", "-d", domain, "-o", outPath, "--silent"}
	logFn(fmt.Sprintf("[debug] %s %s", cfg.ToolPath, strings.Join(args, " ")))

	cmd := exec.CommandContext(ctx, cfg.ToolPath, args...)
	hideWindow(cmd)
	stdout, _ := cmd.StdoutPipe()
	cmd.Stderr = cmd.Stdout
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("启动失败: %w", err)
	}
	streamLog(stdout, logFn)
	if err := cmd.Wait(); err != nil {
		logFn(fmt.Sprintf("[!] ksubdomain exited: %v", err))
	}

	lines := readLines(outPath)
	return parseKsubdomainLines(lines, maxSubdomainResults), nil
}

// parseKsubdomainLines 解析 ksubdomain 输出行
// 格式可能是 "sub.example.com" 或 "sub.example.com => 1.2.3.4"
func parseKsubdomainLines(lines []string, limit int) []string {
	var out []string
	for _, l := range lines {
		if idx := strings.Index(l, "=>"); idx != -1 {
			l = strings.TrimSpace(l[:idx])
		}
		if l != "" {
			out = append(out, l)
			if limit > 0 && len(out) >= limit {
				break
			}
		}
	}
	return out
}

// ── oneforall ─────────────────────────────────────────────────────

func runOneforall(ctx context.Context, cfg SubdomainConfig, domain string, logFn func(string)) ([]string, error) {
	pyPath := cfg.PythonPath
	if pyPath == "" {
		pyPath = "python"
	}
	toolDir := filepath.Dir(cfg.ToolPath)

	args := []string{cfg.ToolPath, "--target", domain, "run"}
	logFn(fmt.Sprintf("[debug] %s %s (cwd=%s)", pyPath, strings.Join(args, " "), toolDir))

	cmd := exec.CommandContext(ctx, pyPath, args...)
	cmd.Dir = toolDir
	hideWindow(cmd)
	stdout, _ := cmd.StdoutPipe()
	cmd.Stderr = cmd.Stdout
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("启动失败: %w", err)
	}
	streamLog(stdout, logFn)
	if err := cmd.Wait(); err != nil {
		logFn(fmt.Sprintf("[!] oneforall exited: %v", err))
	}

	// 读取 results/<domain>.csv
	resultPath := filepath.Join(toolDir, "results", domain+".csv")
	f, err := os.Open(resultPath)
	if err != nil {
		return nil, fmt.Errorf("找不到结果文件 %s: %w", resultPath, err)
	}
	defer f.Close()

	r := csv.NewReader(f)
	r.FieldsPerRecord = -1
	r.LazyQuotes = true
	records, err := r.ReadAll()
	if err != nil || len(records) < 2 {
		return nil, fmt.Errorf("读取 csv 失败: %w", err)
	}

	// 找 subdomain 列
	header := records[0]
	idx := -1
	for i, h := range header {
		hl := strings.ToLower(strings.TrimSpace(h))
		if hl == "subdomain" || hl == "host" {
			idx = i
			break
		}
	}
	if idx == -1 {
		return nil, fmt.Errorf("结果 csv 中找不到 subdomain 列")
	}

	subs := []string{}
	for _, row := range records[1:] {
		if len(row) > idx {
			s := strings.TrimSpace(row[idx])
			if s != "" {
				subs = append(subs, s)
				if len(subs) >= maxSubdomainResults {
					break
				}
			}
		}
	}
	return subs, nil
}

// ── subfinder ────────────────────────────────────────────────────

func runSubfinder(ctx context.Context, cfg SubdomainConfig, domain string, logFn func(string)) ([]string, error) {
	args := []string{"-d", domain, "-silent"}

	threads := cfg.Threads
	if threads <= 0 {
		threads = 10
	}
	args = append(args, "-t", fmt.Sprintf("%d", threads))

	timeout := cfg.Timeout
	if timeout <= 0 {
		timeout = 30
	}
	args = append(args, "-timeout", fmt.Sprintf("%d", timeout))

	if cfg.All {
		args = append(args, "-all")
	}

	logFn(fmt.Sprintf("[debug] %s %s", cfg.ToolPath, strings.Join(args, " ")))

	cmd := exec.CommandContext(ctx, cfg.ToolPath, args...)
	hideWindow(cmd)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("stdout pipe: %w", err)
	}
	stderr, _ := cmd.StderrPipe()
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("启动失败: %w", err)
	}

	// 读取 stderr（后台）
	go func() {
		sc := bufio.NewScanner(stderr)
		sc.Buffer(make([]byte, 1024*1024), 4*1024*1024)
		for sc.Scan() {
			line := strings.TrimSpace(sc.Text())
			if line != "" {
				logFn("    [stderr] " + line)
			}
		}
	}()

	// subfinder -silent 每行一个子域名
	var subs []string
	seen := map[string]struct{}{}
	sc := bufio.NewScanner(stdout)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		logFn(line)
		if _, ok := seen[line]; !ok {
			seen[line] = struct{}{}
			subs = append(subs, line)
			if len(subs) >= maxSubdomainResults {
				logFn(fmt.Sprintf("[!] 已达上限 %d 条，停止收集", maxSubdomainResults))
				break
			}
		}
	}
	if err := cmd.Wait(); err != nil {
		logFn(fmt.Sprintf("[!] subfinder exited: %v", err))
	}
	return subs, nil
}

// ── 公共 ──────────────────────────────────────────────────────────

func streamLog(r interface{ Read([]byte) (int, error) }, logFn func(string)) {
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 1024*1024), 4*1024*1024)
	for sc.Scan() {
		line := strings.TrimRight(sc.Text(), "\r\n")
		if line != "" {
			logFn(line)
		}
	}
}

func readLines(path string) []string {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	var out []string
	for _, l := range strings.Split(string(b), "\n") {
		l = strings.TrimSpace(l)
		if l != "" {
			out = append(out, l)
		}
	}
	return out
}

const maxSubdomainResults = 10000

func saveSubdomains(projectID int64, subs []string, source string) int {
	if len(subs) == 0 {
		return 0
	}
	if len(subs) > maxSubdomainResults {
		subs = subs[:maxSubdomainResults]
	}
	entries := make([]model.AssetEntry, 0, len(subs))
	for _, s := range subs {
		entries = append(entries, model.AssetEntry{
			Host:   s,
			Type:   "domain",
			Source: source,
		})
	}
	res, err := db.UpsertAssets(projectID, entries)
	if err != nil {
		return 0
	}
	return res.NewIP + res.NewDomain
}
