// Package store 提供 SQLite 连接、迁移与事务管理。
package store

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"

	_ "modernc.org/sqlite" // 纯 Go 实现的 SQLite 驱动，无需 CGO
)

// DB 包装 *sql.DB，附带数据库文件路径。
type DB struct {
	*sql.DB
	Path string
}

// Open 打开（必要时创建）SQLite 数据库文件并执行迁移。
// 只使用真实文件，绝不使用 :memory:。
func Open(ctx context.Context, path string) (*DB, error) {
	if path == "" || path == ":memory:" {
		return nil, fmt.Errorf("数据库路径必须为真实文件路径，禁止 :memory:")
	}
	if dir := filepath.Dir(path); dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return nil, fmt.Errorf("创建数据库目录失败: %w", err)
		}
	}
	dsn := fmt.Sprintf("file:%s?_pragma=journal_mode(WAL)&_pragma=foreign_keys(1)&_pragma=busy_timeout(5000)&_pragma=synchronous(NORMAL)", path)
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("打开数据库失败: %w", err)
	}
	// SQLite 写操作串行化，限制连接数以避免 SQLITE_BUSY。
	db.SetMaxOpenConns(1)
	if err := db.PingContext(ctx); err != nil {
		db.Close()
		return nil, fmt.Errorf("数据库连接失败: %w", err)
	}
	wrapped := &DB{DB: db, Path: path}
	if err := Migrate(ctx, wrapped.DB); err != nil {
		db.Close()
		return nil, err
	}
	return wrapped, nil
}
