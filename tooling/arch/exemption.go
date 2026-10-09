package arch

import "codeburg.org/lexbit/relurpify/tooling/arch/gatescan"

// Exemption names the sites an AST-based gate deliberately accepts.
//
// The grep gates these checks replace hid their exceptions in grep -v filters:
// basename filters that silently exempted every file so named anywhere in the
// tree, with no record of why the exception existed. An Exemption states each
// accepted site explicitly, so a newly added file that happens to share a name
// is still policed and every exception is readable at the gate that makes it.
//
// Prefer no exemption. A gate that accepts nothing cannot be argued with.
type Exemption struct {
	// Prefixes are slash-separated path prefixes matched at component
	// boundaries, relative to the walk root: "tooling/arch" exempts that tree
	// but not "tooling/archx". "userconfig" matches "userconfig/config/a.go".
	Prefixes []string
	// Files are exact paths, relative to the walk root and slash-separated.
	// Never a basename: the point is to exempt one known site, not a name.
	Files []string
}

// Covers reports whether path sits under an exempted prefix or is an exempted
// file.
func (e Exemption) Covers(path string) bool {
	if gatescan.HasPathPrefix(path, e.Prefixes...) {
		return true
	}
	for _, f := range e.Files {
		if path == f {
			return true
		}
	}
	return false
}
