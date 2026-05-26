package db

import (
	"database/sql"
	"fmt"

	_ "modernc.org/sqlite"
)

var conn *sql.DB

// Init 初始化数据库连接，建表
func Init(path string) error {
	var err error
	conn, err = sql.Open("sqlite", path+"?_pragma=foreign_keys(1)&_pragma=journal_mode(WAL)&_busy_timeout=5000")
	if err != nil {
		return fmt.Errorf("open db: %w", err)
	}
	if err := conn.Ping(); err != nil {
		return fmt.Errorf("ping db: %w", err)
	}
	return migrate()
}

// Close 关闭连接
func Close() error {
	if conn != nil {
		return conn.Close()
	}
	return nil
}

func migrate() error {
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS projects (
			id          INTEGER PRIMARY KEY AUTOINCREMENT,
			name        TEXT    NOT NULL UNIQUE,
			created_at  TEXT    NOT NULL DEFAULT (datetime('now','localtime'))
		)`,
		`CREATE TABLE IF NOT EXISTS assets (
			id          INTEGER PRIMARY KEY AUTOINCREMENT,
			project_id  INTEGER NOT NULL,
			type        TEXT    NOT NULL CHECK(type IN ('ip','domain')),
			host        TEXT    NOT NULL,
			port        TEXT,
			sources     TEXT    NOT NULL DEFAULT '[]',
			tags        TEXT    NOT NULL DEFAULT '[]',
			status      TEXT,
			status_code INTEGER,
			title       TEXT,
			server      TEXT,
			tech        TEXT,
			probed_at   TEXT,
			created_at  TEXT    NOT NULL DEFAULT (datetime('now','localtime')),
			UNIQUE(project_id, host, port),
			FOREIGN KEY(project_id) REFERENCES projects(id) ON DELETE CASCADE
		)`,
		`CREATE TABLE IF NOT EXISTS settings (
			key   TEXT PRIMARY KEY,
			value TEXT NOT NULL
		)`,
		`CREATE INDEX IF NOT EXISTS idx_assets_project ON assets(project_id)`,
		`CREATE INDEX IF NOT EXISTS idx_assets_status  ON assets(status)`,
		`CREATE INDEX IF NOT EXISTS idx_assets_proj_type_status ON assets(project_id, type, status)`,
	}
	for _, s := range stmts {
		if _, err := conn.Exec(s); err != nil {
			return fmt.Errorf("migrate: %w (sql=%s)", err, s)
		}
	}
	// 兼容旧库：如果 tags 列不存在则添加
	conn.Exec("ALTER TABLE assets ADD COLUMN tags TEXT NOT NULL DEFAULT '[]'")
	// 兼容旧库：如果 resolved_ips 列不存在则添加
	conn.Exec("ALTER TABLE assets ADD COLUMN resolved_ips TEXT DEFAULT '[]'")
	return nil
}
