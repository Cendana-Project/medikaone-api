package appointment

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"strings"
	"time"
	"unicode"

	"github.com/google/uuid"

	"github.com/Cendana-Project/medikaone-api/internal/constant"
	"github.com/Cendana-Project/medikaone-api/internal/model/entity"
	"github.com/Cendana-Project/medikaone-api/internal/model/request"
	"github.com/Cendana-Project/medikaone-api/internal/model/response"
	repository "github.com/Cendana-Project/medikaone-api/internal/repository/appointment"
)

const checkInGrantTTL = 5 * time.Minute

type checkInTokenClaims struct {
	Purpose           string `json:"purpose"`
	AppointmentID     string `json:"appointment_id"`
	AppointmentNumber string `json:"appointment_number"`
	HospitalID        string `json:"hospital_id"`
	ActorID           string `json:"actor_id,omitempty"`
	Method            string `json:"method"`
	ExpiresAt         int64  `json:"expires_at"`
}

func (s *Service) LookupCheckIn(ctx context.Context, hospitalID, actorID string, req request.CheckInLookupRequest) (*response.CheckInLookupResult, error) {
	modeCount := 0
	if strings.TrimSpace(req.QRPayload) != "" {
		modeCount++
	}
	if strings.TrimSpace(req.AppointmentNumber) != "" || strings.TrimSpace(req.VerificationCode) != "" {
		modeCount++
	}
	if req.Identity != nil {
		modeCount++
	}
	if modeCount != 1 {
		return nil, constant.ErrCheckInLookupModeInvalid
	}

	var rows []response.Appointment
	method := ""
	switch {
	case strings.TrimSpace(req.QRPayload) != "":
		claims, err := s.parseOperationalToken(strings.TrimSpace(req.QRPayload), "appointment_qr")
		if err != nil {
			return nil, constant.ErrAppointmentQRInvalid
		}
		if claims.HospitalID != hospitalID {
			return nil, constant.ErrAppointmentNotFound
		}
		row, err := s.repo.GetAppointment(ctx, claims.AppointmentID)
		if err != nil {
			return nil, mapRepositoryError(err)
		}
		if row.HospitalID != hospitalID || row.AppointmentNumber != claims.AppointmentNumber {
			return nil, constant.ErrAppointmentQRInvalid
		}
		rows, method = []response.Appointment{*row}, entity.CheckInMethodQR

	case strings.TrimSpace(req.AppointmentNumber) != "" || strings.TrimSpace(req.VerificationCode) != "":
		if strings.TrimSpace(req.AppointmentNumber) == "" {
			return nil, constant.NewFieldRequiredError("appointment_number")
		}
		if strings.TrimSpace(req.VerificationCode) == "" {
			return nil, constant.NewFieldRequiredError("verification_code")
		}
		row, err := s.repo.GetAppointmentByNumber(ctx, hospitalID, strings.TrimSpace(req.AppointmentNumber))
		if err != nil {
			return nil, mapRepositoryError(err)
		}
		expected := s.verificationCode(row.ID, row.AppointmentNumber)
		provided := strings.ToUpper(strings.TrimSpace(req.VerificationCode))
		if subtle.ConstantTimeCompare([]byte(expected), []byte(provided)) != 1 {
			return nil, constant.ErrAppointmentVerificationCodeInvalid
		}
		rows, method = []response.Appointment{*row}, entity.CheckInMethodCode

	default:
		filter, err := normalizeCheckInIdentity(*req.Identity)
		if err != nil {
			return nil, err
		}
		date := ""
		if req.AppointmentDate != nil {
			date = strings.TrimSpace(*req.AppointmentDate)
		}
		if date == "" {
			location, _ := time.LoadLocation(DefaultTimezone)
			date = s.now().In(location).Format("2006-01-02")
		}
		if _, err := time.Parse("2006-01-02", date); err != nil {
			return nil, constant.ErrInvalidDateFormat
		}
		rows, err = s.repo.FindCheckInAppointments(ctx, hospitalID, date, filter)
		if err != nil {
			return nil, constant.ErrInternalServerError
		}
		method = entity.CheckInMethodIdentity
	}

	if len(rows) == 0 {
		return nil, constant.ErrAppointmentNotFound
	}
	now := s.now()
	result := &response.CheckInLookupResult{Candidates: make([]response.CheckInCandidate, 0, len(rows))}
	for i := range rows {
		row := rows[i]
		if err := checkInLookupState(row.Status); err != nil {
			if len(rows) == 1 {
				return nil, err
			}
			continue
		}
		record, err := s.repo.GetPatientRecordForAppointment(ctx, row.ID)
		if err != nil {
			return nil, mapRepositoryError(err)
		}
		expiresAt := now.Add(checkInGrantTTL)
		claims := checkInTokenClaims{
			Purpose: "check_in_grant", AppointmentID: row.ID,
			AppointmentNumber: row.AppointmentNumber, HospitalID: hospitalID,
			ActorID: actorID, Method: method, ExpiresAt: expiresAt.Unix(),
		}
		token, err := s.signOperationalToken(claims)
		if err != nil {
			return nil, constant.ErrInternalServerError
		}
		redactClinicalFields(&row)
		result.Candidates = append(result.Candidates, response.CheckInCandidate{
			Appointment: row, Patient: patientPreview(record), CheckInToken: token,
			TokenExpiresAt: expiresAt, LookupMethod: method,
			LateOverrideRequired: row.Status == entity.AppointmentNoShow || now.After(row.ScheduledStartAt.Add(CheckInLateWindow)),
		})
	}
	if len(result.Candidates) == 0 {
		return nil, constant.ErrAppointmentInvalidState
	}
	result.Count = len(result.Candidates)
	return result, nil
}

