package git

import (
	"codeburg.org/lexbit/relurpify/platform/contracts"
	platformgit "codeburg.org/lexbit/relurpify/platform/git"
)

func init() {
	registerGit := func(key, command string) {
		contracts.RegisterNative(key, func(basePath string) contracts.Tool {
			return &platformgit.GitCommandTool{
				RepoPath: basePath,
				Command:  command,
			}
		})
	}
	registerGit("git_diff", "diff")
	registerGit("git_history", "history")
	registerGit("git_branch", "branch")
	registerGit("git_commit", "commit")
	registerGit("git_blame", "blame")
}
