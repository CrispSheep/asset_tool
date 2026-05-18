package db

import (
	"asset_tool_go/internal/model"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
)

// UpsertAssets 批量写入资产；已存在的合并 source 标签
func UpsertAssets(projectID int64, entries []model.AssetEntry) (model.ImportResult, error) {
	res := model.ImportResult{}
	if len(entries) == 0 {
		return res, nil
	}
	tx, err := conn.Begin()
	if err != nil {
		return res, err
	}
	defer tx.Rollback()

	for _, e := range entries {
		host := strings.TrimSpace(e.Host)
		if host == "" {
			continue
		}
		port := strings.TrimSpace(e.Port)

		var existingID int64
		var existingSrcRaw string
		err := tx.QueryRow(
			"SELECT id, sources FROM assets WHERE project_id=? AND host=? AND IFNULL(port,'')=IFNULL(?,'')",
			projectID, host, nullable(port),
		).Scan(&existingID, &existingSrcRaw)

		if err == sql.ErrNoRows {
			srcJSON := "[]"
			if e.Source != "" {
				if b, err := json.Marshal([]string{e.Source}); err == nil {
					srcJSON = string(b)
				}
			}
			_, err := tx.Exec(
				"INSERT INTO assets(project_id, type, host, port, sources) VALUES(?,?,?,?,?)",
				projectID, e.Type, host, nullable(port), srcJSON,
			)
			if err != nil {
				return res, fmt.Errorf("insert: %w", err)
			}
			if e.Type == "ip" {
				res.NewIP++
			} else {
				res.NewDomain++
			}
		} else if err == nil {
			var srcs []string
			_ = json.Unmarshal([]byte(existingSrcRaw), &srcs)
			if e.Source != "" && !contains(srcs, e.Source) {
				srcs = append(srcs, e.Source)
				if b, err := json.Marshal(srcs); err == nil {
					_, _ = tx.Exec("UPDATE assets SET sources=? WHERE id=?", string(b), existingID)
				}
			}
			res.Skipped++
		} else {
			return res, err
		}
	}

	return res, tx.Commit()
}

// AddPortAssets rustscan 等端口扫描结果：每个端口建独立记录
func AddPortAssets(projectID int64, host string, ports []int, source string) (int, error) {
	if len(ports) == 0 {
		return 0, nil
	}
	tx, err := conn.Begin()
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()

	var baseType, baseSrc string
	err = tx.QueryRow(
		"SELECT type, sources FROM assets WHERE project_id=? AND host=? ORDER BY id LIMIT 1",
		projectID, host,
	).Scan(&baseType, &baseSrc)
	if err == sql.ErrNoRows {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}

	var srcs []string
	_ = json.Unmarshal([]byte(baseSrc), &srcs)
	if source != "" && !contains(srcs, source) {
		srcs = append(srcs, source)
	}
	srcJSON, _ := json.Marshal(srcs)

	added := 0
	for _, p := range ports {
		portStr := fmt.Sprintf("%d", p)
		res, err := tx.Exec(
			"INSERT OR IGNORE INTO assets(project_id, type, host, port, sources) VALUES(?,?,?,?,?)",
			projectID, baseType, host, portStr, string(srcJSON),
		)
		if err == nil {
			if n, e := res.RowsAffected(); e == nil && n > 0 {
				added++
			}
		}
	}
	return added, tx.Commit()
}

