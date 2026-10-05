package middleware

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/Cendana-Project/medikaone-api/internal/constant"
	"github.com/Cendana-Project/medikaone-api/internal/model/entity"
)

type fakeHospitalPermissionResolver struct {
	id   string
	hint string
}

func (f *fakeHospitalPermissionResolver) ResolveHospitalID(_ context.Context, hint string) (string, error) {
	f.hint = hint
	return f.id, nil
}

type fakeHospitalPermissionReader struct {
	hospitalID string
	userID     string
	role       string
	super      bool
	err        error
	queried    bool
}

func (f *fakeHospitalPermissionReader) IsUserSuperAdmin(context.Context, string) (bool, error) {
	return f.super, nil
}

func (f *fakeHospitalPermissionReader) ListHospitalPermissionsByUser(_ context.Context, hospitalID, userID string) ([]entity.Permission, error) {
	f.queried = true
	if f.err != nil {
		return nil, f.err
	}
	// Model an active membership only in the configured hospital. Global roles
	// alone or membership elsewhere must not authorize a tenant operation.
	if hospitalID != f.hospitalID || userID != f.userID {
		return nil, nil
	}
	var permissions []entity.Permission
	for _, slug := range constant.DefaultRolePermissions[f.role] {
		permissions = append(permissions, entity.Permission{Slug: slug})
	}
	return permissions, nil
}

func TestHospitalStaffOperationalPermissions(t *testing.T) {
	previousMode := gin.Mode()
	gin.SetMode(gin.TestMode)
	t.Cleanup(func() { gin.SetMode(previousMode) })
	const hospitalID = "22222222-2222-4222-8222-222222222222"
	const userID = "11111111-1111-4111-8111-111111111111"
	cases := []struct {
		name       string
		role       string
		permission string
		memberOf   string
		super      bool
		want       int
	}{
		{"receptionist reads doctors and schedules", constant.RoleReceptionist, constant.PermissionDoctorScheduleView, hospitalID, false, http.StatusOK},
		{"nurse reads doctors and schedules", constant.RoleNurse, constant.PermissionDoctorScheduleView, hospitalID, false, http.StatusOK},
		{"admin retains read access", constant.RoleAdmin, constant.PermissionDoctorScheduleView, hospitalID, false, http.StatusOK},
		{"super admin does not need membership", constant.RoleSuperAdmin, constant.PermissionDoctorScheduleView, "", true, http.StatusOK},
		{"receptionist cannot read another hospital", constant.RoleReceptionist, constant.PermissionDoctorScheduleView, "another-hospital", false, http.StatusForbidden},
		{"nurse cannot read another hospital", constant.RoleNurse, constant.PermissionDoctorScheduleView, "another-hospital", false, http.StatusForbidden},
		{"global doctor role alone is insufficient", constant.RoleDoctor, constant.PermissionDoctorScheduleView, "", false, http.StatusForbidden},
		{"BOD lacks schedule permission", constant.RoleBOD, constant.PermissionDoctorScheduleView, hospitalID, false, http.StatusForbidden},
		{"patient lacks schedule permission", constant.RolePatient, constant.PermissionDoctorScheduleView, hospitalID, false, http.StatusForbidden},
		{"receptionist can check in", constant.RoleReceptionist, constant.PermissionAppointmentCheckIn, hospitalID, false, http.StatusOK},
		{"nurse cannot check in", constant.RoleNurse, constant.PermissionAppointmentCheckIn, hospitalID, false, http.StatusForbidden},
		{"receptionist cannot change schedules", constant.RoleReceptionist, constant.PermissionDoctorSchedulePropose, hospitalID, false, http.StatusForbidden},
		{"nurse cannot approve schedules", constant.RoleNurse, constant.PermissionDoctorScheduleApprove, hospitalID, false, http.StatusForbidden},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			resolver := &fakeHospitalPermissionResolver{id: hospitalID}
			reader := &fakeHospitalPermissionReader{hospitalID: tc.memberOf, userID: userID, role: tc.role, super: tc.super}
			router := gin.New()
			router.Use(func(c *gin.Context) { c.Set("user_id", userID) }, TenantContext())
			handled := false
			router.GET("/v1/hospitals/:hospital_id/resource", requireHospitalPermissions(resolver, reader, tc.permission), func(c *gin.Context) {
				handled = true
				if c.GetString("hospital_id") != hospitalID {
					t.Fatal("handler did not receive the resolved hospital ID")
				}
				c.JSON(http.StatusOK, gin.H{"data": []any{}})
			})
			req := httptest.NewRequest(http.MethodGet, "/v1/hospitals/HSP-MO-001/resource", nil)
			// A header cannot replace the hospital selected by the route.
			req.Header.Set("X-Hospital-ID", tc.memberOf)
			recorder := httptest.NewRecorder()
			router.ServeHTTP(recorder, req)
			if recorder.Code != tc.want || handled != (tc.want == http.StatusOK) {
				t.Fatalf("status=%d handled=%v body=%s", recorder.Code, handled, recorder.Body.String())
			}
			if resolver.hint != "HSP-MO-001" || reader.queried == tc.super {
				t.Fatalf("unexpected scope lookup: hint=%q queried=%v", resolver.hint, reader.queried)
			}
			if tc.want == http.StatusForbidden && (!strings.Contains(recorder.Body.String(), "REQUIRED_PERMISSION_MISSING") || !strings.Contains(recorder.Body.String(), tc.permission)) {
				t.Fatalf("missing actionable permission error: %s", recorder.Body.String())
			}
		})
	}
}

func TestHospitalPermissionLookupFailsClosed(t *testing.T) {
	previousMode := gin.Mode()
	gin.SetMode(gin.TestMode)
	t.Cleanup(func() { gin.SetMode(previousMode) })
	cases := []struct {
		name  string
		guard gin.HandlerFunc
		user  string
		want  int
	}{
		{"anonymous", RequireHospitalPermissions(nil, nil, constant.PermissionDoctorScheduleView), "", http.StatusUnauthorized},
		{"missing repositories", RequireHospitalPermissions(nil, nil, constant.PermissionDoctorScheduleView), "staff", http.StatusInternalServerError},
		{"repository failure", requireHospitalPermissions(&fakeHospitalPermissionResolver{id: "hospital"}, &fakeHospitalPermissionReader{err: errors.New("unavailable")}, constant.PermissionDoctorScheduleView), "staff", http.StatusInternalServerError},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			router := gin.New()
			router.Use(func(c *gin.Context) { c.Set("user_id", tc.user) }, TenantContext())
			router.GET("/v1/hospitals/:hospital_id/doctors", tc.guard, func(c *gin.Context) {
				t.Error("unauthorized request reached handler")
			})
			recorder := httptest.NewRecorder()
			router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/v1/hospitals/HSP-MO-001/doctors", nil))
			if recorder.Code != tc.want {
				t.Fatalf("status=%d want=%d", recorder.Code, tc.want)
			}
		})
	}
}
