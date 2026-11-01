package appointment

import (
	"encoding/base64"
	"encoding/binary"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
)

const slotIDPrefix = "slt1_"

var errInvalidSlotID = errors.New("invalid slot reference")

// A slot reference identifies an interval, not permission or reserved capacity.
// Resolve it against the current schedule and booking rules on every use.
type slotReference struct {
	scheduleID string
	start      time.Time
	end        time.Time
}

func encodeSlotID(scheduleID string, start, end time.Time) (string, error) {
	id, err := uuid.Parse(scheduleID)
	if err != nil || id == uuid.Nil || !validSlotInterval(start, end) {
		return "", errInvalidSlotID
	}
	var payload [32]byte
	copy(payload[:16], id[:])
	binary.BigEndian.PutUint64(payload[16:24], uint64(start.Unix()))
	binary.BigEndian.PutUint64(payload[24:32], uint64(end.Unix()))
	var encoded [48]byte
	copy(encoded[:], slotIDPrefix)
	base64.RawURLEncoding.Encode(encoded[len(slotIDPrefix):], payload[:])
	return string(encoded[:]), nil
}

func decodeSlotID(value string) (slotReference, error) {
	if len(value) != 48 || !strings.HasPrefix(value, slotIDPrefix) {
		return slotReference{}, errInvalidSlotID
	}
	var payload [32]byte
	n, err := base64.RawURLEncoding.Strict().Decode(payload[:], []byte(value[len(slotIDPrefix):]))
	if err != nil || n != len(payload) {
		return slotReference{}, errInvalidSlotID
	}
	id, err := uuid.FromBytes(payload[:16])
	if err != nil || id == uuid.Nil {
		return slotReference{}, errInvalidSlotID
	}
	startSeconds, endSeconds := binary.BigEndian.Uint64(payload[16:24]), binary.BigEndian.Uint64(payload[24:32])
	// Only positive Unix seconds represent supported civil appointment dates.
	const maxUnixSeconds = uint64(253402300799) // 9999-12-31T23:59:59Z
	if startSeconds == 0 || endSeconds > maxUnixSeconds || endSeconds <= startSeconds {
		return slotReference{}, errInvalidSlotID
	}
	return slotReference{scheduleID: id.String(), start: time.Unix(int64(startSeconds), 0).UTC(), end: time.Unix(int64(endSeconds), 0).UTC()}, nil
}

func validSlotInterval(start, end time.Time) bool {
	return start.Unix() > 0 && end.Unix() <= 253402300799 && end.After(start) && start.Nanosecond() == 0 && end.Nanosecond() == 0
}
