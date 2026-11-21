package rules

import (
	"fmt"
	"strings"
)

const (
	maxEmailLength    = 254
	minPasswordLength = 8
	maxPasswordLength = 128
)

// ValidationError reports an invalid user input. The message is safe to return
// to the client.
type ValidationError struct {
	Message string
}

func (e *ValidationError) Error() string {
	return e.Message
}

// NormalizeEmail trims the address and lowercases it, so that the database
// uniqueness check does not depend on the input case.
func NormalizeEmail(email string) string {
	return strings.ToLower(strings.TrimSpace(email))
}

// ValidateEmail checks the address shape. It is intentionally stricter than
// RFC 5322: the service accepts one @ with a non-empty local and domain part.
func ValidateEmail(email string) error {
	if email == "" {
		return &ValidationError{Message: "Укажите адрес электронной почты."}
	}
	if len(email) > maxEmailLength {
		return &ValidationError{Message: "Адрес электронной почты слишком длинный."}
	}
	local, domain, found := strings.Cut(email, "@")
	if !found || local == "" || domain == "" {
		return &ValidationError{Message: "Некорректный адрес электронной почты."}
	}
	if strings.ContainsAny(email, " \t\r\n") || !strings.Contains(domain, ".") {
		return &ValidationError{Message: "Некорректный адрес электронной почты."}
	}
	if strings.HasPrefix(domain, ".") || strings.HasSuffix(domain, ".") {
		return &ValidationError{Message: "Некорректный адрес электронной почты."}
	}
	return nil
}

// ValidatePassword checks the password length in characters.
func ValidatePassword(password string) error {
	length := len([]rune(password))
	switch {
	case length == 0:
		return &ValidationError{Message: "Укажите пароль."}
	case length < minPasswordLength:
		return &ValidationError{Message: fmt.Sprintf("Пароль должен содержать не менее %d символов.", minPasswordLength)}
	case length > maxPasswordLength:
		return &ValidationError{Message: fmt.Sprintf("Пароль должен содержать не более %d символов.", maxPasswordLength)}
	default:
		return nil
	}
}
