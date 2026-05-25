package model

// This file keeps the validation contract entry points separate from the file
// loaders so callers can validate in-memory data without touching disk.

// ValidateProvider is a convenience wrapper around Provider.Validate.
func ValidateProvider(p Provider) error { return p.Validate() }

// ValidateProfile is a convenience wrapper around Profile.Validate.
func ValidateProfile(p Profile) error { return p.Validate() }

// DecodeWithSchema is injected by cfgload at init to break import cycles.
var DecodeWithSchema func(path string, data []byte, out any) (any, error)

