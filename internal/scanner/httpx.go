package scanner

import (
	"asset_tool_go/internal/db"
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"

	wruntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

// HttpxConfig 探活配置
type HttpxConfig struct {
	HttpxPath          string `json:"httpx_path"`
	Threads            int    `json:"threads"`
	Timeout            int    `json:"timeout"`
	Retries            int    `json:"retries"`
	RateLimit          int    `json:"rate_limit"`
	ProbeTitle         bool   `json:"probe_title"`
	ProbeTech          bool   `json:"probe_tech"`
	ProbeServer        bool   `json:"probe_server"`
	ProbeContentLength bool   `json:"probe_content_length"`
	ProbeIP            bool   `json:"probe_ip"`
	ProbeCDN           bool   `json:"probe_cdn"`
	FollowRedirects    bool   `json:"follow_redirects"`
	MatchCodes         string `json:"match_codes"`
	FilterCodes        string `json:"filter_codes"`
	OnlyUnprobed       bool   `json:"only_unprobed"`    // 仅探活未探测的资产
	OnlyIP             bool   `json:"only_ip"`          // 只探活 IP 资产
	OnlyDomain         bool   `json:"only_domain"`      // 只探活域名资产
	SkipDnsFailed      bool   `json:"skip_dns_failed"`  // 跳过 DNS 解析失败的域名
}

// RunHttpx 执行探活，进度通过 EventsEmit 推送
func RunHttpx(appCtx context.Context, jobID string, projectID int64, cfg HttpxConfig) error {
	var hosts []string
	var err error
	typeFilter := ""
	if cfg.OnlyDomain {
		typeFilter = "domain"
	} else if cfg.OnlyIP {
		typeFilter = "ip"
	}
	if cfg.OnlyUnprobed {
		// 只取 status 为空的资产
		assets, e := db.ListAssets(projectID, typeFilter, "", cfg.SkipDnsFailed)
		if e != nil {
			return fmt.Errorf("list assets: %w", e)
		}
		for _, a := range assets {
			if a.Status == "" {
				if a.Port != "" {
					hosts = append(hosts, a.Host+":"+a.Port)
				} else {
					hosts = append(hosts, a.Host)
				}
			}
		}
	} else {
		hosts, err = db.GetAllHosts(projectID, typeFilter, cfg.SkipDnsFailed)
		if err != nil {
			return fmt.Errorf("get hosts: %w", err)
		}
	}
	if len(hosts) == 0 {
		wruntime.EventsEmit(appCtx, "httpx:done", map[string]any{"alive": 0, "total": 0})
		return nil
	}

	tmp, err := os.CreateTemp("", "httpx_in_*.txt")
	if err != nil {
		return fmt.Errorf("temp file: %w", err)
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)
	tmp.WriteString(strings.Join(hosts, "\n"))
	tmp.Close()

	args := []string{
		"-l", tmpPath,
		"-json", "-silent",
		"-threads", strconv.Itoa(cfg.Threads),
		"-timeout", strconv.Itoa(cfg.Timeout),
		"-retries", strconv.Itoa(cfg.Retries),
	}
	if cfg.ProbeTitle {
		args = append(args, "-title")
	}
	if cfg.ProbeTech {
		args = append(args, "-tech-detect")
	}
	if cfg.ProbeServer {
		args = append(args, "-server")
	}
	if cfg.ProbeContentLength {
		args = append(args, "-content-length")
	}
	if cfg.ProbeIP {
		args = append(args, "-ip")
	}
	if cfg.ProbeCDN {
		args = append(args, "-cdn")
	}
	if cfg.FollowRedirects {
		args = append(args, "-follow-redirects")
	}
	if cfg.RateLimit > 0 {
		args = append(args, "-rate-limit", strconv.Itoa(cfg.RateLimit))
	}
	if mc := strings.TrimSpace(cfg.MatchCodes); mc != "" {
		args = append(args, "-mc", mc)
	}
	if fc := strings.TrimSpace(cfg.FilterCodes); fc != "" {
		args = append(args, "-fc", fc)
	}

	ctx, cancel := context.WithCancel(appCtx)
	defer cancel()
	job := NewJob(jobID, cancel)
	RegisterJob(job)
	defer UnregisterJob(jobID)

	wruntime.EventsEmit(appCtx, "httpx:start", map[string]any{"total": len(hosts), "job_id": jobID})

	cmd := exec.CommandContext(ctx, cfg.HttpxPath, args...)
	hideWindow(cmd)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("stdout pipe: %w", err)
	}

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start httpx: %w (path=%s)", err, cfg.HttpxPath)
	}

	total := len(hosts)
	processed := 0
	alive := 0
	responded := map[string]struct{}{}

	sc := bufio.NewScanner(stdout)
	sc.Buffer(make([]byte, 1024*1024), 4*1024*1024)

	for sc.Scan() {
		if ctx.Err() != nil {
			break
		}
		job.WaitIfPaused(ctx)

		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		var v map[string]any
		if err := json.Unmarshal([]byte(line), &v); err != nil {
			continue
		}

		host := getStr(v, "input")
		dbHost, dbPort := splitHostPort(host)
		statusCode := getInt(v, "status-code", "status_code")
		processed++
		if statusCode != nil {
			alive++
			responded[host] = struct{}{}
			title := getStr(v, "title")
			server := firstNonEmpty(getStr(v, "webserver"), getStr(v, "server"))
			tech := ""
			if t, ok := v["tech"].([]any); ok {
				if b, err := json.Marshal(t); err == nil {
					tech = string(b)
				}
			}
			_ = db.UpdateProbeResult(projectID, dbHost, dbPort, "alive", statusCode, title, server, tech)
		}

		wruntime.EventsEmit(appCtx, "httpx:progress", map[string]any{
			"processed":   processed,
			"total":       total,
			"host":        host,
			"status_code": statusCode,
			"alive":       alive,
		})
	}
	cmd.Wait()

	cancelled := ctx.Err() != nil

	if !cancelled {
		for _, h := range hosts {
			if _, ok := responded[h]; ok {
				continue
			}
			processed++
			dbHost, dbPort := splitHostPort(h)
			_ = db.UpdateProbeResult(projectID, dbHost, dbPort, "dead", nil, "", "", "[]")
			wruntime.EventsEmit(appCtx, "httpx:progress", map[string]any{
				"processed":   processed,
				"total":       total,
				"host":        h,
				"status_code": nil,
				"alive":       alive,
			})
		}
	}

	wruntime.EventsEmit(appCtx, "httpx:done", map[string]any{
		"alive":     alive,
		"total":     total,
		"cancelled": cancelled,
	})
	return nil
}

// ── helpers ──────────────────────────────────────────────────────────

func getStr(m map[string]any, key string) string {
	if v, ok := m[key]; ok {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}

func getInt(m map[string]any, keys ...string) *int {
	for _, k := range keys {
		if v, ok := m[k]; ok {
			switch x := v.(type) {
			case float64:
				i := int(x)
				return &i
			case int:
				return &x
			case string:
				if i, err := strconv.Atoi(x); err == nil {
					return &i
				}
			}
		}
	}
	return nil
}

func firstNonEmpty(ss ...string) string {
	for _, s := range ss {
		if s != "" {
			return s
		}
	}
	return ""
}

// splitHostPort 把 host:port 拆分为 host 和 port（用于数据库匹配）
// 如果没有端口，返回原始字符串和空端口
func splitHostPort(s string) (string, string) {
	idx := strings.LastIndex(s, ":")
	if idx == -1 {
		return s, ""
	}
	port := s[idx+1:]
	if _, err := strconv.Atoi(port); err != nil {
		return s, ""
	}
	return s[:idx], port
}
