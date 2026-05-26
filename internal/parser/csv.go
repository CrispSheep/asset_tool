package parser

import (
	"asset_tool_go/internal/model"
	"encoding/csv"
	"io"
	"net/url"
	"os"
	"regexp"
	"strconv"
	"strings"

	"golang.org/x/text/encoding/simplifiedchinese"
	"golang.org/x/text/transform"
)

var (
	ipRe     = regexp.MustCompile(`^(\d{1,3})\.(\d{1,3})\.(\d{1,3})\.(\d{1,3})$`)
	domainRe = regexp.MustCompile(`^(\*\.)?([a-zA-Z0-9]([a-zA-Z0-9\-]{0,61}[a-zA-Z0-9])?\.)+[a-zA-Z]{2,}$`)
	urlRe    = regexp.MustCompile(`https?://[^\s,"'<>]+`)
	splitRe  = regexp.MustCompile(`[\s,;|"'\[\]<>]+`)
	hpRe     = regexp.MustCompile(`^(.+):(\d{1,5})$`)
)

// CSVStats 解析统计
type CSVStats struct {
	TotalRows int `json:"total_rows"`
	IP        int `json:"ip"`
	Domain    int `json:"domain"`
}

// ParseCSV 解析 CSV 文件，提取 IP / 域名 / URL，返回去重后的资产条目
func ParseCSV(path, source string) ([]model.AssetEntry, CSVStats, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, CSVStats{}, err
	}
	defer f.Close()

	reader := newAutoDecoder(f)
	r := csv.NewReader(reader)
	r.FieldsPerRecord = -1
	r.LazyQuotes = true

	var entries []model.AssetEntry
	seen := make(map[string]struct{})
	stats := CSVStats{}

	for {
		row, err := r.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			// 跳过坏行
			continue
		}
		stats.TotalRows++
		for _, cell := range row {
			for _, item := range ExtractFromValue(cell) {
				key := item.Host + "|" + item.Port
				if _, ok := seen[key]; ok {
					continue
				}
				seen[key] = struct{}{}
				item.Source = source
				entries = append(entries, item)
				if item.Type == "ip" {
					stats.IP++
				} else {
					stats.Domain++
				}
			}
		}
	}
	return entries, stats, nil
}

// ExtractFromValue 从一个字段中提取所有 IP/域名
func ExtractFromValue(raw string) []model.AssetEntry {
	results := []model.AssetEntry{}
	seen := make(map[string]struct{})

	add := func(host, typ, port string) {
		// 通配符域名 *.example.com → example.com
		host = strings.TrimPrefix(host, "*.")
		if host == "" {
			return
		}
		key := host + "|" + port
		if _, ok := seen[key]; ok {
			return
		}
		seen[key] = struct{}{}
		results = append(results, model.AssetEntry{Host: host, Type: typ, Port: port})
	}

	// 1. URL
	for _, u := range urlRe.FindAllString(raw, -1) {
		parsed, err := url.Parse(u)
		if err != nil {
			continue
		}
		host := strings.Trim(parsed.Hostname(), ".")
		port := parsed.Port()
		if host == "" {
			continue
		}
		if isValidIP(host) {
			add(host, "ip", port)
		} else if domainRe.MatchString(host) {
			add(host, "domain", port)
		}
	}

	// 2. 剩余 token
	remainder := urlRe.ReplaceAllString(raw, " ")
	for _, token := range splitRe.Split(remainder, -1) {
		token = strings.Trim(strings.TrimSpace(token), ".")
		if token == "" {
			continue
		}
		host := token
		port := ""
		if m := hpRe.FindStringSubmatch(token); m != nil {
			if p, err := strconv.Atoi(m[2]); err == nil && p >= 1 && p <= 65535 {
				host = m[1]
				port = m[2]
			}
		}
		host = strings.Trim(host, ".")
		if host == "" {
			continue
		}
		if isValidIP(host) {
			add(host, "ip", port)
		} else if domainRe.MatchString(host) {
			add(host, "domain", port)
		}
	}
	return results
}

func isValidIP(s string) bool {
	m := ipRe.FindStringSubmatch(s)
	if m == nil {
		return false
	}
	for i := 1; i <= 4; i++ {
		v, _ := strconv.Atoi(m[i])
		if v < 0 || v > 255 {
			return false
		}
	}
	return true
}

// newAutoDecoder 自动检测 UTF-8 BOM / GBK
func newAutoDecoder(f *os.File) io.Reader {
	// 读前 4 字节判断 BOM
	bom := make([]byte, 3)
	n, _ := f.Read(bom)
	if n == 3 && bom[0] == 0xEF && bom[1] == 0xBB && bom[2] == 0xBF {
		// utf-8 BOM，跳过这 3 字节
		return f
	}
	// 不是 BOM，回到开头
	if _, err := f.Seek(0, io.SeekStart); err != nil {
		return f
	}
	// 探测前 1KB 是否合法 UTF-8
	probe := make([]byte, 1024)
	n, _ = f.Read(probe)
	probe = probe[:n]
	if _, err := f.Seek(0, io.SeekStart); err != nil {
		return f
	}
	if isValidUTF8(probe) {
		return f
	}
	// fallback GBK
	return transform.NewReader(f, simplifiedchinese.GBK.NewDecoder())
}

func isValidUTF8(b []byte) bool {
	for i := 0; i < len(b); {
		r, size := decodeRune(b[i:])
		if r == 0xFFFD && size == 1 {
			return false
		}
		i += size
	}
	return true
}

func decodeRune(b []byte) (rune, int) {
	if len(b) == 0 {
		return 0, 0
	}
	c := b[0]
	switch {
	case c < 0x80:
		return rune(c), 1
	case c < 0xC2:
		return 0xFFFD, 1
	case c < 0xE0:
		if len(b) < 2 || b[1]&0xC0 != 0x80 {
			return 0xFFFD, 1
		}
		return rune(c&0x1F)<<6 | rune(b[1]&0x3F), 2
	case c < 0xF0:
		if len(b) < 3 || b[1]&0xC0 != 0x80 || b[2]&0xC0 != 0x80 {
			return 0xFFFD, 1
		}
		return rune(c&0x0F)<<12 | rune(b[1]&0x3F)<<6 | rune(b[2]&0x3F), 3
	case c < 0xF5:
		if len(b) < 4 || b[1]&0xC0 != 0x80 || b[2]&0xC0 != 0x80 || b[3]&0xC0 != 0x80 {
			return 0xFFFD, 1
		}
		return rune(c&0x07)<<18 | rune(b[1]&0x3F)<<12 | rune(b[2]&0x3F)<<6 | rune(b[3]&0x3F), 4
	}
	return 0xFFFD, 1
}
