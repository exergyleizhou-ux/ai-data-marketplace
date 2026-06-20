package verify

import (
	"fmt"
	"html"
	"net/http"

	"github.com/gin-gonic/gin"
)

// Badge colors. The label is the dark "Oasis C2D" plate; the message is green
// when the cert is registered, grey otherwise.
const (
	badgeLabelBG = "#2f3b36" // dark ink
	badgeGreen   = "#3f7a5c" // forest — verified
	badgeGrey    = "#9ca3af" // neutral — unverified / unknown
)

// badgeTextWidth approximates the pixel width of a short ASCII string at the
// badge font size (≈7px/char + horizontal padding).
func badgeTextWidth(s string) int { return len(s)*7 + 14 }

// badgeSVG renders a shields-style flat badge: a dark label plate on the left, a
// colored message on the right. Both strings are XML-escaped. The handler only
// ever passes fixed strings (NEVER the caller-controlled cert_id), so the badge
// is safe to open directly in a browser (an SVG document executes its scripts).
func badgeSVG(label, message, msgColor string) string {
	lw := badgeTextWidth(label)
	mw := badgeTextWidth(message)
	total := lw + mw
	el := html.EscapeString(label)
	em := html.EscapeString(message)
	return fmt.Sprintf(
		`<svg xmlns="http://www.w3.org/2000/svg" width="%d" height="20" role="img" aria-label="%s: %s">`+
			`<title>%s: %s</title>`+
			`<clipPath id="r"><rect width="%d" height="20" rx="3"/></clipPath>`+
			`<g clip-path="url(#r)">`+
			`<rect width="%d" height="20" fill="%s"/>`+
			`<rect x="%d" width="%d" height="20" fill="%s"/>`+
			`</g>`+
			`<g fill="#fff" font-family="Verdana,Geneva,DejaVu Sans,sans-serif" font-size="11" text-anchor="middle">`+
			`<text x="%d" y="14">%s</text>`+
			`<text x="%d" y="14">%s</text>`+
			`</g></svg>`,
		total, el, em,
		el, em,
		total,
		lw, badgeLabelBG,
		lw, mw, msgColor,
		lw/2, el,
		lw+mw/2, em,
	)
}

// badge serves an embeddable SVG status badge for a certificate. It always
// returns 200 with an SVG (so an embedded <img> degrades gracefully): a green
// "verified" badge when the cert is registered, a grey "unverified" badge
// otherwise. The cert_id is never reflected into the SVG (XSS-safe).
func (h *handler) badge(c *gin.Context) {
	msg, color := "verified", badgeGreen
	if _, err := h.repo.FindByCertID(c.Request.Context(), c.Param("cert_id")); err != nil {
		msg, color = "unverified", badgeGrey
	}
	c.Header("Cache-Control", "public, max-age=300")
	c.Data(http.StatusOK, "image/svg+xml; charset=utf-8", []byte(badgeSVG("Oasis C2D", msg, color)))
}
