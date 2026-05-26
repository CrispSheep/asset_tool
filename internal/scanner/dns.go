package scanner

import (
	"asset_tool_go/internal/db"
	"context"
	"fmt"
	"net"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	wruntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

// DnsConfig DNS 批量解析配置
type DnsConfig struct {
	Concurrency int    `json:"concurrency"` // 并发数，默认 20
	Timeout     int    `json:"timeout"`     // 每个域名超时秒数，默认 5
	DnsServer   string `json:"dns_server"`  // 自定义 DNS 服务器，如 8.8.8.8:53，留空用系统默认
}

// dnsResult 单个域名的解析结果
type dnsResult struct {
	Domain  string
	IPs     []string
	Success bool
	Err     string
}

// RunDns 批量 DNS 解析
func RunDns(appCtx context.Context, jobID string, projectID int64, cfg DnsConfig) error {
	// 取所有域名资产
	domains, err := db.GetDomainHosts(projectID)
	if err != nil {
		return fmt.Errorf("获取域名列表: %w", err)
	}
	if len(domains) == 0 {
		wruntime.EventsEmit(appCtx, "dns:done", map[string]any{"total": 0, "resolved": 0, "failed": 0, "new_ip": 0})
		return nil
	}

	// 参数默认值
	concurrency := cfg.Concurrency
	if concurrency <= 0 {
		concurrency = 20
	}
	timeout := cfg.Timeout
	if timeout <= 0 {
		timeout = 5
	}

	// 创建 job
	ctx, cancel := context.WithCancel(appCtx)
	defer cancel()
	job := NewJob(jobID, cancel)
	RegisterJob(job)
	defer UnregisterJob(jobID)

	wruntime.EventsEmit(appCtx, "dns:start", map[string]any{
		"total":  len(domains),
		"job_id": jobID,
	})

	// 自定义 DNS resolver
	var resolver *net.Resolver
	if cfg.DnsServer != "" {
		server := cfg.DnsServer
		if !strings.Contains(server, ":") {
			server += ":53"
		}
		resolver = &net.Resolver{
			PreferGo: true,
			Dial: func(ctx context.Context, network, address string) (net.Conn, error) {
				d := net.Dialer{Timeout: time.Duration(timeout) * time.Second}
				return d.DialContext(ctx, "udp", server)
			},
		}
	} else {
		resolver = net.DefaultResolver
	}

	// 工作通道
	domainCh := make(chan string, len(domains))
	resultCh := make(chan dnsResult, len(domains))

	for _, d := range domains {
		domainCh <- d
	}
	close(domainCh)

	// 统计
	var processed int64
	var resolved int64
	var failed int64
	var newIP int64

	// 启动 worker
	var wg sync.WaitGroup
	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for domain := range domainCh {
				job.WaitIfPaused(ctx)
				if ctx.Err() != nil {
					return
				}

				resolveCtx, resolveCancel := context.WithTimeout(ctx, time.Duration(timeout)*time.Second)
				ips, err := resolver.LookupHost(resolveCtx, domain)
				resolveCancel()

				res := dnsResult{Domain: domain}
				if err != nil {
					res.Success = false
					res.Err = err.Error()
				} else {
					// 过滤只保留 IPv4
					var v4 []string
					for _, ip := range ips {
						if net.ParseIP(ip) != nil && strings.Contains(ip, ".") {
							v4 = append(v4, ip)
						}
					}
					if len(v4) > 0 {
						res.Success = true
						res.IPs = v4
					} else {
						res.Success = false
						res.Err = "无 IPv4 记录"
					}
				}

				resultCh <- res
			}
		}()
	}

	// 收集结果的 goroutine
	go func() {
		wg.Wait()
		close(resultCh)
	}()

	// 处理结果
	for res := range resultCh {
		if ctx.Err() != nil {
			break
		}

		cur := atomic.AddInt64(&processed, 1)

		if res.Success {
			atomic.AddInt64(&resolved, 1)
			added, dbErr := db.AddDnsResolvedIPs(projectID, res.Domain, res.IPs)
			if dbErr != nil {
				wruntime.EventsEmit(appCtx, "dns:progress", map[string]any{
					"domain":  res.Domain,
					"error":   fmt.Sprintf("写入数据库失败: %v", dbErr),
					"success": false,
				})
			}
			atomic.AddInt64(&newIP, int64(added))

			wruntime.EventsEmit(appCtx, "dns:progress", map[string]any{
				"domain":    res.Domain,
				"ips":       res.IPs,
				"success":   true,
				"processed": cur,
				"total":     len(domains),
				"resolved":  atomic.LoadInt64(&resolved),
				"failed":    atomic.LoadInt64(&failed),
				"new_ip":    atomic.LoadInt64(&newIP),
			})
		} else {
			atomic.AddInt64(&failed, 1)
			if dbErr := db.TagDnsFailed(projectID, res.Domain); dbErr != nil {
				wruntime.EventsEmit(appCtx, "dns:progress", map[string]any{
					"domain":  res.Domain,
					"error":   fmt.Sprintf("标记 DNS 失败标签出错: %v", dbErr),
					"success": false,
				})
			}

			wruntime.EventsEmit(appCtx, "dns:progress", map[string]any{
				"domain":    res.Domain,
				"error":     res.Err,
				"success":   false,
				"processed": cur,
				"total":     len(domains),
				"resolved":  atomic.LoadInt64(&resolved),
				"failed":    atomic.LoadInt64(&failed),
				"new_ip":    atomic.LoadInt64(&newIP),
			})
		}
	}

	cancelled := ctx.Err() != nil
	wruntime.EventsEmit(appCtx, "dns:done", map[string]any{
		"total":     len(domains),
		"resolved":  atomic.LoadInt64(&resolved),
		"failed":    atomic.LoadInt64(&failed),
		"new_ip":    atomic.LoadInt64(&newIP),
		"cancelled": cancelled,
	})
	return nil
}
