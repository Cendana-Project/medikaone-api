package util

import "testing"

func TestIsPasswordSimilarToUserInfoIgnoresEmptyUsername(t *testing.T) {
	if IsPasswordSimilarToUserInfo("Strong!Pass9", "", "user@example.com") {
		t.Fatal("an empty username must not match every password")
	}
}

func TestIsPasswordSimilarToUserInfoDetectsIdentityParts(t *testing.T) {
	tests := []struct {
		name     string
		password string
		username string
		email    string
	}{
		{name: "username", password: "A!hamdan9Safe", username: "hamdan", email: "owner@example.com"},
		{name: "email local part", password: "A!owner9Safe", username: "other", email: "owner@example.com"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if !IsPasswordSimilarToUserInfo(tt.password, tt.username, tt.email) {
				t.Fatal("expected password to be detected as similar")
			}
		})
	}
}