// ListAssets 列出资产，支持按 type / status 过滤，skipDnsFailed 跳过带 "DNS无效" 标签的
func ListAssets(projectID int64, typeFilter, statusFilter string, skipDnsFailed ...bool) ([]model.Asset, error) {
	q := `SELECT id, project_id, type, host, IFNULL(port,''), sources, IFNULL(tags,'[]'),
	             IFNULL(status,''), status_code, IFNULL(title,''), IFNULL(server,''),
	             IFNULL(tech,''), IFNULL(probed_at,''), created_at
	      FROM assets WHERE project_id=?`
	args := []any{projectID}
	if typeFilter != "" {
		q += " AND type=?"
		args = append(args, typeFilter)
	}
	if statusFilter != "" {
		q += " AND status=?"
		args = append(args, statusFilter)
	}
	if len(skipDnsFailed) > 0 && skipDnsFailed[0] {
		q += ` AND NOT (tags LIKE '%DNS无效%')`
	}
	q += " ORDER BY created_at DESC, id DESC"

	rows, err := conn.Query(q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []model.Asset
	for rows.Next() {
		var a model.Asset
		var srcRaw, tagsRaw string
		var statusCode sql.NullInt64
		err := rows.Scan(
			&a.ID, &a.ProjectID, &a.Type, &a.Host, &a.Port, &srcRaw, &tagsRaw,
			&a.Status, &statusCode, &a.Title, &a.Server,
			&a.Tech, &a.ProbedAt, &a.CreatedAt,
		)
		if err != nil {
			return nil, err
		}
		if statusCode.Valid {
			c := int(statusCode.Int64)
			a.StatusCode = &c
		}
		_ = json.Unmarshal([]byte(srcRaw), &a.Sources)
		if a.Sources == nil {
			a.Sources = []string{}
		}
		_ = json.Unmarshal([]byte(tagsRaw), &a.Tags)
		if a.Tags == nil {
			a.Tags = []string{}
		}
		result = append(result, a)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return result, nil
}

// DeleteAsset 删除单个资产
func DeleteAsset(id int64) error {
	_, err := conn.Exec("DELETE FROM assets WHERE id=?", id)
	return err
}

// UpdateTags 更新资产标签
func UpdateTags(id int64, tags []string) error {
	b, _ := json.Marshal(tags)
	_, err := conn.Exec("UPDATE assets SET tags=? WHERE id=?", string(b), id)
	return err
}

// BatchUpdateTags 批量给资产加标签（追加，不覆盖）
func BatchAddTag(ids []int64, tag string) error {
	if len(ids) == 0 || tag == "" {
		return nil
	}
	tx, err := conn.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for _, id := range ids {
		var raw string
		if err := tx.QueryRow("SELECT IFNULL(tags,'[]') FROM assets WHERE id=?", id).Scan(&raw); err != nil {
			continue
		}
		var tags []string
		_ = json.Unmarshal([]byte(raw), &tags)
		if !contains(tags, tag) {
			tags = append(tags, tag)
			b, _ := json.Marshal(tags)
			tx.Exec("UPDATE assets SET tags=? WHERE id=?", string(b), id)
		}
	}
	return tx.Commit()
}

// BatchRemoveTag 批量移除标签
func BatchRemoveTag(ids []int64, tag string) error {
	if len(ids) == 0 || tag == "" {
		return nil
	}
	tx, err := conn.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for _, id := range ids {
		var raw string
		if err := tx.QueryRow("SELECT IFNULL(tags,'[]') FROM assets WHERE id=?", id).Scan(&raw); err != nil {
			continue
		}
		var tags []string
		_ = json.Unmarshal([]byte(raw), &tags)
		newTags := make([]string, 0, len(tags))
		for _, t := range tags {
			if t != tag {
				newTags = append(newTags, t)
			}
		}
		b, _ := json.Marshal(newTags)
		tx.Exec("UPDATE assets SET tags=? WHERE id=?", string(b), id)
	}
	return tx.Commit()
}

// DeleteAssets 批量删除
func DeleteAssets(ids []int64) error {
	if len(ids) == 0 {
		return nil
	}
	tx, err := conn.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for _, id := range ids {
		if _, err := tx.Exec("DELETE FROM assets WHERE id=?", id); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// GetAllHosts 取项目所有 host[:port] 用于探活/端口扫描输入
func GetAllHosts(projectID int64, typeFilter string, skipDnsFailed ...bool) ([]string, error) {
	q := "SELECT host, IFNULL(port,'') FROM assets WHERE project_id=?"
	args := []any{projectID}
	if typeFilter != "" {
		q += " AND type=?"
		args = append(args, typeFilter)
	}
	if len(skipDnsFailed) > 0 && skipDnsFailed[0] {
		q += ` AND NOT (tags LIKE '%DNS无效%')`
	}
	rows, err := conn.Query(q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var hosts []string
	for rows.Next() {
		var h, p string
		if err := rows.Scan(&h, &p); err != nil {
			return nil, err
		}
		if p != "" {
			hosts = append(hosts, h+":"+p)
		} else {
			hosts = append(hosts, h)
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return hosts, nil
}

// GetIPPorts 取项目中 IP 资产已发现的所有去重端口，返回逗号分隔字符串如 "80,443,8080"
func GetIPPorts(projectID int64) (string, error) {
	q := `SELECT DISTINCT port FROM assets WHERE project_id=? AND type='ip' AND port<>'' ORDER BY CAST(port AS INTEGER)`
	rows, err := conn.Query(q, projectID)
	if err != nil {
		return "", err
	}
	defer rows.Close()
	var ports []string
	for rows.Next() {
		var p string
		if err := rows.Scan(&p); err != nil {
			return "", err
		}
		ports = append(ports, p)
	}
	if err := rows.Err(); err != nil {
		return "", err
	}
	return strings.Join(ports, ","), nil
}

// UpdateProbeResult 写入 httpx 探活结果
// port 为空字符串时匹配无端口记录，非空时精确匹配端口
func UpdateProbeResult(projectID int64, host, port string, status string, code *int, title, server, tech string) error {
	var codeVal any
	if code != nil {
		codeVal = *code
	}
	_, err := conn.Exec(
		`UPDATE assets SET status=?, status_code=?, title=?, server=?, tech=?, probed_at=datetime('now','localtime')
		 WHERE project_id=? AND host=? AND IFNULL(port,'')=?`,
		status, codeVal, title, server, tech, projectID, host, port,
	)
	return err
}

// ── DNS 解析相关 ──────────────────────────────────────────────────────

// GetDomainHosts 取项目所有域名（不含端口），用于 DNS 批量解析
func GetDomainHosts(projectID int64) ([]string, error) {
	rows, err := conn.Query(
		"SELECT DISTINCT host FROM assets WHERE project_id=? AND type='domain' AND IFNULL(port,'')=''",
		projectID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var domains []string
	for rows.Next() {
		var h string
		if err := rows.Scan(&h); err != nil {
			return nil, err
		}
		domains = append(domains, h)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return domains, nil
}

// AddDnsResolvedIPs 将 DNS 解析出的 IP 写入资产表，返回新增数量
func AddDnsResolvedIPs(projectID int64, domain string, ips []string) (int, error) {
	if len(ips) == 0 {
		return 0, nil
	}
	tx, err := conn.Begin()
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()

	added := 0
	srcJSON := `["dns"]`
	for _, ip := range ips {
		// 尝试插入，已存在则合并 source
		var existingID int64
		var existingSrcRaw string
		err := tx.QueryRow(
			"SELECT id, sources FROM assets WHERE project_id=? AND host=? AND IFNULL(port,'')=''",
			projectID, ip,
		).Scan(&existingID, &existingSrcRaw)

		if err == sql.ErrNoRows {
			// 不存在，插入新 IP 资产
			_, err := tx.Exec(
				"INSERT INTO assets(project_id, type, host, sources) VALUES(?,?,?,?)",
				projectID, "ip", ip, srcJSON,
			)
			if err == nil {
				added++
			}
		} else if err != nil {
			// 真正的数据库错误，跳过此条
			continue
		} else {
			// 已存在，合并 source
			var srcs []string
			_ = json.Unmarshal([]byte(existingSrcRaw), &srcs)
			if !contains(srcs, "dns") {
				srcs = append(srcs, "dns")
				if b, err := json.Marshal(srcs); err == nil {
					_, _ = tx.Exec("UPDATE assets SET sources=? WHERE id=?", string(b), existingID)
				}
			}
		}
	}

	// 给域名加标签 "DNS✓"
	addTagInTx(tx, projectID, domain, "DNS✓")

	return added, tx.Commit()
}

// TagDnsFailed 给 DNS 解析失败的域名加标签 "DNS无效"
func TagDnsFailed(projectID int64, domain string) error {
	tx, err := conn.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	addTagInTx(tx, projectID, domain, "DNS无效")
	return tx.Commit()
}

// addTagInTx 在事务内给指定 host 的所有资产加标签
func addTagInTx(tx interface {
	QueryRow(query string, args ...any) *sql.Row
	Exec(query string, args ...any) (sql.Result, error)
}, projectID int64, host, tag string) {
	var id int64
	var raw string
	err := tx.QueryRow(
		"SELECT id, IFNULL(tags,'[]') FROM assets WHERE project_id=? AND host=? AND IFNULL(port,'')='' LIMIT 1",
		projectID, host,
	).Scan(&id, &raw)
	if err != nil {
		return
	}
	var tags []string
	_ = json.Unmarshal([]byte(raw), &tags)

	// 如果是 "DNS✓"，先移除 "DNS无效"；反之亦然
	if tag == "DNS✓" {
		tags = removeFromSlice(tags, "DNS无效")
	} else if tag == "DNS无效" {
		tags = removeFromSlice(tags, "DNS✓")
	}

	if !contains(tags, tag) {
		tags = append(tags, tag)
		b, _ := json.Marshal(tags)
		tx.Exec("UPDATE assets SET tags=? WHERE id=?", string(b), id)
	}
}

// removeFromSlice 从切片中移除指定元素
func removeFromSlice(s []string, v string) []string {
	out := make([]string, 0, len(s))
	for _, x := range s {
		if x != v {
			out = append(out, x)
		}
	}
	return out
}

// ── helpers ──────────────────────────────────────────────────────────────

// AssetPageResult 分页查询结果
type AssetPageResult struct {
	Items []model.Asset `json:"items"`
	Total int           `json:"total"`
}

// ListAssetsPage 分页列出资产
func ListAssetsPage(projectID int64, typeFilter, statusFilter, keyword, networkFilter, sortBy, sortOrder string, page, pageSize int) (AssetPageResult, error) {
	var result AssetPageResult
	if page < 1 {
		page = 1
	}
	if pageSize < 1 {
		pageSize = 50
	}
	offset := (page - 1) * pageSize

	// 构造 WHERE 子句
	where := "WHERE project_id=?"
	args := []any{projectID}
	if typeFilter != "" {
		where += " AND type=?"
		args = append(args, typeFilter)
	}
	if statusFilter != "" {
		if statusFilter == "unprobed" {
			where += " AND (status IS NULL OR status='')"
		} else {
			where += " AND status=?"
			args = append(args, statusFilter)
		}
	}
	if networkFilter == "intranet" {
		where += ` AND (host LIKE '10.%' OR host LIKE '192.168.%' OR host LIKE '127.%' OR host GLOB '172.1[6-9].*' OR host GLOB '172.2[0-9].*' OR host GLOB '172.3[0-1].*')`
	} else if networkFilter == "extranet" {
		where += ` AND NOT (host LIKE '10.%' OR host LIKE '192.168.%' OR host LIKE '127.%' OR host GLOB '172.1[6-9].*' OR host GLOB '172.2[0-9].*' OR host GLOB '172.3[0-1].*')`
	}
	if kw := strings.TrimSpace(keyword); kw != "" {
		where += " AND (host LIKE ? OR IFNULL(title,'') LIKE ? OR IFNULL(server,'') LIKE ? OR IFNULL(port,'') LIKE ?)"
		like := "%" + kw + "%"
		args = append(args, like, like, like, like)
	}

	// 查总数
	countQ := "SELECT COUNT(*) FROM assets " + where
	if err := conn.QueryRow(countQ, args...).Scan(&result.Total); err != nil {
		return result, fmt.Errorf("count: %w", err)
	}

	// 排序
	allowedCols := map[string]string{
		"host": "host", "port": "IFNULL(port,'')", "status": "IFNULL(status,'')",
		"status_code": "status_code", "title": "IFNULL(title,'')", "server": "IFNULL(server,'')",
		"probed_at": "IFNULL(probed_at,'')", "created_at": "created_at",
	}
	orderClause := "ORDER BY created_at DESC, id DESC"
	if col, ok := allowedCols[sortBy]; ok {
		dir := "ASC"
		if sortOrder == "desc" {
			dir = "DESC"
		}
		orderClause = fmt.Sprintf("ORDER BY %s %s, id DESC", col, dir)
	}

	// 查分页数据
	q := `SELECT id, project_id, type, host, IFNULL(port,''), sources, IFNULL(tags,'[]'),
	             IFNULL(status,''), status_code, IFNULL(title,''), IFNULL(server,''),
	             IFNULL(tech,''), IFNULL(probed_at,''), created_at
	      FROM assets ` + where + " " + orderClause + " LIMIT ? OFFSET ?"
	args = append(args, pageSize, offset)

	rows, err := conn.Query(q, args...)
	if err != nil {
		return result, err
	}
	defer rows.Close()

	for rows.Next() {
		var a model.Asset
		var srcRaw, tagsRaw string
		var statusCode sql.NullInt64
		err := rows.Scan(
			&a.ID, &a.ProjectID, &a.Type, &a.Host, &a.Port, &srcRaw, &tagsRaw,
			&a.Status, &statusCode, &a.Title, &a.Server,
			&a.Tech, &a.ProbedAt, &a.CreatedAt,
		)
		if err != nil {
			return result, err
		}
		if statusCode.Valid {
			c := int(statusCode.Int64)
			a.StatusCode = &c
		}
		_ = json.Unmarshal([]byte(srcRaw), &a.Sources)
		if a.Sources == nil {
			a.Sources = []string{}
		}
		_ = json.Unmarshal([]byte(tagsRaw), &a.Tags)
		if a.Tags == nil {
			a.Tags = []string{}
		}
		result.Items = append(result.Items, a)
	}
	if err := rows.Err(); err != nil {
		return result, err
	}
	if result.Items == nil {
		result.Items = []model.Asset{}
	}
	return result, nil
}

// CountAssetStats 统计项目资产各维度数量
func CountAssetStats(projectID int64) (map[string]int, error) {
	q := `SELECT
		COUNT(*)                                       AS total,
		SUM(CASE WHEN type='ip'     THEN 1 ELSE 0 END) AS ip_count,
		SUM(CASE WHEN type='domain' THEN 1 ELSE 0 END) AS domain_count,
		SUM(CASE WHEN status='alive' THEN 1 ELSE 0 END) AS alive_count,
		SUM(CASE WHEN status='dead'  THEN 1 ELSE 0 END) AS dead_count,
		SUM(CASE WHEN status IS NULL OR status='' THEN 1 ELSE 0 END) AS unprobed_count,
		COUNT(DISTINCT CASE WHEN port IS NOT NULL AND port!='' THEN host||':'||port END) AS port_count
	FROM assets WHERE project_id=?`
	var total, ip, domain, alive, dead, unprobed, ports int
	err := conn.QueryRow(q, projectID).Scan(&total, &ip, &domain, &alive, &dead, &unprobed, &ports)
	if err != nil {
		return nil, err
	}
	return map[string]int{
		"total":    total,
		"ip":       ip,
		"domain":   domain,
		"alive":    alive,
		"dead":     dead,
		"unprobed": unprobed,
		"ports":    ports,
	}, nil
}

func nullable(s string) any {
	if s == "" {
		return nil
	}
	return s
}

func contains(arr []string, s string) bool {
	for _, x := range arr {
		if x == s {
			return true
		}
	}
	return false
}
