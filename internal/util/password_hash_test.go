package util

import (
	"context"
	"errors"
	"testing"
)

func TestPasswordHashAdmissionRejectsWhenAllSlotsAreBusy(t *testing.T) {
	passwordWorkSlots <- struct{}{}
	passwordWorkSlots <- struct{}{}
	t.Cleanup(func() {
		<-passwordWorkSlots
		<-passwordWorkSlots
	})

	if _, err := HashPasswordScrypt(context.Background(), "Strong!Pass9"); !errors.Is(err, ErrPasswordWorkLimit) {
		t.Fatalf("HashPasswordScrypt() error = %v, want ErrPasswordWorkLimit", err)
	}
}

func TestSharedScryptHashRoundTrip(t *testing.T) {
	hash, err := HashPasswordScrypt(context.Background(), " Strong!Pass9 ")
	if err != nil {
		t.Fatal(err)
	}
	matched, err := VerifyPasswordScrypt(context.Background(), hash, " Strong!Pass9 ")
	if err != nil || !matched {
		t.Fatalf("exact password match = %v, err = %v", matched, err)
	}
	matched, err = VerifyPasswordScrypt(context.Background(), hash, "Strong!Pass9")
	if err != nil || matched {
		t.Fatalf("trimmed password match = %v, err = %v", matched, err)
	}
}
