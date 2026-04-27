// sqlc 自动生成的数据库查询层基础结构，提供 DBTX 接口和事务支持。
package sqlc

import (
	"context"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// DBTX 抽象 pgx 连接和事务的公共接口。
type DBTX interface {
	Exec(context.Context, string, ...any) (pgconn.CommandTag, error)
	Query(context.Context, string, ...any) (pgx.Rows, error)
	QueryRow(context.Context, string, ...any) pgx.Row
}

// Queries 持有 DBTX 执行 SQL。
type Queries struct {
	db DBTX
}

// New 创建 Queries 实例。
func New(db DBTX) *Queries {
	return &Queries{db: db}
}

// WithTx 返回使用给定事务的 Queries 副本。
func (q *Queries) WithTx(tx pgx.Tx) *Queries {
	return &Queries{db: tx}
}
