// Package openapi embeds the OpenAPI specification.
package openapi

import "embed"

//go:embed openapi.yaml
var FS embed.FS
