package git

import (
	"codeburg.org/lexbit/relurpify/capability/ports"
	platformgit "codeburg.org/lexbit/relurpify/platform/git"
)

func Constructors() map[string]ports.NativeToolConstructor {
	return map[string]ports.NativeToolConstructor{
		"git_diff":    func(basePath string) ports.Tool { return &platformgit.GitCommandTool{RepoPath: basePath, Command: "diff"} },
		"git_history": func(basePath string) ports.Tool { return &platformgit.GitCommandTool{RepoPath: basePath, Command: "history"} },
		"git_branch":  func(basePath string) ports.Tool { return &platformgit.GitCommandTool{RepoPath: basePath, Command: "branch"} },
		"git_commit":  func(basePath string) ports.Tool { return &platformgit.GitCommandTool{RepoPath: basePath, Command: "commit"} },
		"git_blame":   func(basePath string) ports.Tool { return &platformgit.GitCommandTool{RepoPath: basePath, Command: "blame"} },
	}
}

func init() {
	for k, v := range Constructors() {
		ports.RegisterNative(k, v)
	}
}
