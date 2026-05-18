package scanner

import (
	"asset_tool_go/internal/db"
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"sort"
	"strconv"
	"strings"

	wruntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

// NaabuConfig naabu 端口扫描参数
type NaabuConfig struct {
	NaabuPath   string `json:"naabu_path"`
	Ports       string `json:"ports"`        // "80,443,1-1000" 或 "-" 全端口
	Rate        int    `json:"rate"`         // 每秒数据包数
	Concurrency int    `json:"concurrency"`  // 并发主机数
	Timeout     int    `json:"timeout"`      // 单端口超时 ms
	Retries     int    `json:"retries"`      // 重试次数
	ScanType    string `json:"scan_type"`    // "s" SYN 或 "c" CONNECT
	ExcludeCDN  bool   `json:"exclude_cdn"`  // 跳过 CDN IP
	Verify      bool   `json:"verify"`       // 二次确认（更准但慢）
	OnlyIP      bool   `json:"only_ip"`      // 只扫 IP 资产
	OnlyDomain  bool   `json:"only_domain"`  // 只扫域名资产
	OnlyAlive   bool   `json:"only_alive"`   // 只扫 alive
	SkipDnsFailed bool `json:"skip_dns_failed"` // 跳过 DNS 解析失败的域名
	UseIPPorts  bool   `json:"use_ip_ports"`  // 使用 IP 扫描已发现的端口
}

// RunNaabu 执行 naabu 端口扫描
func RunNaabu(appCtx context.Context, jobID string, projectID int64, cfg NaabuConfig) error {
	typeFilter := ""
	if cfg.OnlyDomain {
		typeFilter = "domain"
	} else if cfg.OnlyIP {
		typeFilter = "ip"
	}
	statusFilter := ""
	if cfg.OnlyAlive {
		statusFilter = "alive"
	}
	assets, err := db.ListAssets(projectID, typeFilter, statusFilter, cfg.SkipDnsFailed)
	if err != nil {
		return fmt.Errorf("list assets: %w", err)
	}

	hostSet := map[string]struct{}{}
	for _, a := range assets {
		hostSet[a.Host] = struct{}{}
	}
	hosts := make([]string, 0, len(hostSet))
	for h := range hostSet {
		hosts = append(hosts, h)
	}
	sort.Strings(hosts)

	if len(hosts) == 0 {
		wruntime.EventsEmit(appCtx, "naabu:done", map[string]any{"hosts": 0, "ports": 0, "new": 0})
		return nil
	}

	tmp, err := os.CreateTemp("", "naabu_in_*.txt")
	if err != nil {
		return fmt.Errorf("temp file: %w", err)
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)
	tmp.WriteString(strings.Join(hosts, "\n"))
	tmp.Close()

	args := []string{
		"-list", tmpPath,
		"-json", "-silent",
		"-rate", strconv.Itoa(cfg.Rate),
		"-c", strconv.Itoa(cfg.Concurrency),
		"-timeout", strconv.Itoa(cfg.Timeout),
		"-retries", strconv.Itoa(cfg.Retries),
	}
	if cfg.UseIPPorts {
		ipPorts, err := db.GetIPPorts(projectID)
		if err != nil {
			return fmt.Errorf("get ip ports: %w", err)
		}
		if ipPorts == "" {
			wruntime.EventsEmit(appCtx, "naabu:log", "[!] 未找到 IP 已发现的端口，跳过扫描")
			wruntime.EventsEmit(appCtx, "naabu:done", map[string]any{"hosts": 0, "ports": 0, "new": 0})
			return nil
		}
		args = append(args, "-p", ipPorts)
		wruntime.EventsEmit(appCtx, "naabu:log", fmt.Sprintf("[i] 使用 IP 已发现端口: %s", ipPorts))
	} else if p := strings.TrimSpace(cfg.Ports); p != "" {
		args = append(args, "-p", p)
	}
	if st := strings.TrimSpace(cfg.ScanType); st != "" {
		args = append(args, "-scan-type", st)
	}
	if cfg.ExcludeCDN {
		args = append(args, "-exclude-cdn")
	}
	if cfg.Verify {
		args = append(args, "-verify")
	}

	ctx, cancel := context.WithCancel(appCtx)
	defer cancel()
	job := NewJob(jobID, cancel)
	RegisterJob(job)
	defer UnregisterJob(jobID)

	wruntime.EventsEmit(appCtx, "naabu:start", map[string]any{
		"hosts":  len(hosts),
		"job_id": jobID,
	})
	wruntime.EventsEmit(appCtx, "naabu:log",
		fmt.Sprintf("[debug] %s %s", cfg.NaabuPath, strings.Join(args, " ")))

	cmd := exec.CommandContext(ctx, cfg.NaabuPath, args...)
	hideWindow(cmd)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("stdout pipe: %w", err)
	}
	cmd.Stderr = nil

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start naabu: %w (path=%s)", err, cfg.NaabuPath)
	}

	// 流式读 JSON
	hostPorts := map[string][]int{}
	totalPorts := 0

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
			// 不是 JSON 就当日志
			wruntime.EventsEmit(appCtx, "naabu:log", "    "+line)
			continue
		}

		// naabu JSON: {"host":"x.com","ip":"1.2.3.4","port":80,"protocol":"tcp",...}
		host := getStr(v, "host")
		if host == "" {
			host = getStr(v, "ip")
		}
		port := 0
		if pi := getInt(v, "port"); pi != nil {
			port = *pi
		}
		if host == "" || port == 0 {
			continue
		}

		hostPorts[host] = append(hostPorts[host], port)
		totalPorts++
		wruntime.EventsEmit(appCtx, "naabu:port", map[string]any{
			"host":  host,
			"port":  port,
			"count": totalPorts,
		})
	}
	cmd.Wait()

	cancelled := ctx.Err() != nil
	totalNew := 0

	// 写库（每个 host 的端口去重 + 写多条独立资产）
	if !cancelled {
		for host, ports := range hostPorts {
			uniq := map[int]struct{}{}
			for _, p := range ports {
				uniq[p] = struct{}{}
			}
			ps := make([]int, 0, len(uniq))
			for p := range uniq {
				ps = append(ps, p)
			}
			sort.Ints(ps)

			// 误报保护
			if len(ps) > 200 {
				wruntime.EventsEmit(appCtx, "naabu:log", fmt.Sprintf(
					"[!] %s 扫到 %d 个端口，疑似误报，已忽略", host, len(ps)))
				continue
			}

			added, _ := db.AddPortAssets(projectID, host, ps, "naabu")
			totalNew += added
			wruntime.EventsEmit(appCtx, "naabu:log",
				fmt.Sprintf("[+] %s 开放 %d 个端口 (新增 %d): %s",
					host, len(ps), added, intsJoin(ps, ",")))
		}
	}

	wruntime.EventsEmit(appCtx, "naabu:done", map[string]any{
		"hosts":     len(hostPorts),
		"ports":     totalPorts,
		"new":       totalNew,
		"cancelled": cancelled,
	})
	return nil
}
