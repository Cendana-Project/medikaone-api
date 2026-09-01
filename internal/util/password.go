package util

import (
	"strings"
	"unicode"
)

func IsValidPassword(password string) bool {
	if len(password) < 8 {
		return false
	}

	var (
		hasUpper, hasLower, hasDigit, hasSpecial bool
	)

	for _, char := range password {
		switch {
		case unicode.IsUpper(char):
			hasUpper = true
		case unicode.IsLower(char):
			hasLower = true
		case unicode.IsDigit(char):
			hasDigit = true
		case unicode.IsPunct(char) || unicode.IsSymbol(char):
			hasSpecial = true
		}
	}

	// Periksa apakah password memenuhi kriteria kompleksitas
	isComplex := hasUpper && hasLower && hasDigit && hasSpecial

	// Periksa apakah password tidak memiliki angka berurutan atau karakter berulang
	hasNoSequential := !hasSequentialNumbers(password)
	hasNoRepeated := !hasRepeatedChars(password)

	return isComplex && hasNoSequential && hasNoRepeated
}

func hasSequentialNumbers(password string) bool {
	// Check for sequential numbers (e.g., 1234, 9876)
	for i := 0; i < len(password)-3; i++ {
		if unicode.IsDigit(rune(password[i])) &&
			unicode.IsDigit(rune(password[i+1])) &&
			unicode.IsDigit(rune(password[i+2])) &&
			unicode.IsDigit(rune(password[i+3])) {

			// Check if they're sequential
			if (password[i+1] == password[i]+1 &&
				password[i+2] == password[i]+2 &&
				password[i+3] == password[i]+3) ||
				(password[i+1] == password[i]-1 &&
					password[i+2] == password[i]-2 &&
					password[i+3] == password[i]-3) {
				return true
			}
		}
	}
	return false
}

func hasRepeatedChars(password string) bool {
	// Check for 4 or more repeated characters
	for i := 0; i < len(password)-3; i++ {
		if password[i] == password[i+1] &&
			password[i] == password[i+2] &&
			password[i] == password[i+3] {
			return true
		}
	}
	return false
}

func IsPasswordSimilarToUserInfo(password, username, email string) bool {
	password = strings.ToLower(password)
	username = strings.ToLower(strings.TrimSpace(username))
	email = strings.ToLower(strings.TrimSpace(email))

	if username != "" && strings.Contains(password, username) {
		return true
	}
	if email != "" && strings.Contains(password, email) {
		return true
	}

	localPart, _, found := strings.Cut(email, "@")
	if found && len([]rune(localPart)) >= 3 && strings.Contains(password, localPart) {
		return true
	}

	return false
}