func (s *Service) ConfirmCheckIn(ctx context.Context, hospitalID, actorID, appointmentID string, req request.ConfirmCheckInRequest) (*response.Appointment, error) {
	if _, err := uuid.Parse(appointmentID); err != nil {
		return nil, constant.ErrInvalidUUIDFormat
	}
	claims, err := s.parseOperationalToken(strings.TrimSpace(req.CheckInToken), "check_in_grant")
	if err != nil {
		return nil, constant.ErrCheckInTokenInvalidOrExpired
	}
	if claims.AppointmentID != appointmentID || claims.HospitalID != hospitalID || claims.ActorID != actorID {
		return nil, constant.ErrCheckInTokenInvalidOrExpired
	}
	row, err := s.repo.GetAppointment(ctx, appointmentID)
	if err != nil {
		return nil, mapRepositoryError(err)
	}
	if row.HospitalID != hospitalID || row.AppointmentNumber != claims.AppointmentNumber {
		return nil, constant.ErrAppointmentNotFound
	}
	if err := checkInLookupState(row.Status); err != nil {
		return nil, err
	}

	now := s.now()
	location, err := time.LoadLocation(row.Timezone)
	if err != nil {
		location, _ = time.LoadLocation(DefaultTimezone)
	}
	appointmentDate := row.AppointmentDate
	today := now.In(location).Format("2006-01-02")
	if today != appointmentDate {
		if today > appointmentDate {
			return nil, constant.ErrAppointmentCheckInExpired
		}
		return nil, constant.ErrAppointmentOutsideCheckInWindow
	}
	if now.Before(row.ScheduledStartAt.Add(-CheckInEarlyWindow)) {
		return nil, constant.ErrAppointmentOutsideCheckInWindow
	}
	late := row.Status == entity.AppointmentNoShow || now.After(row.ScheduledStartAt.Add(CheckInLateWindow))
	overrideReason := cleanOptional(req.OverrideReason)
	if late && overrideReason == nil {
		return nil, constant.ErrAppointmentLateOverrideReasonRequired
	}
	if err := s.repo.CheckIn(ctx, row.ID, actorID, claims.Method, overrideReason, now); err != nil {
		return nil, mapRepositoryError(err)
	}
	return s.GetHospitalAppointment(ctx, hospitalID, row.ID)
}

