package response

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestPublicHospitalFacilitiesRemainJSON(t *testing.T) {
	data, err := json.Marshal(Hospital{ID: "hospital", Name: "Example", Facilities: json.RawMessage(`{"parking":true}`)})
	if err != nil {
		t.Fatal(err)
	}
	var result map[string]any
	if err := json.Unmarshal(data, &result); err != nil {
		t.Fatal(err)
	}
	if _, ok := result["facilities"].(map[string]any); !ok {
		t.Fatalf("facilities were encoded as text instead of JSON: %s", data)
	}
	for _, private := range []string{"seed_key", "deleted_at", "user_id", "password"} {
		if strings.Contains(string(data), private) {
			t.Fatalf("private field %q leaked", private)
		}
	}
}
