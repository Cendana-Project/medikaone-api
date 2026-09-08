package doctor_hospital

import (
	"testing"
	"time"
)

func TestHospitalWorkerDOBMeetsMinimumRequiresOlderThanFifteen(t *testing.T) {
	today := time.Date(2026, 9, 8, 0, 0, 0, 0, time.UTC)
	if hospitalWorkerDOBMeetsMinimum(today.AddDate(-15, 0, 0), today) {
		t.Fatal("a worker exactly fifteen years old must be rejected")
	}
	if !hospitalWorkerDOBMeetsMinimum(today.AddDate(-15, 0, -1), today) {
		t.Fatal("a worker older than fifteen years must be accepted")
	}
}

func TestEscapeLikePattern(t *testing.T) {
	got := escapeLikePattern(`dr_100%\sip`)
	want := `dr\_100\%\\sip`
	if got != want {
		t.Fatalf("escapeLikePattern() = %q, want %q", got, want)
	}
}

func TestNormalizePhoneIdentity(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{input: "+62 812-3456-7890", want: "6281234567890"},
		{input: "(021) 555.1234", want: "0215551234"},
		{input: "123456", want: ""},
		{input: "SIP-3174-2026-001", want: ""},
	}
	for _, test := range tests {
		if got := normalizePhoneIdentity(test.input); got != test.want {
			t.Errorf("normalizePhoneIdentity(%q) = %q, want %q", test.input, got, test.want)
		}
	}
}