// CheckIn keeps the original one-call code flow compatible for existing
// clients. New receptionist clients should use LookupCheckIn then
// ConfirmCheckIn so patient details can be reviewed before state mutation.
func (s *Service) CheckInLegacy(ctx context.Context, hospitalID, actorID string, req request.VerifyAppointmentRequest) (*response.Appointment, error) {
	lookup, err := s.LookupCheckIn(ctx, hospitalID, actorID, request.CheckInLookupRequest{
		AppointmentNumber: req.AppointmentNumber,
		VerificationCode:  req.VerificationCode,
	})
	if err != nil {
		return nil, err
	}
	if len(lookup.Candidates) != 1 {
		return nil, constant.ErrAppointmentNotFound
	}
	return s.ConfirmCheckIn(ctx, hospitalID, actorID, lookup.Candidates[0].Appointment.ID, request.ConfirmCheckInRequest{
		CheckInToken:   lookup.Candidates[0].CheckInToken,
		OverrideReason: req.OverrideReason,
	})
}

func (s *Service) CreateWalkInAppointment(ctx context.Context, hospitalID, actorID, idempotencyKey string, req request.CreateWalkInAppointmentRequest) (*response.Appointment, bool, error) {
	if err := validateIdempotencyKey(idempotencyKey); err != nil {
		return nil, false, err
	}
	schedule, err := s.repo.GetActiveSchedule(ctx, req.ScheduleID)
	if err != nil {
		return nil, false, mapRepositoryError(err)
	}
	if schedule.HospitalID != hospitalID {
		return nil, false, constant.ErrScheduleNotFound
	}
	location, err := time.LoadLocation(schedule.Timezone)
	if err != nil {
		return nil, false, constant.ErrInternalServerError
	}
	now := s.now()
	localNow := now.In(location)
	date := time.Date(localNow.Year(), localNow.Month(), localNow.Day(), 0, 0, 0, 0, location)
	if int(date.Weekday()) != schedule.DayOfWeek {
		return nil, false, constant.ErrScheduleNotFound
	}
	sessionStart, sessionEnd, err := scheduleWindow(*schedule, date)
	if err != nil || !sessionEnd.After(now) {
		return nil, false, constant.ErrAppointmentSlotUnavailable
	}
	start, end := sessionStart, sessionEnd
	if schedule.BookingMode == entity.BookingModeFixedSlot {
		if req.StartTime == nil || strings.TrimSpace(*req.StartTime) == "" {
			return nil, false, constant.NewFieldRequiredError("start_time")
		}
		clock, parseErr := time.Parse("15:04", strings.TrimSpace(*req.StartTime))
		if parseErr != nil {
			return nil, false, constant.NewInvalidFieldValueError("start_time", "a valid HH:mm time", "berupa waktu HH:mm yang valid")
		}
		start = time.Date(date.Year(), date.Month(), date.Day(), clock.Hour(), clock.Minute(), 0, 0, location).UTC()
		end = start.Add(time.Duration(schedule.SlotDurationMinutes) * time.Minute)
		if start.Before(sessionStart) || end.After(sessionEnd) || !end.After(now) ||
			int(start.Sub(sessionStart).Minutes())%schedule.SlotDurationMinutes != 0 {
			return nil, false, constant.ErrAppointmentSlotUnavailable
		}
	} else if req.StartTime != nil && strings.TrimSpace(*req.StartTime) != "" && strings.TrimSpace(*req.StartTime) != schedule.StartTime {
		return nil, false, constant.NewInvalidFieldValueError("start_time", "empty or equal to the session start for SESSION_QUEUE", "kosong atau sama dengan awal sesi untuk SESSION_QUEUE")
	}
	patient, err := normalizeWalkInPatient(req.Patient)
	if err != nil {
		return nil, false, err
	}
	overrideReason := cleanOptional(req.CapacityOverrideReason)
	if req.CapacityOverride {
		if overrideReason == nil {
			return nil, false, constant.ErrWalkInCapacityOverrideReasonRequired
		}
		allowed, permissionErr := s.repo.CanOverrideWalkInCapacity(ctx, hospitalID, actorID)
		if permissionErr != nil {
			return nil, false, constant.ErrInternalServerError
		}
		if !allowed {
			return nil, false, constant.ErrWalkInCapacityOverrideForbidden
		}
	} else if overrideReason != nil {
		return nil, false, constant.NewInvalidFieldValueError("capacity_override_reason", "empty when capacity_override is false", "kosong ketika capacity_override bernilai false")
	}

	reason := strings.TrimSpace(req.ReasonForVisit)
	consentVersion := strings.TrimSpace(req.ConsentVersion)
	hashPayload, _ := json.Marshal(struct {
		HospitalID, ActorID, ScheduleID, StartAt, Reason, ConsentVersion string
		Patient                                                          repository.WalkInPatientInput
		Note                                                             *string
		CapacityOverride                                                 bool
	}{hospitalID, actorID, schedule.ID, start.Format(time.RFC3339Nano), reason, consentVersion, patient, cleanOptional(req.Note), req.CapacityOverride})
	hash := sha256.Sum256(hashPayload)
	row, replay, err := s.repo.CreateWalkIn(ctx, repository.WalkInInput{
		ActorID: actorID, Patient: patient, Schedule: *schedule,
		AppointmentDate: date.Format("2006-01-02"), ScheduledStartAt: start,
		ScheduledEndAt: end, ReasonForVisit: reason, Note: cleanOptional(req.Note),
		ConsentVersion: consentVersion, IdempotencyKey: strings.TrimSpace(idempotencyKey),
		IdempotencyRequestHash: hex.EncodeToString(hash[:]), CapacityOverride: req.CapacityOverride,
		CapacityOverrideReason: overrideReason, Now: now,
	})
	if err != nil {
		return nil, false, mapRepositoryError(err)
	}
	redactClinicalFields(row)
	return row, replay, nil
}

