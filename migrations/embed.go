// 数据库迁移：使用 embed 将 SQL 文件编译进二进制，启动时自动执行。
package migrations

import "embed"

// Files 内嵌所有 SQL 迁移文件，供 goose 在启动时自动执行。
//
//go:embed *.sql
var Files embed.FS
