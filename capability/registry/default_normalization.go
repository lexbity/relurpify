package registry

const (
	ExecRunBuild_default_normalization       = "exec_run_build"
	ExecRunCode_default_normalization        = "exec_run_code"
	ExecRunLinter_default_normalization      = "exec_run_linter"
	ExecRunTests_default_normalization       = "exec_run_tests"
	FileCreate_default_normalization         = "file_create"
	FileDelete_default_normalization         = "file_delete"
	FileList_default_normalization           = "file_list"
	FileRead_default_normalization           = "file_read"
	FileSearch_default_normalization         = "file_search"
	FileWrite_default_normalization          = "file_write"
	GitBlame_default_normalization           = "git_blame"
	GitBranch_default_normalization          = "git_branch"
	GitCommit_default_normalization          = "git_commit"
	GitDiff_default_normalization            = "git_diff"
	GitHistory_default_normalization         = "git_history"
	LspDocumentSymbols_default_normalization = "lsp_document_symbols"
	LspFormat_default_normalization          = "lsp_format"
	LspGetDefinition_default_normalization   = "lsp_get_definition"
	LspGetDiagnostics_default_normalization  = "lsp_get_diagnostics"
	LspGetHover_default_normalization        = "lsp_get_hover"
	LspGetReferences_default_normalization   = "lsp_get_references"
	LspSearchSymbols_default_normalization   = "lsp_search_symbols"
	QueryAst_default_normalization           = "query_ast"
	SearchFindSimilar_default_normalization  = "search_find_similar"
	SearchGrep_default_normalization         = "search_grep"
	SearchSemantic_default_normalization     = "search_semantic"
)

