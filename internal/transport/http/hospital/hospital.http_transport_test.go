package hospital

import (
	"testing"

	"github.com/gin-gonic/gin"
)

func TestHospitalHintPrefersCanonicalMiddlewareValue(t *testing.T) {
	c, _ := gin.CreateTestContext(nil)
	c.Params = gin.Params{{Key: "hospital_id", Value: "path-hospital"}}
	c.Set("hospital_hint", "hint-hospital")
	c.Set("hospital_id", "canonical-hospital")

	if got := hospitalHint(c); got != "canonical-hospital" {
		t.Fatalf("got %q, want canonical hospital", got)
	}
}

func TestHospitalHintFallsBackToAuthorizedPathHint(t *testing.T) {
	c, _ := gin.CreateTestContext(nil)
	c.Params = gin.Params{{Key: "hospital_id", Value: "path-hospital"}}
	c.Set("hospital_hint", "path-hospital")

	if got := hospitalHint(c); got != "path-hospital" {
		t.Fatalf("got %q, want path hospital", got)
	}
}
