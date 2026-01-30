// Package schemas provides embedded JSON schema files for validation.
package schemas

import "embed"

// FS contains all JSON schema files embedded at compile time.
// Access schemas via FS.ReadFile("run-config/v1.json"), etc.
//
//go:embed */v1.json
var FS embed.FS
