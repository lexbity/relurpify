package sqlite

import (
	"codeburg.org/lexbit/relurpify/platform/contracts"
	platformsqlite "codeburg.org/lexbit/relurpify/platform/db/sqlite"
)

func init() {
	contracts.RegisterNative("sqlite_database_detect", func(basePath string) contracts.Tool {
		return &platformsqlite.SQLiteDatabaseDetectTool{BasePath: basePath}
	})
	contracts.RegisterNative("sqlite_schema_inspect", func(basePath string) contracts.Tool {
		return platformsqlite.NewSQLiteSchemaInspectTool(basePath)
	})
	contracts.RegisterNative("sqlite_query", func(basePath string) contracts.Tool {
		return platformsqlite.NewSQLiteQueryTool(basePath)
	})
	contracts.RegisterNative("sqlite_integrity_check", func(basePath string) contracts.Tool {
		return platformsqlite.NewSQLiteIntegrityCheckTool(basePath)
	})
}
