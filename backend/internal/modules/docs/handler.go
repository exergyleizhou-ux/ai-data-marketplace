// Package docs serves the embedded OpenAPI 3.0 specification and a minimal
// Swagger UI page at /docs.
package docs

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/lei/ai-data-marketplace/backend/api"
)

// Register mounts documentation routes on the top-level engine (not under
// /api/v1) so /docs is a first-class path.  No auth — the spec is public.
func Register(engine *gin.Engine) {
	h := &handler{}
	engine.GET("/docs/openapi.yaml", h.spec)
	engine.GET("/docs", h.ui)
}

type handler struct{}

func (h *handler) spec(c *gin.Context) {
	c.Data(http.StatusOK, "application/yaml", api.Spec)
}

func (h *handler) ui(c *gin.Context) {
	c.Data(http.StatusOK, "text/html; charset=utf-8", swaggerUI)
}

// swaggerUI is a single self-contained HTML page.  It loads Swagger UI from
// cdn.jsdelivr.net and points it at /docs/openapi.yaml.
var swaggerUI = []byte(`<!DOCTYPE html>
<html lang="en">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <title>Oasis API — Swagger UI</title>
  <link rel="stylesheet" href="https://cdn.jsdelivr.net/npm/swagger-ui-dist@5/swagger-ui.css">
</head>
<body>
  <div id="swagger-ui"></div>
  <script src="https://cdn.jsdelivr.net/npm/swagger-ui-dist@5/swagger-ui-bundle.js" crossorigin></script>
  <script>
    SwaggerUIBundle({
      url: "/docs/openapi.yaml",
      dom_id: "#swagger-ui",
      deepLinking: true,
      presets: [SwaggerUIBundle.presets.apis, SwaggerUIBundle.SwaggerUIStandalonePreset],
      layout: "BaseLayout",
    });
  </script>
</body>
</html>
`)
