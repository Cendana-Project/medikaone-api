package constant

import "github.com/Cendana-Project/medikaone-api/internal/model/response"

// MessageCode is a stable machine-readable code for a successful API outcome.
// Error outcomes use response.CustomError.Code.
type MessageCode string

const (
	MsgSuccess MessageCode = "SUCCESS"

	MsgServiceHealthy MessageCode = "SERVICE_HEALTHY"
	MsgServiceAlive   MessageCode = "SERVICE_ALIVE"
	MsgServiceReady   MessageCode = "SERVICE_READY"

	MsgRegistrationChallengeCreated MessageCode = "REGISTRATION_CHALLENGE_CREATED"
	MsgRegistrationPINResent        MessageCode = "REGISTRATION_PIN_RESENT"
	MsgRegistrationVerified         MessageCode = "REGISTRATION_VERIFIED"
	MsgPublicLoginSucceeded         MessageCode = "PUBLIC_LOGIN_SUCCEEDED"
	MsgHospitalLoginSucceeded       MessageCode = "HOSPITAL_LOGIN_SUCCEEDED"
	MsgTokensRefreshed              MessageCode = "TOKENS_REFRESHED"
	MsgPatientRoleSelected          MessageCode = "PATIENT_ROLE_SELECTED"
	MsgPasswordResetPINSent         MessageCode = "PASSWORD_RESET_PIN_SENT"
	MsgPasswordResetPINVerified     MessageCode = "PASSWORD_RESET_PIN_VERIFIED"
	MsgPasswordResetCompleted       MessageCode = "PASSWORD_RESET_COMPLETED"
	MsgPasswordChanged              MessageCode = "PASSWORD_CHANGED"
	MsgPatientProfileCompleted      MessageCode = "PATIENT_PROFILE_COMPLETED"
	MsgLogoutCompleted              MessageCode = "LOGOUT_COMPLETED"
	MsgAllSessionsLoggedOut         MessageCode = "ALL_SESSIONS_LOGGED_OUT"

	MsgPatientProfileUpdated  MessageCode = "PATIENT_PROFILE_UPDATED"
	MsgDoctorProfileUpdated   MessageCode = "DOCTOR_PROFILE_UPDATED"
	MsgUserProfileRetrieved   MessageCode = "USER_PROFILE_RETRIEVED"
	MsgTenantProfileRetrieved MessageCode = "TENANT_PROFILE_RETRIEVED"

	MsgHospitalCreated      MessageCode = "HOSPITAL_CREATED"
	MsgHospitalAdminCreated MessageCode = "HOSPITAL_ADMIN_CREATED"
	MsgHospitalStaffCreated MessageCode = "HOSPITAL_STAFF_CREATED"

	MsgDoctorSearchCompleted              MessageCode = "DOCTOR_SEARCH_COMPLETED"
	MsgHospitalDepartmentCreated          MessageCode = "HOSPITAL_DEPARTMENT_CREATED"
	MsgHospitalDepartmentsListed          MessageCode = "HOSPITAL_DEPARTMENTS_LISTED"
	MsgHospitalRoomCreated                MessageCode = "HOSPITAL_ROOM_CREATED"
	MsgHospitalRoomsListed                MessageCode = "HOSPITAL_ROOMS_LISTED"
	MsgDoctorInvitationCreated            MessageCode = "DOCTOR_INVITATION_CREATED"
	MsgHospitalDoctorInvitationsListed    MessageCode = "HOSPITAL_DOCTOR_INVITATIONS_LISTED"
	MsgHospitalDoctorInvitationRetrieved  MessageCode = "HOSPITAL_DOCTOR_INVITATION_RETRIEVED"
	MsgDoctorInvitationCancelled          MessageCode = "DOCTOR_INVITATION_CANCELLED"
	MsgDoctorInvitationResent             MessageCode = "DOCTOR_INVITATION_RESENT"
	MsgHospitalDoctorContractURLRetrieved MessageCode = "HOSPITAL_DOCTOR_CONTRACT_URL_RETRIEVED"
	MsgHospitalDoctorsListed              MessageCode = "HOSPITAL_DOCTORS_LISTED"
	MsgDoctorAffiliationsListed           MessageCode = "DOCTOR_AFFILIATIONS_LISTED"
	MsgDoctorAffiliationStatusUpdated     MessageCode = "DOCTOR_AFFILIATION_STATUS_UPDATED"
	MsgDoctorInvitationsListed            MessageCode = "DOCTOR_INVITATIONS_LISTED"
	MsgDoctorInvitationRetrieved          MessageCode = "DOCTOR_INVITATION_RETRIEVED"
	MsgDoctorContractURLRetrieved         MessageCode = "DOCTOR_CONTRACT_URL_RETRIEVED"
	MsgDoctorInvitationAccepted           MessageCode = "DOCTOR_INVITATION_ACCEPTED"
	MsgDoctorInvitationRejected           MessageCode = "DOCTOR_INVITATION_REJECTED"
	MsgNotificationsListed                MessageCode = "NOTIFICATIONS_LISTED"
	MsgNotificationMarkedRead             MessageCode = "NOTIFICATION_MARKED_READ"

	MsgAppointmentAvailabilityListed         MessageCode = "APPOINTMENT_AVAILABILITY_LISTED"
	MsgAppointmentCreated                    MessageCode = "APPOINTMENT_CREATED"
	MsgAppointmentCreationReplayed           MessageCode = "APPOINTMENT_CREATION_REPLAYED"
	MsgPatientAppointmentsListed             MessageCode = "PATIENT_APPOINTMENTS_LISTED"
	MsgPatientAppointmentRetrieved           MessageCode = "PATIENT_APPOINTMENT_RETRIEVED"
	MsgPatientAppointmentCancelled           MessageCode = "PATIENT_APPOINTMENT_CANCELLED"
	MsgPatientAppointmentRescheduled         MessageCode = "PATIENT_APPOINTMENT_RESCHEDULED"
	MsgPatientAppointmentRescheduleReplayed  MessageCode = "PATIENT_APPOINTMENT_RESCHEDULE_REPLAYED"
	MsgDoctorAppointmentsListed              MessageCode = "DOCTOR_APPOINTMENTS_LISTED"
	MsgDoctorAppointmentRetrieved            MessageCode = "DOCTOR_APPOINTMENT_RETRIEVED"
	MsgConsultationStarted                   MessageCode = "CONSULTATION_STARTED"
	MsgAppointmentCompleted                  MessageCode = "APPOINTMENT_COMPLETED"
	MsgDoctorScheduleChangeCreated           MessageCode = "DOCTOR_SCHEDULE_CHANGE_CREATED"
	MsgDoctorScheduleChangesListed           MessageCode = "DOCTOR_SCHEDULE_CHANGES_LISTED"
	MsgDoctorScheduleChangeApproved          MessageCode = "DOCTOR_SCHEDULE_CHANGE_APPROVED"
	MsgDoctorScheduleChangeRejected          MessageCode = "DOCTOR_SCHEDULE_CHANGE_REJECTED"
	MsgHospitalAppointmentsListed            MessageCode = "HOSPITAL_APPOINTMENTS_LISTED"
	MsgHospitalAppointmentRetrieved          MessageCode = "HOSPITAL_APPOINTMENT_RETRIEVED"
	MsgHospitalAppointmentCancelled          MessageCode = "HOSPITAL_APPOINTMENT_CANCELLED"
	MsgHospitalAppointmentRescheduled        MessageCode = "HOSPITAL_APPOINTMENT_RESCHEDULED"
	MsgHospitalAppointmentRescheduleReplayed MessageCode = "HOSPITAL_APPOINTMENT_RESCHEDULE_REPLAYED"
	MsgAppointmentCheckedIn                  MessageCode = "APPOINTMENT_CHECKED_IN"
	MsgAppointmentCheckInLookupCompleted     MessageCode = "APPOINTMENT_CHECK_IN_LOOKUP_COMPLETED"
	MsgWalkInAppointmentCreated              MessageCode = "WALK_IN_APPOINTMENT_CREATED"
	MsgWalkInAppointmentCreationReplayed     MessageCode = "WALK_IN_APPOINTMENT_CREATION_REPLAYED"
	MsgPatientRecordClaimed                  MessageCode = "PATIENT_RECORD_CLAIMED"
	MsgHospitalAppointmentQueueListed        MessageCode = "HOSPITAL_APPOINTMENT_QUEUE_LISTED"
	MsgAppointmentVitalsCompleted            MessageCode = "APPOINTMENT_VITALS_COMPLETED"
	MsgHospitalScheduleChangeCreated         MessageCode = "HOSPITAL_SCHEDULE_CHANGE_CREATED"
	MsgHospitalScheduleChangesListed         MessageCode = "HOSPITAL_SCHEDULE_CHANGES_LISTED"
	MsgHospitalScheduleChangeApproved        MessageCode = "HOSPITAL_SCHEDULE_CHANGE_APPROVED"
	MsgHospitalScheduleChangeRejected        MessageCode = "HOSPITAL_SCHEDULE_CHANGE_REJECTED"

	MsgExaminationRetrieved          MessageCode = "EXAMINATION_RETRIEVED"
	MsgVitalsDraftSaved              MessageCode = "VITALS_DRAFT_SAVED"
	MsgVitalsFinalized               MessageCode = "VITALS_FINALIZED"
	MsgVitalsCorrected               MessageCode = "VITALS_CORRECTED"
	MsgConsultationDraftSaved        MessageCode = "CONSULTATION_DRAFT_SAVED"
	MsgExaminationCompleted          MessageCode = "EXAMINATION_COMPLETED"
	MsgConsultationCorrected         MessageCode = "CONSULTATION_CORRECTED"
	MsgMedicalHistoryListed          MessageCode = "MEDICAL_HISTORY_LISTED"
	MsgMedicalRecordRetrieved        MessageCode = "MEDICAL_RECORD_RETRIEVED"
	MsgMedicalAttachmentUploaded     MessageCode = "MEDICAL_ATTACHMENT_UPLOADED"
	MsgMedicalAttachmentURLRetrieved MessageCode = "MEDICAL_ATTACHMENT_URL_RETRIEVED"
)

