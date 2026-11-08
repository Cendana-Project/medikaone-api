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
			got := mapDoctorProfileWriteError(test.err, nil)
			if errors.Is(got, ErrDoctorSIPConflict) != test.wantConflict {
				t.Fatalf("mapDoctorProfileWriteError() = %v, conflict = %v", got, test.wantConflict)
			}
			if test.wantConflict && !errors.Is(got, test.err) {
				t.Fatalf("mapped error must preserve cause %v: %v", test.err, got)
			}
		})
	}
}

func TestMapDoctorSpecialtyConstraintErrors(t *testing.T) {
	for _, constraint := range []string{"doctor_profiles_specialty_id_fkey", "doctor_profiles_active_specialty", "unrelated_fk"} {
		t.Run(constraint, func(t *testing.T) {
			cause := &pgconn.PgError{Code: "23503", ConstraintName: constraint}
			got := mapDoctorProfileWriteError(cause, nil)
			wantMissing := constraint != "unrelated_fk"
			if errors.Is(got, ErrDoctorSpecialtyNotFound) != wantMissing || !errors.Is(got, cause) {
				t.Fatalf("mapped specialty constraint = %v, missing=%v", got, wantMissing)
			}
			if errors.Is(got, ErrDoctorSIPConflict) {
				t.Fatalf("foreign key failure must not be reported as duplicate SIP: %v", got)
			}
		})
	}
}

func TestTranslatedDoctorForeignKeyRequiresSelectedSpecialty(t *testing.T) {
	id := "11111111-1111-4111-8111-111111111111"
	for _, selection := range []any{nil, (*string)(nil), id, &id} {
		got := mapDoctorProfileWriteError(gorm.ErrForeignKeyViolated, selection)
		wantMissing := selection == id || selection == &id
		if errors.Is(got, ErrDoctorSpecialtyNotFound) != wantMissing || !errors.Is(got, gorm.ErrForeignKeyViolated) {
			t.Fatalf("selection=%#v mapped translated error=%v", selection, got)
		}
	}
}
