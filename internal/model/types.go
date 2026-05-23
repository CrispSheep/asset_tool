package model

// Project 项目
type Project struct {
	ID          int64  `json:"id"`
	Name        string `json:"name"`
	CreatedAt   string `json:"created_at"`
	AssetCount  int    `json:"asset_count"`
	IPCount     int    `json:"ip_count"`
	DomainCount int    `json:"domain_count"`
	AliveCount  int    `json:"alive_count"`
}

// Asset 资产
type Asset struct {
	ID          int64    `json:"id"`
	ProjectID   int64    `json:"project_id"`
	Type        string   `json:"type"`        // "ip" | "domain"
	Host        string   `json:"host"`
	Port        string   `json:"port"`
	Sources     []string `json:"sources"`
	Tags        []string `json:"tags"`        // 用户自定义标签
	ResolvedIPs []string `json:"resolved_ips"` // 域名 DNS 解析出的 IP 列表
	Status      string   `json:"status"`      // "alive" | "dead" | ""
	StatusCode  *int     `json:"status_code"`
	Title       string   `json:"title"`
	Server      string   `json:"server"`
	Tech        string   `json:"tech"`        // JSON 字符串
	ProbedAt    string   `json:"probed_at"`
	CreatedAt   string   `json:"created_at"`
}

// AssetEntry 准备入库的资产条目
type AssetEntry struct {
	Host   string `json:"host"`
	Type   string `json:"type"`
	Port   string `json:"port"`
	Source string `json:"source"`
}

// ImportResult CSV 导入结果
type ImportResult struct {
	NewIP     int `json:"new_ip"`
	NewDomain int `json:"new_domain"`
	Skipped   int `json:"skipped"`
}
