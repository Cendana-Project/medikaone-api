package appointment

import (
	"context"
	"errors"
	"sort"
	"strings"
	"time"

	"gorm.io/gorm"

	"github.com/Cendana-Project/medikaone-api/internal/constant"
	"github.com/Cendana-Project/medikaone-api/internal/model/request"
	"github.com/Cendana-Project/medikaone-api/internal/model/response"
	"github.com/Cendana-Project/medikaone-api/internal/util"
)

func (s *Service) ListGroupedAvailability(ctx context.Context, query request.GroupedAvailabilityQuery) (*response.GroupedDoctorAvailability, error) {
	query.DoctorID, query.HospitalID = strings.TrimSpace(query.DoctorID), strings.TrimSpace(query.HospitalID)
	query.DateFrom, query.DateTo = strings.TrimSpace(query.DateFrom), strings.TrimSpace(query.DateTo)
	if err := util.ValidateStruct(query); err != nil {
		return nil, err
	}
	now := s.now()
	from, to, err := normalizeDateRange(query.DateFrom, query.DateTo, now)
	if err != nil {
		return nil, err
	}
	doctor, err := s.repo.GetAvailabilityDoctor(ctx, query.DoctorID)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, constant.ErrDoctorNotFound
	}
	if err != nil || doctor == nil {
		return nil, constant.ErrInternalServerError
	}
	rows, err := s.listAvailability(ctx, query.HospitalID, query.DoctorID, from.Format("2006-01-02"), to.Format("2006-01-02"), now, true)
	if err != nil {
		return nil, err
	}
	out := &response.GroupedDoctorAvailability{
		AvailabilityDoctor: *doctor, DateFrom: from.Format("2006-01-02"), DateTo: to.Format("2006-01-02"),
		Dates: make([]response.AvailabilityDate, 0),
	}
	byDate := make(map[string][]response.DoctorScheduleAvailability)
	for _, row := range rows {
		byDate[row.Date] = append(byDate[row.Date], row)
	}
	for date := from; !date.After(to); date = date.AddDate(0, 0, 1) {
		day := response.AvailabilityDate{Date: date.Format("2006-01-02"), Hospitals: make([]response.AvailabilityHospital, 0)}
		hospitalIndexes := make(map[string]int)
		for _, row := range byDate[day.Date] {
			location, err := time.LoadLocation(row.Timezone)
			if err != nil {
				return nil, constant.ErrInternalServerError
			}
			index, exists := hospitalIndexes[row.HospitalID]
			if !exists {
				index = len(day.Hospitals)
				hospitalIndexes[row.HospitalID] = index
				day.Hospitals = append(day.Hospitals, response.AvailabilityHospital{
					HospitalID: row.HospitalID, HospitalName: row.HospitalName, Slots: make([]response.GroupedAvailabilitySlot, 0),
				})
			}
			hospital := &day.Hospitals[index]
			for _, slot := range row.Slots {
				item := response.GroupedAvailabilitySlot{
					ScheduleID: row.ScheduleID, AffiliationID: row.AffiliationID,
					DepartmentID: row.DepartmentID, DepartmentName: row.DepartmentName, RoomID: row.RoomID, RoomName: row.RoomName,
					ScheduleType: "RECURRING", BookingMode: row.BookingMode, Timezone: row.Timezone,
					StartTime: slot.StartAt.In(location).Format("15:04"), EndTime: slot.EndAt.In(location).Format("15:04"),
					StartAt: slot.StartAt, EndAt: slot.EndAt, SlotDurationMinutes: row.SlotDurationMinutes,
					Capacity: slot.Capacity, AvailableCapacity: slot.AvailableCapacity,
				}
				if row.ScheduleDate != nil {
					item.ScheduleType = "SPECIFIC"
				}
				item.Status, item.UnavailableReason = availabilityStatus(slot, now)
				item.IsBookable = item.Status == "AVAILABLE"
				hospital.Slots = append(hospital.Slots, item)
				hospital.HasAvailableSlots = hospital.HasAvailableSlots || item.IsBookable
				day.HasAvailableSlots = day.HasAvailableSlots || item.IsBookable
			}
		}
		for i := range day.Hospitals {
			slots := day.Hospitals[i].Slots
			sort.Slice(slots, func(i, j int) bool {
				if !slots[i].StartAt.Equal(slots[j].StartAt) {
					return slots[i].StartAt.Before(slots[j].StartAt)
				}
				return slots[i].ScheduleID < slots[j].ScheduleID
			})
		}
		sort.Slice(day.Hospitals, func(i, j int) bool {
			a, b := strings.ToLower(day.Hospitals[i].HospitalName), strings.ToLower(day.Hospitals[j].HospitalName)
			if a != b {
				return a < b
			}
			return day.Hospitals[i].HospitalID < day.Hospitals[j].HospitalID
		})
		out.Dates = append(out.Dates, day)
	}
	return out, nil
}

func availabilityStatus(slot response.AvailabilitySlot, now time.Time) (string, *string) {
	var reason string
	switch {
	case !slot.StartAt.After(now):
		reason = "PAST"
	case !slot.StartAt.After(now.Add(MinimumBookingLeadTime)):
		reason = "MINIMUM_LEAD_TIME"
	case slot.StartAt.After(now.Add(BookingHorizon)):
		reason = "OUTSIDE_BOOKING_HORIZON"
	case slot.AvailableCapacity <= 0:
		reason = "CAPACITY_FULL"
		return "FULL", &reason
	default:
		return "AVAILABLE", nil
	}
	return "CLOSED", &reason
}