func successDetail(titleEng, titleIdn string) response.MessageDetail {
	return response.MessageDetail{
		TitleEng: titleEng,
		DescEng:  titleEng + ".",
		TitleIdn: titleIdn,
		DescIdn:  titleIdn + ".",
	}
}

var MessageCatalog = map[MessageCode]response.MessageDetail{
	MsgSuccess:                               successDetail("Operation completed successfully", "Operasi berhasil diselesaikan"),
	MsgServiceHealthy:                        successDetail("Service is healthy", "Layanan dalam kondisi sehat"),
	MsgServiceAlive:                          successDetail("Service is alive", "Layanan aktif"),
	MsgServiceReady:                          successDetail("Service is ready", "Layanan siap menerima permintaan"),
	MsgRegistrationChallengeCreated:          successDetail("Registration challenge created", "Tantangan registrasi berhasil dibuat"),
	MsgRegistrationPINResent:                 successDetail("Registration PIN resent", "PIN registrasi berhasil dikirim ulang"),
	MsgRegistrationVerified:                  successDetail("Registration verified", "Registrasi berhasil diverifikasi"),
	MsgPublicLoginSucceeded:                  successDetail("Public login succeeded", "Login aplikasi berhasil"),
	MsgHospitalLoginSucceeded:                successDetail("Hospital login succeeded", "Login rumah sakit berhasil"),
	MsgTokensRefreshed:                       successDetail("Authentication tokens refreshed", "Token autentikasi berhasil diperbarui"),
	MsgPatientRoleSelected:                   successDetail("Patient role selected", "Peran pasien berhasil dipilih"),
	MsgPasswordResetPINSent:                  successDetail("Password reset PIN sent", "PIN reset password berhasil dikirim"),
	MsgPasswordResetPINVerified:              successDetail("Password reset PIN verified", "PIN reset password berhasil diverifikasi"),
	MsgPasswordResetCompleted:                successDetail("Password reset completed", "Reset password berhasil diselesaikan"),
	MsgPasswordChanged:                       successDetail("Password changed", "Password berhasil diubah"),
	MsgPatientProfileCompleted:               successDetail("Patient profile completed", "Profil pasien berhasil dilengkapi"),
	MsgLogoutCompleted:                       successDetail("Logout completed", "Logout berhasil"),
	MsgAllSessionsLoggedOut:                  successDetail("All sessions logged out", "Semua sesi berhasil di-logout"),
	MsgPatientProfileUpdated:                 successDetail("Patient profile updated", "Profil pasien berhasil diperbarui"),
	MsgDoctorProfileUpdated:                  successDetail("Doctor profile updated", "Profil dokter berhasil diperbarui"),
	MsgUserProfileRetrieved:                  successDetail("User profile retrieved", "Profil pengguna berhasil diambil"),
	MsgTenantProfileRetrieved:                successDetail("Hospital-scoped profile retrieved", "Profil dalam lingkup rumah sakit berhasil diambil"),
	MsgHospitalCreated:                       successDetail("Hospital created", "Rumah sakit berhasil dibuat"),
	MsgHospitalAdminCreated:                  successDetail("Hospital administrator created", "Administrator rumah sakit berhasil dibuat"),
	MsgHospitalStaffCreated:                  successDetail("Hospital staff member created", "Staf rumah sakit berhasil dibuat"),
	MsgDoctorSearchCompleted:                 successDetail("Doctor search completed", "Pencarian dokter berhasil diselesaikan"),
	MsgHospitalDepartmentCreated:             successDetail("Hospital department created", "Departemen rumah sakit berhasil dibuat"),
	MsgHospitalDepartmentsListed:             successDetail("Hospital departments retrieved", "Daftar departemen rumah sakit berhasil diambil"),
	MsgHospitalRoomCreated:                   successDetail("Hospital room created", "Ruangan rumah sakit berhasil dibuat"),
	MsgHospitalRoomsListed:                   successDetail("Hospital rooms retrieved", "Daftar ruangan rumah sakit berhasil diambil"),
	MsgDoctorInvitationCreated:               successDetail("Doctor invitation created", "Undangan dokter berhasil dibuat"),
	MsgHospitalDoctorInvitationsListed:       successDetail("Hospital doctor invitations retrieved", "Daftar undangan dokter rumah sakit berhasil diambil"),
	MsgHospitalDoctorInvitationRetrieved:     successDetail("Hospital doctor invitation retrieved", "Undangan dokter rumah sakit berhasil diambil"),
	MsgDoctorInvitationCancelled:             successDetail("Doctor invitation cancelled", "Undangan dokter berhasil dibatalkan"),
	MsgDoctorInvitationResent:                successDetail("Doctor invitation resent", "Undangan dokter berhasil dikirim ulang"),
	MsgHospitalDoctorContractURLRetrieved:    successDetail("Hospital doctor contract URL retrieved", "URL kontrak dokter untuk rumah sakit berhasil diambil"),
	MsgHospitalDoctorsListed:                 successDetail("Hospital doctors retrieved", "Daftar dokter rumah sakit berhasil diambil"),
	MsgDoctorAffiliationsListed:              successDetail("Doctor affiliations retrieved", "Daftar afiliasi dokter berhasil diambil"),
	MsgDoctorAffiliationStatusUpdated:        successDetail("Doctor affiliation status updated", "Status afiliasi dokter berhasil diperbarui"),
	MsgDoctorInvitationsListed:               successDetail("Doctor invitations retrieved", "Daftar undangan dokter berhasil diambil"),
	MsgDoctorInvitationRetrieved:             successDetail("Doctor invitation retrieved", "Undangan dokter berhasil diambil"),
	MsgDoctorContractURLRetrieved:            successDetail("Doctor contract URL retrieved", "URL kontrak dokter berhasil diambil"),
	MsgDoctorInvitationAccepted:              successDetail("Doctor invitation accepted", "Undangan dokter berhasil diterima"),
	MsgDoctorInvitationRejected:              successDetail("Doctor invitation rejected", "Undangan dokter berhasil ditolak"),
	MsgNotificationsListed:                   successDetail("Notifications retrieved", "Daftar notifikasi berhasil diambil"),
	MsgNotificationMarkedRead:                successDetail("Notification marked as read", "Notifikasi berhasil ditandai sudah dibaca"),
	MsgAppointmentAvailabilityListed:         successDetail("Appointment availability retrieved", "Ketersediaan appointment berhasil diambil"),
	MsgAppointmentCreated:                    successDetail("Appointment created", "Appointment berhasil dibuat"),
	MsgAppointmentCreationReplayed:           successDetail("Existing appointment returned for idempotent retry", "Appointment yang sudah ada dikembalikan untuk percobaan idempoten"),
	MsgPatientAppointmentsListed:             successDetail("Patient appointments retrieved", "Daftar appointment pasien berhasil diambil"),
	MsgPatientAppointmentRetrieved:           successDetail("Patient appointment retrieved", "Appointment pasien berhasil diambil"),
	MsgPatientAppointmentCancelled:           successDetail("Patient appointment cancelled", "Appointment pasien berhasil dibatalkan"),
	MsgPatientAppointmentRescheduled:         successDetail("Patient appointment rescheduled", "Jadwal appointment pasien berhasil diubah"),
	MsgPatientAppointmentRescheduleReplayed:  successDetail("Existing rescheduled appointment returned for idempotent retry", "Perubahan jadwal appointment yang sudah ada dikembalikan untuk percobaan idempoten"),
	MsgDoctorAppointmentsListed:              successDetail("Doctor appointments retrieved", "Daftar appointment dokter berhasil diambil"),
	MsgDoctorAppointmentRetrieved:            successDetail("Doctor appointment retrieved", "Appointment dokter berhasil diambil"),
	MsgConsultationStarted:                   successDetail("Consultation started", "Konsultasi berhasil dimulai"),
	MsgAppointmentCompleted:                  successDetail("Appointment completed", "Appointment berhasil diselesaikan"),
	MsgDoctorScheduleChangeCreated:           successDetail("Doctor schedule change request created", "Permintaan perubahan jadwal dokter berhasil dibuat"),
	MsgDoctorScheduleChangesListed:           successDetail("Doctor schedule change requests retrieved", "Daftar permintaan perubahan jadwal dokter berhasil diambil"),
	MsgDoctorScheduleChangeApproved:          successDetail("Doctor schedule change request approved", "Permintaan perubahan jadwal dokter berhasil disetujui"),
	MsgDoctorScheduleChangeRejected:          successDetail("Doctor schedule change request rejected", "Permintaan perubahan jadwal dokter berhasil ditolak"),
	MsgHospitalAppointmentsListed:            successDetail("Hospital appointments retrieved", "Daftar appointment rumah sakit berhasil diambil"),
	MsgHospitalAppointmentRetrieved:          successDetail("Hospital appointment retrieved", "Appointment rumah sakit berhasil diambil"),
	MsgHospitalAppointmentCancelled:          successDetail("Hospital appointment cancelled", "Appointment rumah sakit berhasil dibatalkan"),
	MsgHospitalAppointmentRescheduled:        successDetail("Hospital appointment rescheduled", "Jadwal appointment rumah sakit berhasil diubah"),
	MsgHospitalAppointmentRescheduleReplayed: successDetail("Existing hospital reschedule returned for idempotent retry", "Perubahan jadwal oleh rumah sakit yang sudah ada dikembalikan untuk percobaan idempoten"),
	MsgAppointmentCheckedIn:                  successDetail("Appointment check-in completed", "Check-in appointment berhasil diselesaikan"),
	MsgAppointmentCheckInLookupCompleted:     successDetail("Appointment check-in lookup completed", "Pencarian check-in appointment berhasil diselesaikan"),
	MsgWalkInAppointmentCreated:              successDetail("Walk-in appointment created and checked in", "Appointment walk-in berhasil dibuat dan di-check-in"),
	MsgWalkInAppointmentCreationReplayed:     successDetail("Existing walk-in appointment returned for idempotent retry", "Appointment walk-in yang sudah ada dikembalikan untuk percobaan idempoten"),
	MsgPatientRecordClaimed:                  successDetail("Walk-in patient record claimed", "Patient record walk-in berhasil diklaim"),
	MsgHospitalAppointmentQueueListed:        successDetail("Hospital appointment queue retrieved", "Antrean appointment rumah sakit berhasil diambil"),
	MsgAppointmentVitalsCompleted:            successDetail("Appointment vitals completed", "Pemeriksaan tanda vital appointment berhasil diselesaikan"),
	MsgHospitalScheduleChangeCreated:         successDetail("Hospital schedule change request created", "Permintaan perubahan jadwal rumah sakit berhasil dibuat"),
	MsgHospitalScheduleChangesListed:         successDetail("Hospital schedule change requests retrieved", "Daftar permintaan perubahan jadwal rumah sakit berhasil diambil"),
	MsgHospitalScheduleChangeApproved:        successDetail("Hospital schedule change request approved", "Permintaan perubahan jadwal rumah sakit berhasil disetujui"),
	MsgHospitalScheduleChangeRejected:        successDetail("Hospital schedule change request rejected", "Permintaan perubahan jadwal rumah sakit berhasil ditolak"),
	MsgExaminationRetrieved:                  successDetail("Examination record retrieved", "Data pemeriksaan berhasil diambil"),
	MsgVitalsDraftSaved:                      successDetail("Vital signs draft saved", "Draft tanda vital berhasil disimpan"),
	MsgVitalsFinalized:                       successDetail("Vital signs finalized", "Tanda vital berhasil difinalisasi"),
	MsgVitalsCorrected:                       successDetail("Vital signs correction recorded", "Koreksi tanda vital berhasil dicatat"),
	MsgConsultationDraftSaved:                successDetail("Consultation draft saved", "Draft konsultasi berhasil disimpan"),
	MsgExaminationCompleted:                  successDetail("Examination completed", "Pemeriksaan berhasil diselesaikan"),
	MsgConsultationCorrected:                 successDetail("Consultation correction recorded", "Koreksi konsultasi berhasil dicatat"),
	MsgMedicalHistoryListed:                  successDetail("Medical history retrieved", "Riwayat pemeriksaan berhasil diambil"),
	MsgMedicalRecordRetrieved:                successDetail("Medical record retrieved", "Rekam medis berhasil diambil"),
	MsgMedicalAttachmentUploaded:             successDetail("Medical attachment uploaded", "Lampiran medis berhasil diunggah"),
	MsgMedicalAttachmentURLRetrieved:         successDetail("Medical attachment URL retrieved", "URL lampiran medis berhasil diambil"),
}

func GetMessageDetail(code MessageCode) response.MessageDetail {
	return MessageCatalog[code]
}

func NewSuccessResponse(code MessageCode) *response.BaseResponse {
	detail, ok := MessageCatalog[code]
	if !ok {
		code = MsgSuccess
		detail = MessageCatalog[code]
	}
	return &response.BaseResponse{Message: string(code), MessageDetail: detail}
}
