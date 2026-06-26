// Package dashboards provides embedded dashboard configurations.
package dashboards

import _ "embed"

// DefaultProjectsJSON is the embedded default dashboard projects configuration.
// It contains an array of project definitions, each with its own dashboards.
//
//go:embed default_project.json
var DefaultProjectsJSON []byte
