package constant

// ==========================
// PERMISSION SLUGS — MedikaOne
// ==========================

// User & Role & Permission (admin area)
const (
	PermissionUserView   = "user.view"
	PermissionUserCreate = "user.create"
	PermissionUserUpdate = "user.update"
	PermissionUserDelete = "user.delete"

	PermissionRoleView   = "role.view"
	PermissionRoleAssign = "role.assign" // assign role ke user

	PermissionPermissionView = "permission.view"
)

// Patient / Doctor profile
const (
	PermissionPatientView = "patient.view"
	PermissionPatientEdit = "patient.edit"

	PermissionDoctorView = "doctor.view"
	PermissionDoctorEdit = "doctor.edit"
)

// EMR & Billing
const (
	PermissionEMRView = "emr.view"
	PermissionEMREdit = "emr.edit"

	PermissionBillingView = "billing.view"
	PermissionBillingEdit = "billing.edit"
)

// Appointment
const (
	PermissionAppointmentView       = "appointment.view"
	PermissionAppointmentEdit       = "appointment.edit"
	PermissionAppointmentCreate     = "appointment.create"
	PermissionAppointmentCancel     = "appointment.cancel"
	PermissionAppointmentReschedule = "appointment.reschedule"
	PermissionAppointmentCheckIn    = "appointment.checkin"
	PermissionAppointmentQueue      = "appointment.queue"
	PermissionAppointmentComplete   = "appointment.complete"

	PermissionDoctorScheduleView    = "doctor_schedule.view"
	PermissionDoctorSchedulePropose = "doctor_schedule.propose"
	PermissionDoctorScheduleApprove = "doctor_schedule.approve"
)

// ==========================
// DEFAULT ROLE → PERMISSIONS
// ==========================
//
// Catatan:
// - super_admin mendapat SEMUA permission
// - admin (rumah sakit) = manajemen RS
// - nurse & receptionist hak operasional
// - bod pengawasan & laporan (read mostly)
// - patient & doctor minimal yang diperlukan
var DefaultRolePermissions = map[string][]string{
	// super admin: semua
	RoleSuperAdmin: {
		PermissionUserView, PermissionUserCreate, PermissionUserUpdate, PermissionUserDelete,
		PermissionRoleView, PermissionRoleAssign, PermissionPermissionView,
		PermissionPatientView, PermissionPatientEdit,
		PermissionDoctorView, PermissionDoctorEdit,
		PermissionEMRView, PermissionEMREdit,
		PermissionBillingView, PermissionBillingEdit,
		PermissionAppointmentView, PermissionAppointmentEdit, PermissionAppointmentCreate,
		PermissionAppointmentCancel, PermissionAppointmentReschedule, PermissionAppointmentCheckIn,
		PermissionAppointmentQueue, PermissionAppointmentComplete,
		PermissionDoctorScheduleView, PermissionDoctorSchedulePropose, PermissionDoctorScheduleApprove,
	},

	// admin RS
	RoleAdmin: {
		PermissionUserView, PermissionUserCreate, PermissionUserUpdate,
		PermissionRoleView, PermissionRoleAssign, PermissionPermissionView,
		PermissionPatientView, PermissionPatientEdit,
		PermissionDoctorView, PermissionDoctorEdit,
		PermissionEMRView, PermissionEMREdit,
		PermissionBillingView, PermissionBillingEdit,
		PermissionAppointmentView, PermissionAppointmentEdit, PermissionAppointmentCreate,
		PermissionAppointmentCancel, PermissionAppointmentReschedule, PermissionAppointmentCheckIn,
		PermissionAppointmentQueue, PermissionAppointmentComplete,
		PermissionDoctorScheduleView, PermissionDoctorSchedulePropose, PermissionDoctorScheduleApprove,
	},

	// nurse
	RoleNurse: {
		PermissionPatientView, PermissionPatientEdit,
		PermissionEMRView, PermissionEMREdit,
		PermissionBillingView,
		PermissionAppointmentView, PermissionAppointmentEdit,
		PermissionAppointmentQueue, PermissionDoctorScheduleView,
	},

	// receptionist
	RoleReceptionist: {
		PermissionAppointmentView, PermissionAppointmentEdit,
		PermissionAppointmentCancel, PermissionAppointmentReschedule,
		PermissionAppointmentCheckIn, PermissionAppointmentQueue,
		PermissionDoctorScheduleView,
		PermissionPatientView,
		PermissionBillingView, PermissionBillingEdit,
	},

	// BOD (direksi)
	RoleBOD: {
		PermissionBillingView,
		PermissionPermissionView, PermissionRoleView,
		PermissionDoctorView, PermissionPatientView,
		PermissionAppointmentView,
	},

	// patient (minimum untuk update profil)
	RolePatient: {
		PermissionAppointmentView, PermissionAppointmentCreate,
		PermissionAppointmentCancel, PermissionAppointmentReschedule,
		PermissionPatientView, PermissionPatientEdit,
	},

	// doctor (minimum untuk update profil)
	RoleDoctor: {
		PermissionAppointmentView, PermissionAppointmentComplete,
		PermissionDoctorScheduleView, PermissionDoctorSchedulePropose, PermissionDoctorScheduleApprove,
		PermissionDoctorView, PermissionDoctorEdit,
		PermissionEMRView,
	},
}
