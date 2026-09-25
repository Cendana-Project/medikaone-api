package constant

import "net/http"

const (
	MsgDoctorsListed                 MessageCode = "DOCTORS_LISTED"
	MsgDoctorRetrieved               MessageCode = "DOCTOR_RETRIEVED"
	MsgHospitalsListed               MessageCode = "HOSPITALS_LISTED"
	MsgHospitalRetrieved             MessageCode = "HOSPITAL_RETRIEVED"
	MsgHospitalUpdated               MessageCode = "HOSPITAL_UPDATED"
	MsgHospitalDeleted               MessageCode = "HOSPITAL_DELETED"
	MsgHospitalDepartmentUpdated     MessageCode = "HOSPITAL_DEPARTMENT_UPDATED"
	MsgHospitalDepartmentDeleted     MessageCode = "HOSPITAL_DEPARTMENT_DELETED"
	MsgHospitalRoomUpdated           MessageCode = "HOSPITAL_ROOM_UPDATED"
	MsgHospitalRoomDeleted           MessageCode = "HOSPITAL_ROOM_DELETED"
	MsgDoctorInvitationUpdated       MessageCode = "DOCTOR_INVITATION_UPDATED"
	MsgDoctorInvitationDeleted       MessageCode = "DOCTOR_INVITATION_DELETED"
	MsgDoctorAffiliationDeleted      MessageCode = "DOCTOR_AFFILIATION_DELETED"
	MsgAccountDeleted                MessageCode = "ACCOUNT_DELETED"
	MsgNotificationDeleted           MessageCode = "NOTIFICATION_DELETED"
	MsgDoctorScheduleCreated         MessageCode = "DOCTOR_SCHEDULE_CREATED"
	MsgDoctorScheduleDeleted         MessageCode = "DOCTOR_SCHEDULE_DELETED"
	MsgDepartmentsListed             MessageCode = "DEPARTMENTS_LISTED"
	MsgDoctorRecommendationsListed   MessageCode = "DOCTOR_RECOMMENDATIONS_LISTED"
	MsgHospitalRecommendationsListed MessageCode = "HOSPITAL_RECOMMENDATIONS_LISTED"
)

var (
	ErrDoctorNotFound = apiError("DOCTOR_NOT_FOUND", http.StatusNotFound,
		"Doctor not found", "The requested doctor was not found.",
		"Dokter tidak ditemukan", "Dokter yang diminta tidak ditemukan.")
	ErrResourceInUse = apiError("RESOURCE_IN_USE", http.StatusConflict,
		"Resource is in use", "Resolve active appointments or dependent resources before deleting this resource.",
		"Data masih digunakan", "Selesaikan appointment aktif atau data yang bergantung sebelum menghapus data ini.")
	ErrLastAdministrator = apiError("LAST_ADMINISTRATOR", http.StatusConflict,
		"Administrator replacement required", "Assign another active administrator before deleting this account.",
		"Pengganti administrator diperlukan", "Tetapkan administrator aktif lain sebelum menghapus akun ini.")
)