func (s *Service) ClaimPatientRecord(ctx context.Context, userID string, req request.ClaimPatientRecordRequest) (*response.PatientRecord, error) {
	identityType := strings.ToUpper(strings.TrimSpace(req.IdentityType))
	identityNumber := strings.TrimSpace(req.IdentityNumber)
	if normalizeIdentityNumber(identityNumber) == "" {
		return nil, constant.ErrPatientRecordIdentityMismatch
	}
	if _, err := time.Parse("2006-01-02", strings.TrimSpace(req.DateOfBirth)); err != nil {
		return nil, constant.ErrInvalidDateFormat
	}
	row, err := s.repo.ClaimPatientRecord(ctx, userID, identityType, identityNumber, strings.TrimSpace(req.DateOfBirth), s.now())
	if err != nil {
		return nil, mapRepositoryError(err)
	}
	result := patientRecordResponse(row)
	return &result, nil
}

func (s *Service) ListHospitalQueuePage(ctx context.Context, filter repository.QueueFilter) (*response.AppointmentPage, error) {
	if filter.Date == "" {
		location, _ := time.LoadLocation(DefaultTimezone)
		filter.Date = s.now().In(location).Format("2006-01-02")
	}
	if err := validateOptionalDate(filter.Date); err != nil {
		return nil, err
	}
	for _, value := range []string{filter.DoctorID, filter.DepartmentID} {
		if value != "" {
			if _, err := uuid.Parse(value); err != nil {
				return nil, constant.ErrInvalidUUIDFormat
			}
		}
	}
	filter.Status = strings.ToUpper(strings.TrimSpace(filter.Status))
	if filter.Status != "" && filter.Status != entity.AppointmentWaitingVitals &&
		filter.Status != entity.AppointmentWaitingDoctor && filter.Status != entity.AppointmentInConsultation {
		return nil, constant.NewInvalidFieldValueError("status", "WAITING_VITALS, WAITING_DOCTOR, or IN_CONSULTATION", "WAITING_VITALS, WAITING_DOCTOR, atau IN_CONSULTATION")
	}
	filter.BookingMode = strings.ToUpper(strings.TrimSpace(filter.BookingMode))
	if filter.BookingMode != "" && filter.BookingMode != entity.BookingModeFixedSlot && filter.BookingMode != entity.BookingModeSessionQueue {
		return nil, constant.NewInvalidFieldValueError("booking_mode", "FIXED_SLOT or SESSION_QUEUE", "FIXED_SLOT atau SESSION_QUEUE")
	}
	result, err := s.repo.ListQueue(ctx, filter)
	if err != nil {
		return nil, constant.ErrInternalServerError
	}
	for i := range result.Items {
		redactClinicalFields(&result.Items[i])
	}
	return result, nil
}

