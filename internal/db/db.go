// Package db 封装 SQLite 的初始化和建表操作。
// 这里只做最基础的数据库连接准备，具体的增删改查由各业务文件自己写 SQL。
package db

import (
	"database/sql"
	"os"
	"path/filepath"

	_ "github.com/mattn/go-sqlite3" // 匿名导入：注册 SQLite 驱动
)

// Init 连接 SQLite，并确保 data 目录和 videos 表存在。
// 返回一个已打开的数据库连接，整个应用共用这一个连接即可。
func Init(dbPath string) (*sql.DB, error) {
	// 1. 确保数据库所在目录存在（第一次运行 data 目录还没建）
	if err := os.MkdirAll(filepath.Dir(dbPath), 0o755); err != nil {
		return nil, err
	}

	// 2. 打开 SQLite；文件不存在会自动创建
	conn, err := sql.Open("sqlite3", dbPath+"?_busy_timeout=5000&_journal_mode=WAL")
	if err != nil {
		return nil, err
	}

	// 3. 建表（IF NOT EXISTS 表示重复执行也不会报错）
	err = createTables(conn)
	if err != nil {
		conn.Close()
		return nil, err
	}
	return conn, nil
}

// createTables 执行建表 SQL。
// 目前只有一张 videos 表，后续加功能可以在这里继续追加。
func createTables(conn *sql.DB) error {
	sqlStmt := `
	CREATE TABLE IF NOT EXISTS videos (
		id          INTEGER PRIMARY KEY AUTOINCREMENT,
		folder_path TEXT    NOT NULL DEFAULT '/',
		file_name   TEXT    NOT NULL,
		file_path   TEXT    NOT NULL UNIQUE,
		file_size   INTEGER NOT NULL DEFAULT 0,
		duration    INTEGER NOT NULL DEFAULT 0,
		resolution  TEXT    NOT NULL DEFAULT '',
		thumbnail   TEXT    NOT NULL DEFAULT '',
		created_at  DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
		updated_at  DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
	);
	CREATE INDEX IF NOT EXISTS idx_videos_folder  ON videos(folder_path);
	CREATE INDEX IF NOT EXISTS idx_videos_name    ON videos(file_name);
	CREATE INDEX IF NOT EXISTS idx_videos_created ON videos(created_at);
	`
	_, err := conn.Exec(sqlStmt)
	return err
}
