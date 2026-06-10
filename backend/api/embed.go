// Package api embeds the OpenAPI 3.0 specification for the Oasis API.
package api

import _ "embed"

//go:embed openapi.yaml
var Spec []byte
