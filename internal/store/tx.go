package store

import (
	"context"
	"database/sql"
	"fmt"
)

// TxManager 管理真实 SQLite 事务；任何一步失败都会整体回滚。
type TxManager struct {
	db *DB
}

// NewTxManager 创建事务管理器。
func NewTxManager(db *DB) *TxManager { return &TxManager{db: db} }

// DB 返回底层数据库。
func (m *TxManager) DB() *DB { return m.db }

// Queryer 抽象 *sql.DB 与 *sql.Tx 的共同接口。
type Queryer interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

// InTx 在事务中执行 fn。fn 返回错误即回滚，否则提交。
// 提交失败同样回滚并返回错误，保证多步写入的原子性。
func (m *TxManager) InTx(ctx context.Context, fn func(tx *sql.Tx) error) (err error) {
	tx, err := m.db.BeginTx(ctx, &sql.TxOptions{})
	if err != nil {
		return fmt.Errorf("开启事务失败: %w", err)
	}
	defer func() {
		if p := recover(); p != nil {
			_ = tx.Rollback()
			panic(p)
		}
		if err != nil {
			_ = tx.Rollback()
		}
	}()
	if err = fn(tx); err != nil {
		return err
	}
	if err = tx.Commit(); err != nil {
		return fmt.Errorf("提交事务失败: %w", err)
	}
	return nil
}
