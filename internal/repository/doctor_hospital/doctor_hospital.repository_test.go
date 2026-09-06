package doctor_hospital

import "testing"

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