func (s *Service) decoratePatientAppointment(row *response.Appointment) {
	row.VerificationCode = s.verificationCode(row.ID, row.AppointmentNumber)
	location, err := time.LoadLocation(row.Timezone)
	if err != nil {
		location, _ = time.LoadLocation(DefaultTimezone)
	}
	date, err := time.ParseInLocation("2006-01-02", row.AppointmentDate, location)
	if err != nil {
		return
	}
	claims := checkInTokenClaims{
		Purpose: "appointment_qr", AppointmentID: row.ID,
		AppointmentNumber: row.AppointmentNumber, HospitalID: row.HospitalID,
		Method: entity.CheckInMethodQR, ExpiresAt: date.AddDate(0, 0, 1).Unix(),
	}
	if token, err := s.signOperationalToken(claims); err == nil {
		row.QRPayload = token
	}
}

func (s *Service) signOperationalToken(claims checkInTokenClaims) (string, error) {
	raw, err := json.Marshal(claims)
	if err != nil {
		return "", err
	}
	payload := base64.RawURLEncoding.EncodeToString(raw)
	mac := hmac.New(sha256.New, s.secret)
	_, _ = mac.Write([]byte("appointment-operation:" + payload))
	signature := base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
	return payload + "." + signature, nil
}

func (s *Service) parseOperationalToken(token, purpose string) (checkInTokenClaims, error) {
	var claims checkInTokenClaims
	parts := strings.Split(token, ".")
	if len(parts) != 2 {
		return claims, constant.ErrCheckInTokenInvalidOrExpired
	}
	mac := hmac.New(sha256.New, s.secret)
	_, _ = mac.Write([]byte("appointment-operation:" + parts[0]))
	expected := mac.Sum(nil)
	provided, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil || subtle.ConstantTimeCompare(expected, provided) != 1 {
		return claims, constant.ErrCheckInTokenInvalidOrExpired
	}
	raw, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil || json.Unmarshal(raw, &claims) != nil || claims.Purpose != purpose || claims.ExpiresAt < s.now().Unix() {
		return checkInTokenClaims{}, constant.ErrCheckInTokenInvalidOrExpired
	}
	return claims, nil
}

