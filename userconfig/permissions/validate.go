package permissions

import (
	"errors"
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
)

// Validate ensures the permission set is well-formed. It is the method form of
// ValidatePermissionSet so callers holding a *PermissionSet (e.g. the
// permission manager and tool permission manifests) can validate directly.
func (p *PermissionSet) Validate() error {
	return ValidatePermissionSet(p)
}

// ValidatePermissionSet ensures the permission declaration is consistent.
func ValidatePermissionSet(p *PermissionSet) error {
	if p == nil {
		return nil
	}
	for _, perm := range p.FileSystem {
		if perm.Path == "" {
			return fmt.Errorf("filesystem permission %s missing path", perm.Action)
		}
		if err := validateGlobPath(perm.Path); err != nil {
			return fmt.Errorf("invalid filesystem path %s: %w", perm.Path, err)
		}
	}
	for _, exec := range p.Executables {
		if exec.Binary == "" {
			return errors.New("executable permission missing binary")
		}
		if strings.Contains(exec.Binary, "/") {
			return fmt.Errorf("executable %s must be referenced by name", exec.Binary)
		}
	}
	for _, net := range p.Network {
		if net.Direction == "" {
			return errors.New("network permission missing direction")
		}
		if net.Protocol == "" {
			return fmt.Errorf("network permission for %s missing protocol", net.Direction)
		}
		if net.Direction == "egress" && net.Host == "" {
			return errors.New("egress network permission must declare host")
		}
	}
	return nil
}

func validateGlobPath(path string) error {
	if path == "" {
		return errors.New("glob cannot be empty")
	}
	if strings.Contains(path, "..") {
		return errors.New("glob cannot contain '..'")
	}
	clean := filepath.ToSlash(filepath.Clean(path))
	if strings.HasPrefix(clean, "../") || clean == ".." {
		return errors.New("glob cannot escape workspace")
	}
	re := regexp.MustCompile(`^[\w./*\-{}${}]+$`)
	if !re.MatchString(path) {
		return errors.New("glob contains unsupported characters")
	}
	return nil
}
