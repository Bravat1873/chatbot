// 数据库层：pgx 连接池创建与 goose 数据库迁移。
package db

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"chatbot/migrations"

	"github.com/jackc/pgx/v5/pgxpool"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"
)

// NewPool 创建 pgx 连接池，设置合理的连接数上下限和空闲超时。
func NewPool(ctx context.Context, dsn string) (*pgxpool.Pool, error) {
	config, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		return nil, fmt.Errorf("parse postgres config: %w", err)
	}
	config.MaxConns = 10
	config.MinConns = 1
	config.MaxConnIdleTime = 5 * time.Minute

	pool, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		return nil, fmt.Errorf("create postgres pool: %w", err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("ping postgres: %w", err)
	}
	return pool, nil
}

// RunMigrations 使用 goose 执行嵌入的 SQL 迁移文件，自动升级到最新版本。
func RunMigrations(ctx context.Context, dsn string) error {
	goose.SetBaseFS(migrations.Files) // 使用 embed.FS 内嵌的迁移文件
	if err := goose.SetDialect("postgres"); err != nil {
		return fmt.Errorf("set goose dialect: %w", err)
	}

	sqldb, err := sql.Open("pgx", dsn)
	if err != nil {
		return fmt.Errorf("open sql db for migrations: %w", err)
	}
	defer sqldb.Close()

	if err := sqldb.PingContext(ctx); err != nil {
		return fmt.Errorf("ping sql db for migrations: %w", err)
	}
	if err := goose.UpContext(ctx, sqldb, "."); err != nil {
		return fmt.Errorf("run migrations: %w", err)
	}
	return nil
}
