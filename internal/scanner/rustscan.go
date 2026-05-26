package scanner

import (
	"asset_tool_go/internal/db"
	"bufio"
	"context"
	"fmt"
	"os/exec"
	"regexp"
	"sort"
	"strconv"
	"strings"

	wruntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

// RustscanConfig rustscan 参数
type RustscanConfig struct {
	RustscanPath string `json:"rustscan_path"`
	Ports        string `json:"ports"`         // "1-65535" 或 "80,443"
	Ulimit       int    `json:"ulimit"`        // 文件描述符上限
	BatchSize    int    `json:"batch_size"`    // 每批端口数
	Timeout      int    `json:"timeout"`       // ms
	Tries        int    `json:"tries"`         // 重试次数
	NoCDN        bool   `json:"no_cdn"`        // 跳过 CDN
	OnlyIP       bool   `json:"only_ip"`       // 只扫 IP
	OnlyDomain   bool   `json:"only_domain"`   // 只扫域名资产
	OnlyAlive    bool   `json:"only_alive"`    // 只扫 alive 资产
	NoBanner       bool   `json:"no_banner"`       // 不输出 ascii 横幅（accessible 模式）
	SkipDnsFailed  bool   `json:"skip_dns_failed"` // 跳过 DNS 解析失败的域名
	UseIPPorts     bool   `json:"use_ip_ports"`    // 使用 IP 扫描已发现的端口
	DomainNetwork  string `json:"domain_network"`  // 域名网络过滤: "" | "extranet" | "intranet"
}

var openPortRe = regexp.MustCompile(`Open\s+([\d.]+):(\d+)`)

// RunRustscan 端口扫描
func RunRustscan(appCtx context.Context, jobID string, projectID int64, cfg RustscanConfig) error {
	// 收集要扫描的目标
	var hosts []string

	if cfg.OnlyDomain && cfg.DomainNetwork != "" {
		var err error
		hosts, err = db.GetDomainHostsByNetwork(projectID, cfg.DomainNetwork)
		if err != nil {
			return fmt.Errorf("get domain hosts by network: %w", err)
		}
		if len(hosts) == 0 {
			wruntime.EventsEmit(appCtx, "rustscan:log", "[!] 没有符合条件的域名，请先运行 DNS 解析")
		}
	} else {
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

		// 去重 host（同一个 host 可能因端口已分多条）
		hostSet := map[string]struct{}{}
		for _, a := range assets {
			hostSet[a.Host] = struct{}{}
		}
		hosts = make([]string, 0, len(hostSet))
		for h := range hostSet {
			hosts = append(hosts, h)
		}
	}
	sort.Strings(hosts)

	if len(hosts) == 0 {
		wruntime.EventsEmit(appCtx, "rustscan:done", map[string]any{"done": 0, "total": 0, "new": 0})
		return nil
	}

	if cfg.UseIPPorts {
		ipPorts, err := db.GetIPPorts(projectID)
		if err != nil {
			return fmt.Errorf("get ip ports: %w", err)
		}
		if ipPorts == "" {
			wruntime.EventsEmit(appCtx, "rustscan:log", "[!] 未找到 IP 已发现的端口，跳过扫描")
			wruntime.EventsEmit(appCtx, "rustscan:done", map[string]any{"done": 0, "total": 0, "new": 0})
			return nil
		}
		cfg.Ports = ipPorts
		wruntime.EventsEmit(appCtx, "rustscan:log", fmt.Sprintf("[i] 使用 IP 已发现端口: %s", ipPorts))
	}

	ctx, cancel := context.WithCancel(appCtx)
	defer cancel()
	job := NewJob(jobID, cancel)
	RegisterJob(job)
	defer UnregisterJob(jobID)

	wruntime.EventsEmit(appCtx, "rustscan:start", map[string]any{
		"total":  len(hosts),
		"job_id": jobID,
	})

	totalNew := 0
	done := 0
	for _, host := range hosts {
		// 暂停 / 取消
		job.WaitIfPaused(ctx)
		if ctx.Err() != nil {
			break
		}

		ports, err := scanOneHost(ctx, host, cfg, func(line string) {
			wruntime.EventsEmit(appCtx, "rustscan:log", line)
		})
		if err != nil {
			wruntime.EventsEmit(appCtx, "rustscan:log", fmt.Sprintf("[!] %s 扫描出错: %v", host, err))
			done++
			wruntime.EventsEmit(appCtx, "rustscan:progress", map[string]any{
				"host":  host,
				"ports": []int{},
				"done":  done,
				"total": len(hosts),
				"new":   0,
			})
			continue
		}

		// 误报保护：>200 个端口直接忽略
		if len(ports) > 200 {
			wruntime.EventsEmit(appCtx, "rustscan:log", fmt.Sprintf(
				"[!] %s 扫到 %d 个端口，疑似误报（建议加大 timeout 或检查目标），已忽略", host, len(ports)))
			done++
			wruntime.EventsEmit(appCtx, "rustscan:progress", map[string]any{
				"host": host, "ports": []int{}, "done": done, "total": len(hosts), "new": 0,
			})
			continue
		}

		var added int
		if len(ports) > 0 {
			var dbErr error
			added, dbErr = db.AddPortAssets(projectID, host, ports, "rustscan")
			if dbErr != nil {
				wruntime.EventsEmit(appCtx, "rustscan:log", fmt.Sprintf("[!] %s 写入端口资产失败: %v", host, dbErr))
			}
			totalNew += added
			wruntime.EventsEmit(appCtx, "rustscan:log",
				fmt.Sprintf("[+] %s 开放 %d 个端口 (新增 %d): %s",
					host, len(ports), added, intsJoin(ports, ",")))
		} else {
			wruntime.EventsEmit(appCtx, "rustscan:log", fmt.Sprintf("[-] %s 无开放端口", host))
		}

		done++
		wruntime.EventsEmit(appCtx, "rustscan:progress", map[string]any{
			"host":  host,
			"ports": ports,
			"done":  done,
			"total": len(hosts),
			"new":   added,
		})
	}

	cancelled := ctx.Err() != nil
	wruntime.EventsEmit(appCtx, "rustscan:done", map[string]any{
		"done":      done,
		"total":     len(hosts),
		"new":       totalNew,
		"cancelled": cancelled,
	})
	return nil
}

// scanOneHost 扫描一个 host
func scanOneHost(ctx context.Context, host string, cfg RustscanConfig, logFn func(string)) ([]int, error) {
	args := []string{
		"-a", host,
		"--no-config",
		"--scripts", "none", // 跳过 nmap 调用
	}
	if cfg.Ulimit > 0 {
		args = append(args, "--ulimit", strconv.Itoa(cfg.Ulimit))
	}
	if cfg.BatchSize > 0 {
		args = append(args, "-b", strconv.Itoa(cfg.BatchSize))
	}
	if cfg.Timeout > 0 {
		args = append(args, "-t", strconv.Itoa(cfg.Timeout))
	}
	if cfg.Tries > 0 {
		args = append(args, "--tries", strconv.Itoa(cfg.Tries))
	}
	if cfg.NoBanner {
		args = append(args, "--accessible")
	}

	if p := strings.TrimSpace(cfg.Ports); p != "" {
		// 范围语法 1-65535 用 -r，列表 80,443 用 -p
		if strings.Contains(p, "-") && !strings.Contains(p, ",") {
			args = append(args, "-r", p)
		} else {
			args = append(args, "-p", p)
		}
	}

	logFn(fmt.Sprintf("[*] 扫描 %s", host))
	logFn(fmt.Sprintf("[debug] %s %s", cfg.RustscanPath, strings.Join(args, " ")))

	cmd := exec.CommandContext(ctx, cfg.RustscanPath, args...)
	hideWindow(cmd)
	stderr, _ := cmd.StderrPipe()
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	if err := cmd.Start(); err != nil {
		return nil, err
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

	var ports []int
	sc := bufio.NewScanner(stdout)
	sc.Buffer(make([]byte, 1024*1024), 4*1024*1024)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		logFn("    " + line)
		if m := openPortRe.FindStringSubmatch(line); m != nil {
			if p, err := strconv.Atoi(m[2]); err == nil {
				ports = append(ports, p)
			}
		}
	}
	if err := cmd.Wait(); err != nil {
		logFn(fmt.Sprintf("[!] rustscan exited: %v", err))
	}

	// 去重 + 排序
	uniq := map[int]struct{}{}
	for _, p := range ports {
		uniq[p] = struct{}{}
	}
	out := make([]int, 0, len(uniq))
	for p := range uniq {
		out = append(out, p)
	}
	sort.Ints(out)
	return out, nil
}

func intsJoin(arr []int, sep string) string {
	s := make([]string, len(arr))
	for i, v := range arr {
		s[i] = strconv.Itoa(v)
	}
	return strings.Join(s, sep)
}