// DefaultToolNameNormalization maps common LLM tool name variations to their canonical
// capability names in the Relurpify platform.
var DefaultToolNameNormalization = map[string]string{
	// File Primitives (Contracts / FS)
	"read_file":                  FileRead_default_normalization,
	"view_file":                  FileRead_default_normalization,
	"cat":                        FileRead_default_normalization,
	"read":                       FileRead_default_normalization,
	"file_content":               FileRead_default_normalization,
	"get_file":                   FileRead_default_normalization,
	"read_file_content":          FileRead_default_normalization,
	"view_file_content":          FileRead_default_normalization,
	"readfile":                   FileRead_default_normalization,
	"show_file":                  FileRead_default_normalization,
	"display_file":               FileRead_default_normalization,
	"load_file":                  FileRead_default_normalization,
	"write_file":                 FileWrite_default_normalization,
	"edit_file":                  FileWrite_default_normalization,
	"file_edit":                  "file_edit",
	"save_file":                  FileWrite_default_normalization,
	"update_file":                FileWrite_default_normalization,
	"str_replace_editor":         FileWrite_default_normalization, // Anthropic-style file edit block
	"write_to_file":              FileWrite_default_normalization,
	"write":                      FileWrite_default_normalization,
	"patch_file":                 FileWrite_default_normalization,
	"file_patch":                 FileWrite_default_normalization,
	"edit":                       FileWrite_default_normalization,
	"modify_file":                FileWrite_default_normalization,
	"replace_file_content":       FileWrite_default_normalization,
	"multi_replace_file_content": FileWrite_default_normalization,
	"writefile":                  FileWrite_default_normalization,
	"update":                     FileWrite_default_normalization,
	"save":                       FileWrite_default_normalization,
	"overwrite_file":             FileWrite_default_normalization,
	"put_file":                   FileWrite_default_normalization,
	"change_file":                FileWrite_default_normalization,
	"list_dir":                   FileList_default_normalization,
	"list_directory":             FileList_default_normalization,
	"ls":                         FileList_default_normalization,
	"dir":                        FileList_default_normalization,
	"list":                       FileList_default_normalization,
	"list_files":                 FileList_default_normalization,
	"show_files":                 FileList_default_normalization,
	"listfiles":                  FileList_default_normalization,
	"view_directory":             FileList_default_normalization,
	"read_directory":             FileList_default_normalization,
	"scan_dir":                   FileList_default_normalization,
	"search_file":                FileSearch_default_normalization,
	"grep":                       FileSearch_default_normalization,
	"ripgrep":                    FileSearch_default_normalization,
	"rg":                         FileSearch_default_normalization,
	"search_files":               FileSearch_default_normalization,
	"find_files":                 FileSearch_default_normalization,
	"find":                       FileSearch_default_normalization,
	"file_grep":                  FileSearch_default_normalization,
	"search":                     FileSearch_default_normalization,
	"locate_files":               FileSearch_default_normalization,
	"find_in_files":              FileSearch_default_normalization,
	"create_file":                FileCreate_default_normalization,
	"touch":                      FileCreate_default_normalization,
	"new_file":                   FileCreate_default_normalization,
	"create":                     FileCreate_default_normalization,
	"add_file":                   FileCreate_default_normalization,
	"createfile":                 FileCreate_default_normalization,
	"make_file":                  FileCreate_default_normalization,
	"mkfile":                     FileCreate_default_normalization,
	"newfile":                    FileCreate_default_normalization,
	"delete_file":                FileDelete_default_normalization,
	"remove_file":                FileDelete_default_normalization,
	"rm":                         FileDelete_default_normalization,
	"delete":                     FileDelete_default_normalization,
	"remove":                     FileDelete_default_normalization,
	"deletefile":                 FileDelete_default_normalization,
	"rmfile":                     FileDelete_default_normalization,
	"unlink":                     FileDelete_default_normalization,

	// Git Tools
	"git_show_changes":  GitDiff_default_normalization,
	"git_changes":       GitDiff_default_normalization,
	"show_diff":         GitDiff_default_normalization,
	"diff":              GitDiff_default_normalization,
	"gitdiff":           GitDiff_default_normalization,
	"git_log":           GitHistory_default_normalization,
	"git_show_history":  GitHistory_default_normalization,
	"log":               GitHistory_default_normalization,
	"git_logs":          GitHistory_default_normalization,
	"history":           GitHistory_default_normalization,
	"githistory":        GitHistory_default_normalization,
	"gitlog":            GitHistory_default_normalization,
	"git_create_branch": GitBranch_default_normalization,
	"create_branch":     GitBranch_default_normalization,
	"branch":            GitBranch_default_normalization,
	"gitbranch":         GitBranch_default_normalization,
	"git_make_commit":   GitCommit_default_normalization,
	"commit":            GitCommit_default_normalization,
	"gitcommit":         GitCommit_default_normalization,
	"git_show_blame":    GitBlame_default_normalization,
	"blame":             GitBlame_default_normalization,
	"gitblame":          GitBlame_default_normalization,

	// Language Server Protocol (LSP)
	"get_definition":       LspGetDefinition_default_normalization,
	"find_definition":      LspGetDefinition_default_normalization,
	"go_to_definition":     LspGetDefinition_default_normalization,
	"definition":           LspGetDefinition_default_normalization,
	"goto_definition":      LspGetDefinition_default_normalization,
	"lsp_definition":       LspGetDefinition_default_normalization,
	"getdefinition":        LspGetDefinition_default_normalization,
	"find_def":             LspGetDefinition_default_normalization,
	"get_def":              LspGetDefinition_default_normalization,
	"go_to_def":            LspGetDefinition_default_normalization,
	"get_references":       LspGetReferences_default_normalization,
	"find_references":      LspGetReferences_default_normalization,
	"references":           LspGetReferences_default_normalization,
	"lsp_references":       LspGetReferences_default_normalization,
	"getreferences":        LspGetReferences_default_normalization,
	"find_refs":            LspGetReferences_default_normalization,
	"get_refs":             LspGetReferences_default_normalization,
	"get_hover":            LspGetHover_default_normalization,
	"hover":                LspGetHover_default_normalization,
	"lsp_hover":            LspGetHover_default_normalization,
	"hover_info":           LspGetHover_default_normalization,
	"get_hover_info":       LspGetHover_default_normalization,
	"hoverinfo":            LspGetHover_default_normalization,
	"get_diagnostics":      LspGetDiagnostics_default_normalization,
	"diagnostics":          LspGetDiagnostics_default_normalization,
	"lsp_diagnostics":      LspGetDiagnostics_default_normalization,
	"get_errors":           LspGetDiagnostics_default_normalization,
	"errors":               LspGetDiagnostics_default_normalization,
	"diagnose":             LspGetDiagnostics_default_normalization,
	"getdiagnostics":       LspGetDiagnostics_default_normalization,
	"search_symbols":       LspSearchSymbols_default_normalization,
	"find_symbols":         LspSearchSymbols_default_normalization,
	"symbols":              LspSearchSymbols_default_normalization,
	"workspace_symbols":    LspSearchSymbols_default_normalization,
	"searchsymbols":        LspSearchSymbols_default_normalization,
	"get_document_symbols": LspDocumentSymbols_default_normalization,
	"document_symbols":     LspDocumentSymbols_default_normalization,
	"file_symbols":         LspDocumentSymbols_default_normalization,
	"documentsymbols":      LspDocumentSymbols_default_normalization,
	"format":               LspFormat_default_normalization,
	"format_file":          LspFormat_default_normalization,
	"format_source":        LspFormat_default_normalization,
	"code_format":          LspFormat_default_normalization,
	"formatcode":           LspFormat_default_normalization,

	// AST / Structural Analysis
	"ast_analyze":     QueryAst_default_normalization,
	"analyze_ast":     QueryAst_default_normalization,
	"ast_query":       QueryAst_default_normalization,
	"query_structure": QueryAst_default_normalization,
	"ast":             QueryAst_default_normalization,
	"astanalyze":      QueryAst_default_normalization,
	"queryast":        QueryAst_default_normalization,
	"get_ast":         QueryAst_default_normalization,
	"parse_ast":       QueryAst_default_normalization,

	// Execution & Verification Tasks
	"run_tests":       ExecRunTests_default_normalization,
	"test":            ExecRunTests_default_normalization,
	"pytest":          ExecRunTests_default_normalization,
	"exec_test":       ExecRunTests_default_normalization,
	"run_unit_tests":  ExecRunTests_default_normalization,
	"execute_tests":   ExecRunTests_default_normalization,
	"runtests":        ExecRunTests_default_normalization,
	"go_test":         ExecRunTests_default_normalization,
	"npm_test":        ExecRunTests_default_normalization,
	"cargo_test":      ExecRunTests_default_normalization,
	"rust_cargo_test": ExecRunTests_default_normalization,
	"python_pytest":   ExecRunTests_default_normalization,
	"run_code":        ExecRunCode_default_normalization,
	"execute_code":    ExecRunCode_default_normalization,
	"python":          ExecRunCode_default_normalization,
	"exec_code":       ExecRunCode_default_normalization,
	"run_command":     ExecRunCode_default_normalization,
	"execute_command": ExecRunCode_default_normalization,
	"cmd":             ExecRunCode_default_normalization,
	"runcode":         ExecRunCode_default_normalization,
	"eval":            ExecRunCode_default_normalization,
	"exec_command":    ExecRunCode_default_normalization,
	"run_linter":      ExecRunLinter_default_normalization,
	"lint":            ExecRunLinter_default_normalization,
	"execute_linter":  ExecRunLinter_default_normalization,
	"run_lint":        ExecRunLinter_default_normalization,
	"runlinter":       ExecRunLinter_default_normalization,
	"code_lint":       ExecRunLinter_default_normalization,
	"run_build":       ExecRunBuild_default_normalization,
	"build":           ExecRunBuild_default_normalization,
	"compile":         ExecRunBuild_default_normalization,
	"execute_build":   ExecRunBuild_default_normalization,
	"runbuild":        ExecRunBuild_default_normalization,
	"make":            ExecRunBuild_default_normalization,
	"go_build":        ExecRunBuild_default_normalization,

	// Search & Semantic Queries
	"grep_search":       SearchGrep_default_normalization,
	"ripgrep_search":    SearchGrep_default_normalization,
	"regex_search":      SearchGrep_default_normalization,
	"text_search":       SearchGrep_default_normalization,
	"grepsearch":        SearchGrep_default_normalization,
	"find_similar":      SearchFindSimilar_default_normalization,
	"similarity_search": SearchFindSimilar_default_normalization,
	"find_similar_code": SearchFindSimilar_default_normalization,
	"similar_code":      SearchFindSimilar_default_normalization,
	"findsimilar":       SearchFindSimilar_default_normalization,
	"semantic_search":   SearchSemantic_default_normalization,
	"vector_search":     SearchSemantic_default_normalization,
	"concept_search":    SearchSemantic_default_normalization,
	"semanticsearch":    SearchSemantic_default_normalization,
}

func DefaultToolName(name string) (string, bool) {
	canonical, ok := DefaultToolNameNormalization[name]
	return canonical, ok
}
