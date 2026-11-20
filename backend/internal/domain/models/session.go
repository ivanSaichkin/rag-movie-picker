package models

import "time"

// Session is the result of a successful registration, login or refresh: the
// client stores the access token in memory and the refresh token in a cookie.
type Session struct {
	User             User
	AccessToken      string
	AccessExpiresIn  int
	RefreshToken     string
	RefreshExpiresAt time.Time
}
