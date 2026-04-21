package authorization

import (
	"context"
	"testing"

	"codeburg.org/lexbit/relurpify/framework/core"
)

func BenchmarkCheckFileAccess(b *testing.B) {
	manager, err := NewPermissionManager("/ws", &core.PermissionSet{
		FileSystem: []core.FileSystemPermission{
			{Action: core.FileSystemRead, Path: "/ws/src/**"},
		},
	}, nil, nil)
	if err != nil {
		b.Fatal(err)
	}
	ctx := context.Background()

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if err := manager.CheckFileAccess(ctx, "agent", core.FileSystemRead, "/ws/src/main.go"); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkCheckExecutable(b *testing.B) {
	manager, err := NewPermissionManager("/ws", &core.PermissionSet{
		Executables: []core.ExecutablePermission{
			{Binary: "git"},
		},
	}, nil, nil)
	if err != nil {
		b.Fatal(err)
	}
	ctx := context.Background()

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if err := manager.CheckExecutable(ctx, "agent", "git", []string{"status"}, nil); err != nil {
			b.Fatal(err)
		}
	}
}
