package constant

import (
	"net/http"

	"github.com/Cendana-Project/medikaone-api/internal/model/response"
)

// NOTE:
// - File ini menyatukan semua konstanta error yang digunakan oleh util, transport, dan service (auth).
// - Beberapa error punya pasangan "ValidationFailed" vs "ValidationError" agar kompatibel dengan util lama.
// - Detail pesan memakai GetMessageDetail dari message.constant.go. Jika suatu Msg* belum ada, hapus Detail-nya atau tambahkan di message.constant.go.

var (
	// ====== Generic / Common ======
	ErrInternalServerError = response.CustomError{
		Code:       "INTERNAL_SERVER_ERROR",
		StatusCode: http.StatusInternalServerError,
		Message:    "internal server error",
		Detail:     GetMessageDetail(MsgInternalServerError),
	}
	ErrValidationError = response.CustomError{
		Code:       "VALIDATION_ERROR",
		StatusCode: http.StatusBadRequest,
		Message:    "validation error",
		Detail:     GetMessageDetail(MsgValidationError),
	}
	// Alias gaya penamaan yang lain (dipakai di beberapa tempat)
	ErrValidationFailed = response.CustomError{
		Code:       "VALIDATION_FAILED",
		StatusCode: http.StatusBadRequest,
		Message:    "validation failed",
		Detail:     GetMessageDetail(MsgValidationFailed),
	}

	ErrEndpointNotFound = response.CustomError{
		Code:       "ENDPOINT_NOT_FOUND",
		StatusCode: http.StatusNotFound,
		Message:    "endpoint not found",
		Detail:     GetMessageDetail(MsgEndpointNotFound),
	}

	ErrUnauthorized = response.CustomError{
		Code:       "UNAUTHORIZED",
		StatusCode: http.StatusUnauthorized,
		Message:    "unauthorized",
		Detail:     GetMessageDetail(MsgUnauthorized),
	}
	ErrForbidden = response.CustomError{
		Code:       "FORBIDDEN",
		StatusCode: http.StatusForbidden,
		Message:    "forbidden",
		Detail:     GetMessageDetail(MsgForbidden),
	}
	ErrUserNotAuthenticated = response.CustomError{
		Code:       "USER_NOT_AUTHENTICATED",
		StatusCode: http.StatusUnauthorized,
		Message:    "user not authenticated",
		Detail:     GetMessageDetail(MsgUserNotAuthenticated),
	}

	ErrTooManyRequests = response.CustomError{
		Code:       "TOO_MANY_REQUESTS",
		StatusCode: http.StatusTooManyRequests,
		Message:    "too many requests, please try again later",
	}
	ErrRequestTooLarge = response.CustomError{
		Code:       "REQUEST_TOO_LARGE",
		StatusCode: http.StatusRequestEntityTooLarge,
		Message:    "request body is too large",
	}
	ErrServiceUnavailable = response.CustomError{
		Code:       "SERVICE_UNAVAILABLE",
		StatusCode: http.StatusServiceUnavailable,
		Message:    "service is temporarily unavailable",
	}

	// ====== Input / Format ======
	ErrInvalidEmail = response.CustomError{
		Code:       "INVALID_EMAIL",
		StatusCode: http.StatusBadRequest,
		Message:    "invalid email format",
	}
	ErrInvalidPassword = response.CustomError{
		Code:       "INVALID_PASSWORD",
		StatusCode: http.StatusBadRequest,
		Message:    "Password must meet all requirements: at least 8 characters, contain uppercase letter, lowercase letter, number, and special character. Must not contain sequential numbers (e.g. 1234) or repeated characters (e.g. aaaa).",
	}
	ErrPasswordSimilarToUserInfo = response.CustomError{
		Code:       "PASSWORD_SIMILAR_TO_USER_INFO",
		StatusCode: http.StatusBadRequest,
		Message:    "password cannot be similar to your username or email",
	}
	ErrInvalidDateFormat = response.CustomError{
		Code:       "INVALID_DATE_FORMAT",
		StatusCode: http.StatusBadRequest,
		Message:    "invalid date format, use YYYY-MM-DD (example: 1997-12-22)",
	}
	ErrInvalidUUIDFormat = response.CustomError{
		Code:       "INVALID_UUID_FORMAT",
		StatusCode: http.StatusBadRequest,
		Message:    "invalid UUID format",
		Detail:     GetMessageDetail(MsgInvalidUUIDFormat),
	}

	// ====== Resource / Data ======
	ErrRecordNotFound = response.CustomError{
		Code:       "RECORD_NOT_FOUND",
		StatusCode: http.StatusNotFound,
		Message:    "record not found",
		Detail:     GetMessageDetail(MsgRecordNotFound),
	}
	ErrUserNotFound = response.CustomError{
		Code:       "USER_NOT_FOUND",
		StatusCode: http.StatusNotFound,
		Message:    "user not found",
		Detail:     GetMessageDetail(MsgUserNotFound),
	}
	ErrDuplicateUsernameOrEmail = response.CustomError{
		Code:       "DUPLICATE_USERNAME_OR_EMAIL",
		StatusCode: http.StatusConflict,
		Message:    "Username or email already exists",
	}
	ErrDuplicateNIK = response.CustomError{
		Code:       "DUPLICATE_NIK",
		StatusCode: http.StatusConflict,
		Message:    "NIK already exists",
		Detail:     GetMessageDetail(MsgConflict),
	}
	ErrConflict = response.CustomError{
		Code:       "CONFLICT",
		StatusCode: http.StatusConflict,
		Message:    "conflict",
	}

	// ====== Auth / Token / OTP/PIN ======
	ErrInvalidToken = response.CustomError{
		Code:       "INVALID_TOKEN",
		StatusCode: http.StatusUnauthorized,
		Message:    "invalid token",
	}
	ErrInvalidCredentials = response.CustomError{
		Code:       "INVALID_CREDENTIALS",
		StatusCode: http.StatusUnauthorized,
		Message:    "invalid credentials",
	}
	ErrTokenExpired = response.CustomError{
		Code:       "TOKEN_EXPIRED",
		StatusCode: http.StatusUnauthorized,
		Message:    "token expired",
	}
	ErrInvalidOTP = response.CustomError{
		Code:       "INVALID_OTP",
		StatusCode: http.StatusBadRequest,
		Message:    "invalid or expired OTP",
	}
	ErrInvalidResetToken = response.CustomError{
		Code:       "INVALID_RESET_TOKEN",
		StatusCode: http.StatusBadRequest,
		Message:    "invalid or expired password reset token",
	}
	ErrEmailNotVerified = response.CustomError{
		Code:       "EMAIL_NOT_VERIFIED",
		StatusCode: http.StatusForbidden,
		Message:    "please verify your email before logging in",
	}

	// ====== Role / Permission ======
	ErrInvalidRoleID = response.CustomError{
		Code:       "INVALID_ROLE_ID",
		StatusCode: http.StatusBadRequest,
		Message:    "invalid role_id",
		Detail:     GetMessageDetail(MsgInvalidRoleID),
	}
	ErrRoleAlreadyExist = response.CustomError{
		Code:       "ROLE_ALREADY_EXIST",
		StatusCode: http.StatusConflict,
		Message:    "role already exists",
		Detail:     GetMessageDetail(MsgRoleAlreadyExist),
	}
	ErrRoleNotFound = response.CustomError{
		Code:       "ROLE_NOT_FOUND",
		StatusCode: http.StatusNotFound,
		Message:    "role not found",
		Detail:     GetMessageDetail(MsgRoleNotFound),
	}
	ErrRoleNotAssigned = response.CustomError{
		Code:       "ROLE_NOT_ASSIGNED",
		StatusCode: http.StatusBadRequest,
		Message:    "role not assigned to user",
		Detail:     GetMessageDetail(MsgRoleNotAssigned),
	}
	ErrRoleAlreadyAssigned = response.CustomError{
		Code:       "ROLE_ALREADY_ASSIGNED",
		StatusCode: http.StatusBadRequest,
		Message:    "role already assigned to user",
		Detail:     GetMessageDetail(MsgRoleAlreadyAssigned),
	}
	ErrRoleInUse = response.CustomError{
		Code:       "ROLE_IN_USE",
		StatusCode: http.StatusConflict,
		Message:    "role currently in use",
		Detail:     GetMessageDetail(MsgRoleInUse),
	}
	ErrOnlySuperAdmin = response.CustomError{
		Code:       "ONLY_SUPER_ADMIN",
		StatusCode: http.StatusForbidden,
		Message:    "only super admin",
		Detail:     GetMessageDetail(MsgOnlySuperAdmin),
	}

	// ====== MedikaOne – Auth (registrasi PIN) ======
	ErrEmailAlreadyActive = response.CustomError{
		Code:       "EMAIL_ALREADY_ACTIVE",
		StatusCode: http.StatusConflict,
		Message:    "email already registered and active",
		Detail:     GetMessageDetail(MsgConflict),
	}
	ErrEmailSendFailed = response.CustomError{
		Code:       "EMAIL_SEND_FAILED",
		StatusCode: http.StatusBadGateway,
		Message:    "failed to send verification email, please try again later",
		Detail:     GetMessageDetail(MsgInternalServerError),
	}
	ErrAccountRoleNotFound = response.CustomError{
		Code:       "ACCOUNT_ROLE_NOT_FOUND",
		StatusCode: http.StatusNotFound,
		Message:    "account role not found",
		Detail:     GetMessageDetail(MsgRoleNotFound),
	}

	// ====== Lain-lain yang masih direferensi di util/transport lama ======
	ErrUnauthorizedUpdate = response.CustomError{
		Code:       "UNAUTHORIZED_UPDATE",
		StatusCode: http.StatusForbidden,
		Message:    "You can only update your own user data",
	}
	ErrNewPasswordSame = response.CustomError{
		Code:       "NEW_PASSWORD_SAME",
		StatusCode: http.StatusBadRequest,
		Message:    "new password cannot be the same as old password",
	}
	ErrPasswordNotMatch = response.CustomError{
		Code:       "PASSWORD_NOT_MATCH",
		StatusCode: http.StatusUnauthorized,
		Message:    "password not match",
		Detail:     GetMessageDetail(MsgPasswordNotMatch),
	}
	ErrHospitalNotFound = response.CustomError{
		Code:       "HOSPITAL_NOT_FOUND",
		StatusCode: http.StatusNotFound,
		Message:    "hospital not found",
		Detail:     GetMessageDetail(MsgNotFound),
	}
	ErrUserNotLinkedToHospital = response.CustomError{
		Code:       "USER_NOT_LINKED_TO_HOSPITAL",
		StatusCode: http.StatusForbidden,
		Message:    "user is not linked to this hospital",
		Detail:     GetMessageDetail(MsgForbidden),
	}
	ErrProfileAlreadySet = response.CustomError{
		Code:       "PROFILE_ALREADY_SET",
		StatusCode: http.StatusConflict,
		Message:    "profile has already been set",
		Detail:     GetMessageDetail(MsgConflict),
	}
	ErrRegistrationError = response.CustomError{
		Code:       "REGISTRATION_ERROR",
		StatusCode: http.StatusInternalServerError,
		Message:    "internal server error",
		Detail:     GetMessageDetail(MsgInternalServerError),
	}

	// ====== Doctor hospital registration ======
	ErrDoctorNotEligible = response.CustomError{
		Code:       "DOCTOR_NOT_ELIGIBLE",
		StatusCode: http.StatusUnprocessableEntity,
		Message:    "doctor account must be active, verified, have the DOCTOR role, and have a SIP number",
	}
	ErrDoctorInvitationNotFound = response.CustomError{
		Code:       "DOCTOR_INVITATION_NOT_FOUND",
		StatusCode: http.StatusNotFound,
		Message:    "doctor hospital invitation not found",
	}
	ErrDoctorInvitationExists = response.CustomError{
		Code:       "DOCTOR_INVITATION_ALREADY_EXISTS",
		StatusCode: http.StatusConflict,
		Message:    "an open invitation or hospital affiliation already exists for this doctor",
	}
	ErrDoctorInvitationExpired = response.CustomError{
		Code:       "DOCTOR_INVITATION_EXPIRED",
		StatusCode: http.StatusConflict,
		Message:    "doctor hospital invitation has expired",
	}
	ErrInvalidDoctorInvitationState = response.CustomError{
		Code:       "INVALID_DOCTOR_INVITATION_STATE",
		StatusCode: http.StatusConflict,
		Message:    "doctor hospital invitation cannot perform this action in its current state",
	}
	ErrHospitalPlacementNotFound = response.CustomError{
		Code:       "HOSPITAL_PLACEMENT_NOT_FOUND",
		StatusCode: http.StatusNotFound,
		Message:    "department or room was not found in this hospital",
	}
	ErrDoctorScheduleConflict = response.CustomError{
		Code:       "DOCTOR_SCHEDULE_CONFLICT",
		StatusCode: http.StatusConflict,
		Message:    "the proposed schedule conflicts with an active doctor schedule",
	}
	ErrInvalidContractPDF = response.CustomError{
		Code:       "INVALID_CONTRACT_PDF",
		StatusCode: http.StatusBadRequest,
		Message:    "contract must be a valid PDF no larger than 10 MB",
	}
	ErrStorageUnavailable = response.CustomError{
		Code:       "STORAGE_UNAVAILABLE",
		StatusCode: http.StatusBadGateway,
		Message:    "document storage is temporarily unavailable",
	}
	ErrAffiliationNotFound = response.CustomError{
		Code:       "DOCTOR_AFFILIATION_NOT_FOUND",
		StatusCode: http.StatusNotFound,
		Message:    "doctor hospital affiliation not found",
	}
	ErrNotificationNotFound = response.CustomError{
		Code:       "NOTIFICATION_NOT_FOUND",
		StatusCode: http.StatusNotFound,
		Message:    "notification not found",
	}
)
