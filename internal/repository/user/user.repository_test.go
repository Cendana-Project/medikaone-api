package user

import (
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"gorm.io/gorm"
)

func TestHospitalWorkerDOBMeetsMinimumRequiresOlderThanFifteen(t *testing.T) {
	now := time.Date(2026, 9, 8, 23, 59, 0, 0, time.FixedZone("test", 9*60*60))
	todayUTC := time.Date(2026, 9, 8, 0, 0, 0, 0, time.UTC)
	if hospitalWorkerDOBMeetsMinimum(todayUTC.AddDate(-15, 0, 0), now) {
		t.Fatal("a worker exactly fifteen years old must be rejected")
	}
	if !hospitalWorkerDOBMeetsMinimum(todayUTC.AddDate(-15, 0, -1), now) {
		t.Fatal("a worker older than fifteen years must be accepted")
	}
}

func TestMapDoctorProfileWriteError(t *testing.T) {
	tests := []struct {
		name         string
		err          error
		wantConflict bool
	}{
		{name: "gorm translated duplicate", err: gorm.ErrDuplicatedKey, wantConflict: true},
		{
			name: "normalized SIP index",
			err: &pgconn.PgError{
				Code: "23505", ConstraintName: "ux_doctor_profiles_sip_number_normalized",
			},
			wantConflict: true,
		},
		{
			name: "legacy exact SIP constraint",
			err: &pgconn.PgError{
				Code: "23505", ConstraintName: "doctor_profiles_sip_number_key",
			},
			wantConflict: true,
		},
		{
			name: "unrelated unique constraint",
			err: &pgconn.PgError{
				Code: "23505", ConstraintName: "some_other_constraint",
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := mapDoctorProfileWriteError(test.err)
			if errors.Is(got, ErrDoctorSIPConflict) != test.wantConflict {
				t.Fatalf("mapDoctorProfileWriteError() = %v, conflict = %v", got, test.wantConflict)
			}
			if test.wantConflict && !errors.Is(got, test.err) {
				t.Fatalf("mapped error must preserve cause %v: %v", test.err, got)
			}
		})
	}
}
