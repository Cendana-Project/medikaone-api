package http

import (
	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"

	"github.com/Cendana-Project/medikaone-api/internal/config"
	"github.com/Cendana-Project/medikaone-api/internal/constant"
	appointmentCtrl "github.com/Cendana-Project/medikaone-api/internal/transport/http/appointment"
	authCtrl "github.com/Cendana-Project/medikaone-api/internal/transport/http/auth"
	doctorHospitalCtrl "github.com/Cendana-Project/medikaone-api/internal/transport/http/doctor_hospital"
	examinationCtrl "github.com/Cendana-Project/medikaone-api/internal/transport/http/examination"
	hospCtrl "github.com/Cendana-Project/medikaone-api/internal/transport/http/hospital"
	prescriptionCtrl "github.com/Cendana-Project/medikaone-api/internal/transport/http/prescription"
	userCtrl "github.com/Cendana-Project/medikaone-api/internal/transport/http/user"
	warmupCtrl "github.com/Cendana-Project/medikaone-api/internal/transport/http/warmup"
	transportmw "github.com/Cendana-Project/medikaone-api/internal/transport/middleware"
	"github.com/Cendana-Project/medikaone-api/internal/util"

	hospRepo "github.com/Cendana-Project/medikaone-api/internal/repository/hospital"
	roleRepo "github.com/Cendana-Project/medikaone-api/internal/repository/role"
	userRepo "github.com/Cendana-Project/medikaone-api/internal/repository/user"
)

type Transport struct {
	router                   *gin.Engine
	authController           *authCtrl.Controller
	userController           *userCtrl.Controller
	hospitalController       *hospCtrl.Controller
	doctorHospitalController *doctorHospitalCtrl.Controller
	appointmentController    *appointmentCtrl.Controller
	examinationController    *examinationCtrl.Controller
	prescriptionController   *prescriptionCtrl.Controller
	warmupController         *warmupCtrl.Controller

	roleRepo *roleRepo.Repository
	hospRepo *hospRepo.Repository
	userRepo *userRepo.Repository
	rdb      *redis.Client
}

func NewTransport() *Transport                              { return new(Transport) }
func (t *Transport) WithGinEngine(r *gin.Engine) *Transport { t.router = r; return t }
func (t *Transport) WithAuthController(c *authCtrl.Controller) *Transport {
	t.authController = c
	return t
}
func (t *Transport) WithUserController(c *userCtrl.Controller) *Transport {
	t.userController = c
	return t
}
func (t *Transport) WithHospitalController(c *hospCtrl.Controller) *Transport {
	t.hospitalController = c
	return t
}
func (t *Transport) WithDoctorHospitalController(c *doctorHospitalCtrl.Controller) *Transport {
	t.doctorHospitalController = c
	return t
}
func (t *Transport) WithAppointmentController(c *appointmentCtrl.Controller) *Transport {
	t.appointmentController = c
	return t
}
func (t *Transport) WithExaminationController(c *examinationCtrl.Controller) *Transport {
	t.examinationController = c
	return t
}
func (t *Transport) WithPrescriptionController(c *prescriptionCtrl.Controller) *Transport {
	t.prescriptionController = c
	return t
}
func (t *Transport) WithRoleRepository(repo *roleRepo.Repository) *Transport {
	t.roleRepo = repo
	return t
}
func (t *Transport) WithHospitalRepository(repo *hospRepo.Repository) *Transport {
	t.hospRepo = repo
	return t
}
func (t *Transport) WithUserRepository(repo *userRepo.Repository) *Transport {
	t.userRepo = repo
	return t
}
func (t *Transport) WithRedisClient(rdb *redis.Client) *Transport {
	t.rdb = rdb
	return t
}
func (t *Transport) WithWarmupController(c *warmupCtrl.Controller) *Transport {
	t.warmupController = c
	return t
}

