package sqlite

import (
	"codeburg.org/lexbit/relurpify/capability/ports"
	platformsqlite "codeburg.org/lexbit/relurpify/platform/db/sqlite"
)

func Constructors() map[string]ports.NativeToolConstructor {
	return map[string]ports.NativeToolConstructor{
		"sqlite_database_detect":  func(basePath string) ports.Tool { return &platformsqlite.SQLiteDatabaseDetectTool{BasePath: basePath} },
		"sqlite_schema_inspect":   func(basePath string) ports.Tool { return platformsqlite.NewSQLiteSchemaInspectTool(basePath) },
		"sqlite_query":            func(basePath string) ports.Tool { return platformsqlite.NewSQLiteQueryTool(basePath) },
		"sqlite_integrity_check":  func(basePath string) ports.Tool { return platformsqlite.NewSQLiteIntegrityCheckTool(basePath) },
	}
}

func init() {
	for k, v := range Constructors() {
		ports.RegisterNative(k, v)
	}
}
