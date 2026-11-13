// Package configconvert provides explicit configuration conversion utilities.
// This package implements the one-shot conversion tool required by P05/D-19.
//
// The conversion tool:
//   - Validates old-format configuration inputs
//   - Creates secure backups of the original files
//   - Writes current schema configuration
//   - Produces a deterministic report
//   - Refuses mixed/unknown versions
//
// No legacy decoder branches remain reachable at runtime after conversion.
package configconvert