func normalizeCheckInIdentity(input request.CheckInIdentity) (repository.CheckInIdentityFilter, error) {
	filter := repository.CheckInIdentityFilter{
		MedikaOneID: strings.TrimSpace(input.MedikaOneID), NIK: strings.TrimSpace(input.NIK),
		IdentityType:   strings.ToUpper(strings.TrimSpace(input.IdentityType)),
		IdentityNumber: strings.TrimSpace(input.IdentityNumber),
		Email:          strings.ToLower(strings.TrimSpace(input.Email)), Phone: strings.TrimSpace(input.Phone),
		Name: strings.TrimSpace(input.Name),
	}
	if input.DateOfBirth != nil {
		filter.DateOfBirth = strings.TrimSpace(*input.DateOfBirth)
	}
	if (filter.IdentityType == "") != (filter.IdentityNumber == "") {
		return repository.CheckInIdentityFilter{}, constant.NewInvalidFieldValueError("identity", "both identity_type and identity_number", "identity_type dan identity_number sekaligus")
	}
	if filter.IdentityType != "" && filter.IdentityType != "NIK" && filter.IdentityType != "PASSPORT" &&
		filter.IdentityType != "OTHER" && filter.IdentityType != "MEDIKAONE_ID" {
		return repository.CheckInIdentityFilter{}, constant.NewInvalidFieldValueError("identity_type", "NIK, PASSPORT, OTHER, or MEDIKAONE_ID", "NIK, PASSPORT, OTHER, atau MEDIKAONE_ID")
	}
	if filter.NIK != "" && filter.IdentityNumber != "" {
		return repository.CheckInIdentityFilter{}, constant.NewInvalidFieldValueError("identity", "either nik or identity_type with identity_number, not both", "salah satu dari nik atau identity_type bersama identity_number, bukan keduanya")
	}
	count := 0
	for _, value := range []string{filter.MedikaOneID, filter.NIK, filter.IdentityNumber, filter.Email, filter.Phone, filter.Name, filter.DateOfBirth} {
		if value != "" {
			count++
		}
	}
	if count < 2 || (filter.Name != "" && filter.DateOfBirth == "") {
		return repository.CheckInIdentityFilter{}, constant.ErrCheckInIdentityInsufficient
	}
	return filter, nil
}

func normalizeWalkInPatient(input request.WalkInPatientRequest) (repository.WalkInPatientInput, error) {
	patient := repository.WalkInPatientInput{}
	hasPatientRecordID := input.PatientRecordID != nil && strings.TrimSpace(*input.PatientRecordID) != ""
	hasMedikaOneID := input.MedikaOneID != nil && strings.TrimSpace(*input.MedikaOneID) != ""
	hasIdentity := strings.TrimSpace(input.FirstName) != "" || strings.TrimSpace(input.LastName) != "" ||
		(input.Email != nil && strings.TrimSpace(*input.Email) != "") || strings.TrimSpace(input.Phone) != "" ||
		strings.TrimSpace(input.DateOfBirth) != "" || strings.TrimSpace(input.Gender) != "" ||
		strings.TrimSpace(input.IdentityType) != "" || strings.TrimSpace(input.IdentityNumber) != ""
	modeCount := 0
	for _, enabled := range []bool{hasPatientRecordID, hasMedikaOneID, hasIdentity} {
		if enabled {
			modeCount++
		}
	}
	if modeCount == 0 {
		return repository.WalkInPatientInput{}, constant.ErrWalkInPatientDataRequired
	}
	if modeCount > 1 {
		return repository.WalkInPatientInput{}, constant.ErrWalkInPatientModeInvalid
	}
	if hasPatientRecordID {
		patient.PatientRecordID = strings.TrimSpace(*input.PatientRecordID)
		return patient, nil
	}
	if hasMedikaOneID {
		patient.MedikaOneID = strings.TrimSpace(*input.MedikaOneID)
		return patient, nil
	}
	patient.FirstName = strings.TrimSpace(input.FirstName)
	patient.LastName = strings.TrimSpace(input.LastName)
	patient.Email = cleanOptional(input.Email)
	patient.Phone = strings.TrimSpace(input.Phone)
	patient.DateOfBirth = strings.TrimSpace(input.DateOfBirth)
	patient.Gender = strings.ToUpper(strings.TrimSpace(input.Gender))
	patient.IdentityType = strings.ToUpper(strings.TrimSpace(input.IdentityType))
	patient.IdentityNumber = strings.TrimSpace(input.IdentityNumber)
	patient.IdentityNormalized = normalizeIdentityNumber(patient.IdentityNumber)
	if patient.FirstName == "" || patient.Phone == "" || patient.DateOfBirth == "" ||
		patient.Gender == "" || patient.IdentityType == "" || patient.IdentityNormalized == "" {
		return repository.WalkInPatientInput{}, constant.ErrWalkInPatientDataRequired
	}
	if patient.Gender != "L" && patient.Gender != "P" {
		return repository.WalkInPatientInput{}, constant.NewInvalidFieldValueError("gender", "L or P", "L atau P")
	}
	if _, err := time.Parse("2006-01-02", patient.DateOfBirth); err != nil {
		return repository.WalkInPatientInput{}, constant.ErrInvalidDateFormat
	}
	if patient.IdentityType != "NIK" && patient.IdentityType != "PASSPORT" &&
		patient.IdentityType != "OTHER" && patient.IdentityType != "MEDIKAONE_ID" {
		return repository.WalkInPatientInput{}, constant.NewInvalidFieldValueError("identity_type", "NIK, PASSPORT, OTHER, or MEDIKAONE_ID", "NIK, PASSPORT, OTHER, atau MEDIKAONE_ID")
	}
	if patient.IdentityType == "NIK" && (len(patient.IdentityNormalized) != 16 || !isDigits(patient.IdentityNormalized)) {
		return repository.WalkInPatientInput{}, constant.NewInvalidFieldValueError("identity_number", "a 16-digit NIK", "NIK dengan 16 angka")
	}
	return patient, nil
}

