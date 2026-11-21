package handlers

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
)

const (
	refreshCookieName = "refresh_token"
	refreshCookiePath = "/api/v1/auth"
)

// setRefreshCookie keeps the refresh token out of JavaScript reach: it is an
// HttpOnly cookie scoped to the authentication endpoints only.
func setRefreshCookie(c *gin.Context, value string, expiresAt time.Time, secure bool) {
	maxAge := int(time.Until(expiresAt).Seconds())
	if maxAge < 0 {
		maxAge = 0
	}

	// Secure follows the deployment: local HTTP needs it disabled, HttpOnly
	// and SameSite are always set.
	//nolint:gosec // G124: HttpOnly and SameSite are set; Secure is configured.
	http.SetCookie(c.Writer, &http.Cookie{
		Name:     refreshCookieName,
		Value:    value,
		Path:     refreshCookiePath,
		MaxAge:   maxAge,
		Expires:  expiresAt,
		HttpOnly: true,
		Secure:   secure,
		SameSite: http.SameSiteStrictMode,
	})
}

// clearRefreshCookie removes the cookie, including the Secure flag matching the
// one used when the cookie was set.
func clearRefreshCookie(c *gin.Context, secure bool) {
	//nolint:gosec // G124: HttpOnly and SameSite are set; Secure is configured.
	http.SetCookie(c.Writer, &http.Cookie{
		Name:     refreshCookieName,
		Value:    "",
		Path:     refreshCookiePath,
		MaxAge:   -1,
		Expires:  time.Unix(1, 0),
		HttpOnly: true,
		Secure:   secure,
		SameSite: http.SameSiteStrictMode,
	})
}

func refreshTokenFromCookie(c *gin.Context) (string, bool) {
	value, err := c.Cookie(refreshCookieName)
	if err != nil || value == "" {
		return "", false
	}
	return value, true
}