func (t *Transport) InitRoute() {
	if t.router == nil {
		panic("gin engine is nil")
	}
	t.router.Use(transportmw.RejectDuringMaintenance(t.rdb))
	// ========== WARMUP — PUBLIC ==========
	t.router.GET("/ping", func(c *gin.Context) { t.warmupController.Ping(c) })

	v1 := t.router.Group("/v1")
	v1.GET("/prescriptions/verify/:token",
		transportmw.RateLimitPublicPrescriptionVerificationByIP(
			t.rdb, config.Env.Auth.PublicIPRateLimit, config.Env.Auth.PublicIPRateWindow,
			config.Env.Server.ClientIPHeader,
		),
		t.prescriptionController.Verify,
	)

	// ========== AUTH — PUBLIC ==========
	auth := v1.Group("/auth")
	auth.Use(transportmw.RateLimitPublicAuthByIP(
		t.rdb,
		config.Env.Auth.PublicIPRateLimit,
		config.Env.Auth.PublicIPRateWindow,
		config.Env.Server.ClientIPHeader,
	))
	{
		auth.POST("/register", t.authController.Register)
		auth.POST("/resend-pin", t.authController.ResendPIN)
		auth.POST("/verify-pin", t.authController.VerifyPIN)

		auth.POST("/login", t.authController.LoginPublic)
		auth.POST("/login/hospital", t.authController.LoginHospital)

		auth.POST("/refresh", t.authController.Refresh)
		auth.POST("/password/forgot", t.authController.PasswordForgot)
		auth.POST("/password/verify-pin", t.authController.PasswordResetVerifyPIN)
		auth.POST("/password/reset", t.authController.PasswordReset)
	}

	// === PROTECTED — USER-LEVEL ===
	protected := v1.Group("/")
	protected.Use(transportmw.AuthRequired(t.rdb, t.userRepo))
	{
		protected.GET("/me", t.userController.Me)
		protected.GET("/profile", t.userController.Profile)
		protected.PATCH("/profile", t.userController.UpdateProfile)
		protected.PUT("/profile/photo", t.userController.UploadProfilePhoto)
		protected.GET("/profile/photo", t.userController.GetProfilePhotoURL)
		protected.DELETE("/profile/photo", t.userController.DeleteProfilePhoto)

		protected.POST("/auth/choose-role", t.authController.ChooseRole)

		protected.PUT("/profile/patient",
			transportmw.RequirePermissions(t.roleRepo, constant.PermissionPatientEdit),
			t.userController.UpdatePatientProfile,
		)

		protected.PUT("/auth/password", t.authController.PasswordChange)

		protected.PUT("/profile/doctor",
			transportmw.RequireDoctor(t.roleRepo),
			t.userController.UpdateDoctorProfile,
		)

		protected.POST("/hospitals",
			transportmw.RequireSuperAdmin(t.roleRepo),
			t.hospitalController.CreateHospital,
		)

		protected.POST("/auth/set-profile", t.authController.SetProfile) // endpoint gabungan

		// === NEW: Logout endpoints ===
		protected.POST("/auth/logout", t.authController.Logout)
		protected.POST("/auth/logout-all", t.authController.LogoutAll)

		protected.GET("/doctor/hospital-invitations",
			transportmw.RequirePermissions(t.roleRepo, constant.PermissionDoctorView),
			t.doctorHospitalController.ListDoctorInvitations,
		)
		protected.GET("/doctor/hospital-invitations/:invitation_id",
			transportmw.RequirePermissions(t.roleRepo, constant.PermissionDoctorView),
			t.doctorHospitalController.GetDoctorInvitation,
		)
		protected.GET("/doctor/hospital-invitations/:invitation_id/contract",
			transportmw.RequirePermissions(t.roleRepo, constant.PermissionDoctorView),
			t.doctorHospitalController.GetDoctorContractURL,
		)
		protected.POST("/doctor/hospital-invitations/:invitation_id/accept",
			transportmw.RequirePermissions(t.roleRepo, constant.PermissionDoctorEdit),
			t.doctorHospitalController.AcceptInvitation,
		)
		protected.POST("/doctor/hospital-invitations/:invitation_id/reject",
			transportmw.RequirePermissions(t.roleRepo, constant.PermissionDoctorEdit),
			t.doctorHospitalController.RejectInvitation,
		)
		protected.GET("/doctor/hospital-affiliations",
			transportmw.RequirePermissions(t.roleRepo, constant.PermissionDoctorScheduleView),
			t.doctorHospitalController.ListDoctorAffiliations,
		)

		protected.GET("/notifications", t.doctorHospitalController.ListNotifications)
		protected.PATCH("/notifications/:notification_id/read", t.doctorHospitalController.MarkNotificationRead)

		protected.GET("/appointments/availability",
			transportmw.RequirePermissions(t.roleRepo, constant.PermissionAppointmentView),
			t.appointmentController.ListAvailability,
		)
		protected.POST("/appointments",
			transportmw.RequirePermissions(t.roleRepo, constant.PermissionAppointmentCreate),
			t.appointmentController.CreateAppointment,
		)
		protected.GET("/appointments",
			transportmw.RequirePermissions(t.roleRepo, constant.PermissionAppointmentView),
			t.appointmentController.ListPatientAppointments,
		)
		protected.GET("/appointments/:appointment_id",
			transportmw.RequirePermissions(t.roleRepo, constant.PermissionAppointmentView),
			t.appointmentController.GetPatientAppointment,
		)
		protected.POST("/appointments/:appointment_id/cancel",
			transportmw.RequirePermissions(t.roleRepo, constant.PermissionAppointmentCancel),
			t.appointmentController.CancelPatientAppointment,
		)
		protected.POST("/appointments/:appointment_id/reschedule",
			transportmw.RequirePermissions(t.roleRepo, constant.PermissionAppointmentReschedule),
			t.appointmentController.ReschedulePatientAppointment,
		)
		protected.POST("/patient-records/claim",
			transportmw.RequirePermissions(t.roleRepo, constant.PermissionPatientRecordClaim),
			t.appointmentController.ClaimPatientRecord,
		)

		protected.GET("/doctor/appointments",
			transportmw.RequirePermissions(t.roleRepo, constant.PermissionAppointmentView),
			t.appointmentController.ListDoctorAppointments,
		)
		protected.GET("/doctor/appointments/:appointment_id",
			transportmw.RequirePermissions(t.roleRepo, constant.PermissionAppointmentView),
			t.appointmentController.GetDoctorAppointment,
		)
		protected.POST("/doctor/appointments/:appointment_id/start-consultation",
			transportmw.RequirePermissions(t.roleRepo, constant.PermissionAppointmentComplete),
			t.appointmentController.StartConsultation,
		)
		protected.POST("/doctor/appointments/:appointment_id/complete",
			transportmw.RequirePermissions(t.roleRepo, constant.PermissionExaminationConsultationWrite),
			t.examinationController.CompleteDoctorExamination,
		)
		protected.GET("/doctor/appointments/:appointment_id/examination",
			transportmw.RequirePermissions(t.roleRepo, constant.PermissionExaminationView),
			t.examinationController.GetDoctorExamination,
		)
		protected.GET("/doctor/appointments/:appointment_id/medications",
			transportmw.RequirePermissions(t.roleRepo, constant.PermissionPrescriptionView),
			t.prescriptionController.ListDoctorMedications,
		)
		protected.GET("/doctor/appointments/:appointment_id/prescription",
			transportmw.RequirePermissions(t.roleRepo, constant.PermissionPrescriptionView),
			t.prescriptionController.GetDoctorPrescription,
		)
		protected.PUT("/doctor/appointments/:appointment_id/prescription",
			transportmw.RequirePermissions(t.roleRepo, constant.PermissionPrescriptionWrite),
			t.prescriptionController.SaveDraft,
		)
		protected.POST("/doctor/appointments/:appointment_id/prescription/no-medication",
			transportmw.RequirePermissions(t.roleRepo, constant.PermissionPrescriptionWrite),
			t.prescriptionController.MarkNoMedication,
		)
		protected.POST("/doctor/appointments/:appointment_id/prescription/issue",
			transportmw.RequirePermissions(t.roleRepo, constant.PermissionPrescriptionWrite),
			t.prescriptionController.Issue,
		)
		protected.POST("/doctor/appointments/:appointment_id/prescription/corrections",
			transportmw.RequirePermissions(t.roleRepo, constant.PermissionPrescriptionCorrect),
			t.prescriptionController.Correct,
		)
		protected.POST("/doctor/appointments/:appointment_id/prescription/cancel",
			transportmw.RequirePermissions(t.roleRepo, constant.PermissionPrescriptionCorrect),
			t.prescriptionController.Cancel,
		)
		protected.GET("/doctor/appointments/:appointment_id/prescription/document-url",
			transportmw.RequirePermissions(t.roleRepo, constant.PermissionPrescriptionPrint),
			t.prescriptionController.GetDoctorDocumentURL,
		)
		protected.GET("/doctor/appointments/:appointment_id/medical-history",
			transportmw.RequirePermissions(t.roleRepo, constant.PermissionExaminationView),
			t.examinationController.ListDoctorMedicalHistory,
		)
		protected.GET("/doctor/appointments/:appointment_id/medical-history/:encounter_id",
			transportmw.RequirePermissions(t.roleRepo, constant.PermissionExaminationView),
			t.examinationController.GetDoctorHistoricalMedicalRecord,
		)
		protected.GET("/doctor/appointments/:appointment_id/medical-history/:encounter_id/attachments/:attachment_id/url",
			transportmw.RequirePermissions(t.roleRepo, constant.PermissionExaminationView),
			t.examinationController.GetDoctorHistoricalAttachmentURL,
		)
		protected.PUT("/doctor/appointments/:appointment_id/examination/consultation",
			transportmw.RequirePermissions(t.roleRepo, constant.PermissionExaminationConsultationWrite),
			t.examinationController.SaveDoctorConsultationDraft,
		)
		protected.POST("/doctor/appointments/:appointment_id/examination/consultation/corrections",
			transportmw.RequirePermissions(t.roleRepo, constant.PermissionExaminationCorrect),
			t.examinationController.CorrectDoctorConsultation,
		)
		protected.POST("/doctor/appointments/:appointment_id/examination/attachments",
			transportmw.RequirePermissions(t.roleRepo, constant.PermissionExaminationAttachmentManage),
			t.examinationController.UploadDoctorAttachment,
		)
		protected.GET("/doctor/appointments/:appointment_id/examination/attachments/:attachment_id/url",
			transportmw.RequirePermissions(t.roleRepo, constant.PermissionExaminationView),
			t.examinationController.GetDoctorAttachmentURL,
		)
		protected.GET("/medical-records",
			transportmw.RequirePermissions(t.roleRepo, constant.PermissionMedicalRecordSelfView),
			t.examinationController.ListPatientMedicalHistory,
		)
		protected.GET("/medical-records/:encounter_id",
			transportmw.RequirePermissions(t.roleRepo, constant.PermissionMedicalRecordSelfView),
			t.examinationController.GetPatientMedicalRecord,
		)
		protected.GET("/medical-record-attachments/:attachment_id/url",
			transportmw.RequirePermissions(t.roleRepo, constant.PermissionMedicalRecordSelfView),
			t.examinationController.GetPatientAttachmentURL,
		)
		protected.GET("/prescriptions",
			transportmw.RequirePermissions(t.roleRepo, constant.PermissionPrescriptionSelfView),
			t.prescriptionController.ListPatientPrescriptions,
		)
		protected.GET("/prescriptions/:prescription_id",
			transportmw.RequirePermissions(t.roleRepo, constant.PermissionPrescriptionSelfView),
			t.prescriptionController.GetPatientPrescription,
		)
		protected.GET("/prescriptions/:prescription_id/document-url",
			transportmw.RequirePermissions(t.roleRepo, constant.PermissionPrescriptionSelfView),
			t.prescriptionController.GetPatientDocumentURL,
		)
		protected.POST("/doctor/schedule-change-requests",
			transportmw.RequirePermissions(t.roleRepo, constant.PermissionDoctorSchedulePropose),
			t.appointmentController.CreateDoctorScheduleChange,
		)
		protected.GET("/doctor/schedule-change-requests",
			transportmw.RequirePermissions(t.roleRepo, constant.PermissionDoctorScheduleView),
			t.appointmentController.ListDoctorScheduleChanges,
		)
		protected.POST("/doctor/schedule-change-requests/:change_id/approve",
			transportmw.RequirePermissions(t.roleRepo, constant.PermissionDoctorScheduleApprove),
			t.appointmentController.ApproveDoctorScheduleChange,
		)
		protected.POST("/doctor/schedule-change-requests/:change_id/reject",
			transportmw.RequirePermissions(t.roleRepo, constant.PermissionDoctorScheduleApprove),
			t.appointmentController.RejectDoctorScheduleChange,
		)
	}

	// === PROTECTED — HOSPITAL SCOPED (JWT + Tenant) ===
	tenant := v1.Group("/")
	tenant.Use(transportmw.AuthRequired(t.rdb, t.userRepo), transportmw.TenantContext())
	{
		tenant.POST("/hospitals/:hospital_id/admins",
			transportmw.RequireSuperAdmin(t.roleRepo),
			t.hospitalController.CreateHospitalAdmin,
		)

		tenant.POST("/hospitals/:hospital_id/staff",
			transportmw.RequireHospitalAdminOrSuper(t.hospRepo, t.roleRepo),
			t.hospitalController.CreateHospitalStaff,
		)
		tenant.GET("/tenant/me", t.userController.TenantMe)

		tenant.POST("/hospitals/:hospital_id/departments",
			transportmw.RequireHospitalAdminOrSuper(t.hospRepo, t.roleRepo),
			t.doctorHospitalController.CreateDepartment,
		)
		tenant.GET("/hospitals/:hospital_id/departments",
			transportmw.RequireHospitalAdminOrSuper(t.hospRepo, t.roleRepo),
			t.doctorHospitalController.ListDepartments,
		)
		tenant.POST("/hospitals/:hospital_id/rooms",
			transportmw.RequireHospitalAdminOrSuper(t.hospRepo, t.roleRepo),
			t.doctorHospitalController.CreateRoom,
		)
		tenant.GET("/hospitals/:hospital_id/rooms",
			transportmw.RequireHospitalAdminOrSuper(t.hospRepo, t.roleRepo),
			t.doctorHospitalController.ListRooms,
		)
		tenant.GET("/hospitals/:hospital_id/doctors/search",
			transportmw.RequireHospitalAdminOrSuper(t.hospRepo, t.roleRepo),
			t.doctorHospitalController.SearchDoctor,
		)
		tenant.POST("/hospitals/:hospital_id/doctor-invitations",
			transportmw.RequireHospitalAdminOrSuper(t.hospRepo, t.roleRepo),
			t.doctorHospitalController.CreateInvitation,
		)
		tenant.GET("/hospitals/:hospital_id/doctor-invitations",
			transportmw.RequireHospitalAdminOrSuper(t.hospRepo, t.roleRepo),
			t.doctorHospitalController.ListHospitalInvitations,
		)
		tenant.GET("/hospitals/:hospital_id/doctor-invitations/:invitation_id",
			transportmw.RequireHospitalAdminOrSuper(t.hospRepo, t.roleRepo),
			t.doctorHospitalController.GetHospitalInvitation,
		)
		tenant.GET("/hospitals/:hospital_id/doctor-invitations/:invitation_id/contract",
			transportmw.RequireHospitalAdminOrSuper(t.hospRepo, t.roleRepo),
			t.doctorHospitalController.GetHospitalContractURL,
		)
		tenant.POST("/hospitals/:hospital_id/doctor-invitations/:invitation_id/cancel",
			transportmw.RequireHospitalAdminOrSuper(t.hospRepo, t.roleRepo),
			t.doctorHospitalController.CancelInvitation,
		)
		tenant.POST("/hospitals/:hospital_id/doctor-invitations/:invitation_id/resend",
			transportmw.RequireHospitalAdminOrSuper(t.hospRepo, t.roleRepo),
			t.doctorHospitalController.ResendInvitation,
		)
		tenant.GET("/hospitals/:hospital_id/doctors",
			transportmw.RequireHospitalAdminOrSuper(t.hospRepo, t.roleRepo),
			t.doctorHospitalController.ListHospitalDoctors,
		)
		tenant.PATCH("/hospitals/:hospital_id/doctors/:doctor_id/status",
			transportmw.RequireHospitalAdminOrSuper(t.hospRepo, t.roleRepo),
			t.doctorHospitalController.UpdateAffiliationStatus,
		)

		tenant.GET("/hospitals/:hospital_id/appointments",
			transportmw.RequireHospitalPermissions(t.hospRepo, t.roleRepo, constant.PermissionAppointmentView),
			t.appointmentController.ListHospitalAppointments,
		)
		tenant.GET("/hospitals/:hospital_id/appointments/:appointment_id",
			transportmw.RequireHospitalPermissions(t.hospRepo, t.roleRepo, constant.PermissionAppointmentView),
			t.appointmentController.GetHospitalAppointment,
		)
		tenant.POST("/hospitals/:hospital_id/appointments/:appointment_id/cancel",
			transportmw.RequireHospitalPermissions(t.hospRepo, t.roleRepo, constant.PermissionAppointmentCancel),
			t.appointmentController.CancelHospitalAppointment,
		)
		tenant.POST("/hospitals/:hospital_id/appointments/:appointment_id/reschedule",
			transportmw.RequireHospitalPermissions(t.hospRepo, t.roleRepo, constant.PermissionAppointmentReschedule),
			t.appointmentController.RescheduleHospitalAppointment,
		)
		tenant.POST("/hospitals/:hospital_id/appointments/check-in",
			transportmw.RequireHospitalPermissions(t.hospRepo, t.roleRepo, constant.PermissionAppointmentCheckIn),
			t.appointmentController.CheckIn,
		)
		tenant.POST("/hospitals/:hospital_id/appointments/check-in/lookup",
			transportmw.RequireHospitalPermissions(t.hospRepo, t.roleRepo, constant.PermissionAppointmentCheckIn),
			t.appointmentController.LookupCheckIn,
		)
		tenant.POST("/hospitals/:hospital_id/appointments/:appointment_id/check-in",
			transportmw.RequireHospitalPermissions(t.hospRepo, t.roleRepo, constant.PermissionAppointmentCheckIn),
			t.appointmentController.ConfirmCheckIn,
		)
		tenant.POST("/hospitals/:hospital_id/walk-in-appointments",
			transportmw.RequireHospitalPermissions(t.hospRepo, t.roleRepo, constant.PermissionAppointmentWalkInCreate),
			t.appointmentController.CreateWalkInAppointment,
		)
		tenant.GET("/hospitals/:hospital_id/appointment-queue",
			transportmw.RequireHospitalPermissions(t.hospRepo, t.roleRepo, constant.PermissionAppointmentQueue),
			t.appointmentController.ListHospitalQueue,
		)
		tenant.POST("/hospitals/:hospital_id/appointments/:appointment_id/vitals-complete",
			transportmw.RequireHospitalPermissions(t.hospRepo, t.roleRepo, constant.PermissionExaminationVitalsWrite),
			t.examinationController.FinalizeVitals,
		)
		tenant.GET("/hospitals/:hospital_id/appointments/:appointment_id/examination",
			transportmw.RequireHospitalPermissions(t.hospRepo, t.roleRepo, constant.PermissionExaminationView),
			t.examinationController.GetHospitalExamination,
		)
		tenant.GET("/hospitals/:hospital_id/medications",
			transportmw.RequireHospitalPermissions(t.hospRepo, t.roleRepo, constant.PermissionPrescriptionView),
			t.prescriptionController.ListHospitalMedications,
		)
		tenant.POST("/hospitals/:hospital_id/medications",
			transportmw.RequireHospitalPermissions(t.hospRepo, t.roleRepo, constant.PermissionMedicationCatalogManage),
			t.prescriptionController.CreateMedication,
		)
		tenant.PUT("/hospitals/:hospital_id/medications/:medication_id",
			transportmw.RequireHospitalPermissions(t.hospRepo, t.roleRepo, constant.PermissionMedicationCatalogManage),
			t.prescriptionController.UpdateMedication,
		)
		tenant.GET("/hospitals/:hospital_id/appointments/:appointment_id/prescription",
			transportmw.RequireHospitalPermissions(t.hospRepo, t.roleRepo, constant.PermissionPrescriptionView),
			t.prescriptionController.GetHospitalPrescription,
		)
		tenant.GET("/hospitals/:hospital_id/appointments/:appointment_id/prescription/document-url",
			transportmw.RequireHospitalPermissions(t.hospRepo, t.roleRepo, constant.PermissionPrescriptionPrint),
			t.prescriptionController.GetHospitalDocumentURL,
		)
		tenant.PUT("/hospitals/:hospital_id/appointments/:appointment_id/examination/vitals",
			transportmw.RequireHospitalPermissions(t.hospRepo, t.roleRepo, constant.PermissionExaminationVitalsWrite),
			t.examinationController.SaveVitalDraft,
		)
		tenant.POST("/hospitals/:hospital_id/appointments/:appointment_id/examination/vitals/finalize",
			transportmw.RequireHospitalPermissions(t.hospRepo, t.roleRepo, constant.PermissionExaminationVitalsWrite),
			t.examinationController.FinalizeVitals,
		)
		tenant.POST("/hospitals/:hospital_id/appointments/:appointment_id/examination/vitals/corrections",
			transportmw.RequireHospitalPermissions(t.hospRepo, t.roleRepo, constant.PermissionExaminationCorrect),
			t.examinationController.CorrectVitals,
		)
		tenant.POST("/hospitals/:hospital_id/appointments/:appointment_id/examination/consultation/corrections",
			transportmw.RequireHospitalAdminOrSuper(t.hospRepo, t.roleRepo),
			t.examinationController.CorrectHospitalConsultation,
		)
		tenant.POST("/hospitals/:hospital_id/appointments/:appointment_id/examination/attachments",
			transportmw.RequireHospitalPermissions(t.hospRepo, t.roleRepo, constant.PermissionExaminationAttachmentManage),
			t.examinationController.UploadHospitalAttachment,
		)
		tenant.GET("/hospitals/:hospital_id/appointments/:appointment_id/examination/attachments/:attachment_id/url",
			transportmw.RequireHospitalPermissions(t.hospRepo, t.roleRepo, constant.PermissionExaminationView),
			t.examinationController.GetHospitalAttachmentURL,
		)
		tenant.POST("/hospitals/:hospital_id/schedule-change-requests",
			transportmw.RequireHospitalPermissions(t.hospRepo, t.roleRepo, constant.PermissionDoctorSchedulePropose),
			t.appointmentController.CreateHospitalScheduleChange,
		)
		tenant.GET("/hospitals/:hospital_id/schedule-change-requests",
			transportmw.RequireHospitalPermissions(t.hospRepo, t.roleRepo, constant.PermissionDoctorScheduleView),
			t.appointmentController.ListHospitalScheduleChanges,
		)
		tenant.POST("/hospitals/:hospital_id/schedule-change-requests/:change_id/approve",
			transportmw.RequireHospitalPermissions(t.hospRepo, t.roleRepo, constant.PermissionDoctorScheduleApprove),
			t.appointmentController.ApproveHospitalScheduleChange,
		)
		tenant.POST("/hospitals/:hospital_id/schedule-change-requests/:change_id/reject",
			transportmw.RequireHospitalPermissions(t.hospRepo, t.roleRepo, constant.PermissionDoctorScheduleApprove),
			t.appointmentController.RejectHospitalScheduleChange,
		)
	}

	// 404
	t.router.NoRoute(func(c *gin.Context) {
		resp := constant.ErrEndpointNotFound.ToResponse()
		util.HandleResponse(c, &resp, nil)
	})
}
