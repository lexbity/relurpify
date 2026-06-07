package sqlite

import (
	"codeburg.org/lexbit/relurpify/capability/ports"
	platformsqlite "codeburg.org/lexbit/relurpify/platform/db/sqlite"
)

func init() {
	ports.RegisterNative("sqlite_database_detect", func(basePath string) ports.Tool {
		return &platformsqlite.SQLiteDatabaseDetectTool{BasePath: basePath}
	})
	ports.RegisterNative("sqlite_schema_inspect", func(basePath string) ports.Tool {
		return platformsqlite.NewSQLiteSchemaInspectTool(basePath)
	})
	ports.RegisterNative("sqlite_query", func(basePath string) ports.Tool {
		return platformsqlite.NewSQLiteQueryTool(basePath)
	})
	ports.RegisterNative("sqlite_integrity_check", func(basePath string) ports.Tool {
		return platformsqlite.NewSQLiteIntegrityCheckTool(basePath)
	})
}
