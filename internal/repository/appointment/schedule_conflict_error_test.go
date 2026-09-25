package appointment

import (
	"errors"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"
)

func TestMapScheduleConflictWriteError(t *testing.T) {
	constraintError := &pgconn.PgError{Code: "23P01", ConstraintName: "doctor_schedule_no_overlap"}
	mapped := mapScheduleConflictWriteError(constraintError)
	if !errors.Is(mapped, ErrDoctorScheduleConflict) || !errors.Is(mapped, constraintError) {
		t.Fatalf("mapped error must preserve schedule conflict and database cause: %v", mapped)
	}

	unrelated := &pgconn.PgError{Code: "23P01", ConstraintName: "another_constraint"}
	if got := mapScheduleConflictWriteError(unrelated); got != unrelated {
		t.Fatalf("unrelated error was changed: %v", got)
	}
}