func normalizeIdentityNumber(value string) string {
	var normalized strings.Builder
	for _, character := range strings.ToUpper(strings.TrimSpace(value)) {
		if unicode.IsLetter(character) || unicode.IsDigit(character) {
			normalized.WriteRune(character)
		}
	}
	return normalized.String()
}

func isDigits(value string) bool {
	for _, character := range value {
		if character < '0' || character > '9' {
			return false
		}
	}
	return value != ""
}

func checkInLookupState(status string) error {
	switch status {
	case entity.AppointmentConfirmed, entity.AppointmentNoShow:
		return nil
	case entity.AppointmentCheckedIn, entity.AppointmentWaitingVitals,
		entity.AppointmentWaitingDoctor, entity.AppointmentInConsultation, entity.AppointmentCompleted:
		return constant.ErrAppointmentAlreadyCheckedIn
	default:
		return constant.ErrAppointmentInvalidState
	}
}

func patientPreview(record *repository.PatientRecord) response.CheckInPatientPreview {
	return response.CheckInPatientPreview{
		PatientRecordID: record.ID, MedikaOneID: record.UserID,
		FullName:    strings.TrimSpace(record.FirstName + " " + record.LastName),
		DateOfBirth: record.DateOfBirth, Gender: record.Gender,
		IdentityType: record.IdentityType, IdentityNumberMasked: maskValue(record.IdentityNumber, 4),
		PhoneMasked: maskValue(record.Phone, 4), EmailMasked: maskEmail(record.Email),
	}
}

func patientRecordResponse(record *repository.PatientRecord) response.PatientRecord {
	return response.PatientRecord{
		ID: record.ID, UserID: record.UserID, FirstName: record.FirstName, LastName: record.LastName,
		FullName:    strings.TrimSpace(record.FirstName + " " + record.LastName),
		EmailMasked: maskEmail(record.Email), PhoneMasked: maskValue(record.Phone, 4),
		DateOfBirth: record.DateOfBirth, Gender: record.Gender, IdentityType: record.IdentityType,
		IdentityNumberMasked: maskValue(record.IdentityNumber, 4), ClaimedAt: record.ClaimedAt,
		CreatedAt: record.CreatedAt,
	}
}

func maskValue(value string, visible int) string {
	runes := []rune(strings.TrimSpace(value))
	if len(runes) <= visible {
		return strings.Repeat("*", len(runes))
	}
	return strings.Repeat("*", len(runes)-visible) + string(runes[len(runes)-visible:])
}

func maskEmail(value *string) *string {
	if value == nil || strings.TrimSpace(*value) == "" {
		return nil
	}
	parts := strings.SplitN(strings.TrimSpace(*value), "@", 2)
	masked := "***"
	if len(parts[0]) > 0 {
		masked = string([]rune(parts[0])[0]) + "***"
	}
	if len(parts) == 2 {
		masked += "@" + parts[1]
	}
	return &masked
}
