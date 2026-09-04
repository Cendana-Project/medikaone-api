package constant

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/Cendana-Project/medikaone-api/internal/model/response"
)

// apiError is the only constructor for static public API errors. The code is
// machine-readable; the bilingual detail is safe to display to end users.
func apiError(code string, status int, titleEng, descEng, titleIdn, descIdn string) response.CustomError {
	return response.CustomError{
		Code:       code,
		StatusCode: status,
		Message:    descEng,
		Detail: response.MessageDetail{
			TitleEng: titleEng,
			DescEng:  descEng,
			TitleIdn: titleIdn,
			DescIdn:  descIdn,
		},
	}
}

func safeFieldName(field string) string {
	field = strings.TrimSpace(field)
	if field == "" {
		return "request"
	}
	const maxFieldLength = 64
	runes := []rune(field)
	if len(runes) > maxFieldLength {
		field = string(runes[:maxFieldLength])
	}
	return field
}

// NewFieldRequiredError returns a stable code with field-specific guidance.
func NewFieldRequiredError(field string) response.CustomError {
	field = safeFieldName(field)
	return apiError(
		"FIELD_REQUIRED", http.StatusBadRequest,
		"Required field", fmt.Sprintf("The %q field is required.", field),
		"Field wajib diisi", fmt.Sprintf("Field %q wajib diisi.", field),
	)
}

// NewInvalidFieldValueError is used for cross-field and enum validation that
// cannot be expressed by a single static error value.
func NewInvalidFieldValueError(field, expectedEng, expectedIdn string) response.CustomError {
	field = safeFieldName(field)
	return apiError(
		"INVALID_FIELD_VALUE", http.StatusBadRequest,
		"Invalid field value", fmt.Sprintf("The %q field must be %s.", field, expectedEng),
		"Nilai field tidak valid", fmt.Sprintf("Field %q harus %s.", field, expectedIdn),
	)
}

func NewInvalidFieldLengthError(field, constraintEng, constraintIdn string) response.CustomError {
	field = safeFieldName(field)
	return apiError(
		"INVALID_FIELD_LENGTH", http.StatusBadRequest,
		"Invalid field length", fmt.Sprintf("The %q field must be %s.", field, constraintEng),
		"Panjang field tidak valid", fmt.Sprintf("Field %q harus %s.", field, constraintIdn),
	)
}

func NewInvalidFieldTypeError(field, expectedType string) response.CustomError {
	field = safeFieldName(field)
	expectedType = strings.TrimSpace(expectedType)
	if expectedType == "" {
		expectedType = "the documented JSON type"
	}
	return apiError(
		"INVALID_FIELD_TYPE", http.StatusBadRequest,
		"Invalid field type", fmt.Sprintf("The %q field must use type %s.", field, expectedType),
		"Tipe field tidak valid", fmt.Sprintf("Field %q harus menggunakan tipe %s.", field, expectedType),
	)
}

func NewUnknownFieldError(field string) response.CustomError {
	field = safeFieldName(field)
	return apiError(
		"UNKNOWN_FIELD", http.StatusBadRequest,
		"Unknown field", fmt.Sprintf("The %q field is not accepted by this endpoint.", field),
		"Field tidak dikenal", fmt.Sprintf("Field %q tidak diterima oleh endpoint ini.", field),
	)
}

func NewDuplicateFieldValueError(field string) response.CustomError {
	field = safeFieldName(field)
	return apiError(
		"DUPLICATE_FIELD_VALUE", http.StatusConflict,
		"Field value already exists", fmt.Sprintf("The value of the %q field is already in use.", field),
		"Nilai field sudah digunakan", fmt.Sprintf("Nilai field %q sudah digunakan.", field),
	)
}

// NewRequiredPermissionError identifies the exact permission alternatives
// accepted by an endpoint without creating an unstable code per permission.
func NewRequiredPermissionError(required ...string) response.CustomError {
	permissions := make([]string, 0, len(required))
	for _, permission := range required {
		permission = safeFieldName(permission)
		if permission != "request" {
			permissions = append(permissions, permission)
		}
	}
	if len(permissions) == 0 {
		return ErrForbidden
	}
	joined := strings.Join(permissions, ", ")
	return apiError(
		"REQUIRED_PERMISSION_MISSING", http.StatusForbidden,
		"Required permission missing", fmt.Sprintf("This action requires at least one of these permissions: %s.", joined),
		"Izin yang diperlukan belum dimiliki", fmt.Sprintf("Tindakan ini memerlukan minimal salah satu izin berikut: %s.", joined),
	)
}

var (
	// Common request and server errors.
	ErrInternalServerError = apiError(
		"INTERNAL_SERVER_ERROR", http.StatusInternalServerError,
		"Internal server error", "The server could not complete the request. Please try again later.",
		"Kesalahan server internal", "Server tidak dapat menyelesaikan permintaan. Silakan coba lagi nanti.",
	)
	ErrValidationError = apiError(
		"VALIDATION_ERROR", http.StatusBadRequest,
		"Request validation failed", "One or more request values are invalid.",
		"Validasi permintaan gagal", "Satu atau beberapa nilai permintaan tidak valid.",
	)
	// Backward-compatible Go identifier. It intentionally resolves to the same
	// public code so the API no longer exposes two codes for the same condition.
	ErrValidationFailed = ErrValidationError
	ErrMalformedJSON    = apiError(
		"MALFORMED_JSON", http.StatusBadRequest,
		"Malformed JSON", "The request body must contain exactly one valid JSON value.",
		"JSON tidak valid", "Body permintaan harus berisi tepat satu nilai JSON yang valid.",
	)
	ErrRequestBodyRequired = apiError(
		"REQUEST_BODY_REQUIRED", http.StatusBadRequest,
		"Request body required", "A JSON request body is required.",
		"Body permintaan wajib diisi", "Body permintaan dalam format JSON wajib diisi.",
	)
	ErrEndpointNotFound = apiError(
		"ENDPOINT_NOT_FOUND", http.StatusNotFound,
		"Endpoint not found", "The requested API endpoint does not exist.",
		"Endpoint tidak ditemukan", "Endpoint API yang diminta tidak tersedia.",
	)
	ErrUnauthorized = apiError(
		"UNAUTHORIZED", http.StatusUnauthorized,
		"Authentication required", "Valid authentication credentials are required for this request.",
		"Autentikasi diperlukan", "Kredensial autentikasi yang valid diperlukan untuk permintaan ini.",
	)
	ErrUserNotAuthenticated = ErrUnauthorized
	ErrForbidden            = apiError(
		"FORBIDDEN", http.StatusForbidden,
		"Access denied", "You do not have permission to perform this action.",
		"Akses ditolak", "Anda tidak memiliki izin untuk melakukan tindakan ini.",
	)
	ErrTooManyRequests = apiError(
		"RATE_LIMIT_EXCEEDED", http.StatusTooManyRequests,
		"Request limit exceeded", "Too many requests were received. Please try again later.",
		"Batas permintaan terlampaui", "Terlalu banyak permintaan diterima. Silakan coba lagi nanti.",
	)
	ErrPublicAuthRateLimitExceeded = apiError(
		"PUBLIC_AUTH_RATE_LIMIT_EXCEEDED", http.StatusTooManyRequests,
		"Authentication request limit exceeded", "Too many authentication requests were sent from this client. Try again after the cooldown.",
		"Batas permintaan autentikasi terlampaui", "Terlalu banyak permintaan autentikasi dikirim dari client ini. Coba lagi setelah masa tunggu.",
	)
	ErrRegistrationPINCooldown = apiError(
		"REGISTRATION_PIN_RESEND_COOLDOWN", http.StatusTooManyRequests,
		"Registration PIN cooldown active", "A registration PIN was sent recently. Wait before requesting another PIN.",
		"Masa tunggu PIN registrasi aktif", "PIN registrasi baru saja dikirim. Tunggu sebelum meminta PIN lainnya.",
	)
	ErrRegistrationPINAttemptsExceeded = apiError(
		"REGISTRATION_PIN_ATTEMPTS_EXCEEDED", http.StatusTooManyRequests,
		"Registration PIN attempt limit exceeded", "Too many incorrect PIN attempts invalidated this registration. Start registration again.",
		"Batas percobaan PIN registrasi terlampaui", "Terlalu banyak percobaan PIN yang salah membatalkan registrasi ini. Mulai registrasi kembali.",
	)
	ErrLoginAttemptsExceeded = apiError(
		"LOGIN_ATTEMPTS_EXCEEDED", http.StatusTooManyRequests,
		"Login attempt limit exceeded", "Too many login attempts were made for this account. Try again after the cooldown.",
		"Batas percobaan login terlampaui", "Terlalu banyak percobaan login dilakukan untuk akun ini. Coba lagi setelah masa tunggu.",
	)
	ErrPasswordResetRequestsExceeded = apiError(
		"PASSWORD_RESET_REQUEST_LIMIT_EXCEEDED", http.StatusTooManyRequests,
		"Password reset request limit exceeded", "Too many password reset requests were made. Try again after the cooldown.",
		"Batas permintaan reset password terlampaui", "Terlalu banyak permintaan reset password dilakukan. Coba lagi setelah masa tunggu.",
	)
	ErrPasswordResetPINAttemptsExceeded = apiError(
		"PASSWORD_RESET_PIN_ATTEMPTS_EXCEEDED", http.StatusTooManyRequests,
		"Password reset PIN attempt limit exceeded", "Too many incorrect PIN attempts invalidated this password reset. Request a new PIN.",
		"Batas percobaan PIN reset password terlampaui", "Terlalu banyak percobaan PIN yang salah membatalkan reset password ini. Minta PIN baru.",
	)
	ErrPasswordProcessingBusy = apiError(
		"PASSWORD_PROCESSING_BUSY", http.StatusServiceUnavailable,
		"Password processing busy", "Password processing capacity is temporarily full. Try again shortly.",
		"Pemrosesan password sedang sibuk", "Kapasitas pemrosesan password sementara penuh. Coba lagi sebentar lagi.",
	)
	ErrEmailDeliveryBusy = apiError(
		"EMAIL_DELIVERY_BUSY", http.StatusServiceUnavailable,
		"Email delivery busy", "The email delivery queue is temporarily full. Try again shortly.",
		"Pengiriman email sedang sibuk", "Antrean pengiriman email sementara penuh. Coba lagi sebentar lagi.",
	)
	ErrRequestTooLarge = apiError(
		"REQUEST_TOO_LARGE", http.StatusRequestEntityTooLarge,
		"Request too large", "The request body exceeds the 10 MB limit.",
		"Permintaan terlalu besar", "Body permintaan melebihi batas 10 MB.",
	)
	ErrServiceUnavailable = apiError(
		"SERVICE_UNAVAILABLE", http.StatusServiceUnavailable,
		"Service unavailable", "The service is temporarily unavailable. Please try again later.",
		"Layanan tidak tersedia", "Layanan sementara tidak tersedia. Silakan coba lagi nanti.",
	)
	ErrServiceNotReady = apiError(
		"SERVICE_NOT_READY", http.StatusServiceUnavailable,
		"Service not ready", "One or more required service dependencies are not ready.",
		"Layanan belum siap", "Satu atau beberapa dependensi layanan yang diperlukan belum siap.",
	)

	// Input and format errors.
	ErrInvalidEmail = apiError(
		"INVALID_EMAIL", http.StatusBadRequest,
		"Invalid email", "The email address must use a valid format.",
		"Email tidak valid", "Alamat email harus menggunakan format yang valid.",
	)
	ErrInvalidUsername = apiError(
		"INVALID_USERNAME", http.StatusBadRequest,
		"Invalid username", "The username must be 3-64 characters and contain only letters, numbers, dots, underscores, or hyphens.",
		"Username tidak valid", "Username harus terdiri dari 3-64 karakter dan hanya boleh berisi huruf, angka, titik, garis bawah, atau tanda hubung.",
	)
	ErrInvalidPassword = apiError(
		"INVALID_PASSWORD", http.StatusBadRequest,
		"Password does not meet requirements", "The password must be 8-128 characters and include uppercase, lowercase, number, and special characters without obvious sequences or repeated characters.",
		"Password tidak memenuhi persyaratan", "Password harus terdiri dari 8-128 karakter serta memiliki huruf besar, huruf kecil, angka, dan karakter khusus tanpa urutan atau pengulangan karakter yang mudah ditebak.",
	)
	ErrPasswordSimilarToUserInfo = apiError(
		"PASSWORD_SIMILAR_TO_USER_INFO", http.StatusBadRequest,
		"Password is too similar to account data", "The password must not contain or closely resemble the username or email address.",
		"Password terlalu mirip dengan data akun", "Password tidak boleh mengandung atau terlalu mirip dengan username maupun alamat email.",
	)
	ErrInvalidDateFormat = apiError(
		"INVALID_DATE_FORMAT", http.StatusBadRequest,
		"Invalid date format", "The date must use YYYY-MM-DD format and represent a valid calendar date.",
		"Format tanggal tidak valid", "Tanggal harus menggunakan format YYYY-MM-DD dan merupakan tanggal kalender yang valid.",
	)
	ErrInvalidUUIDFormat = apiError(
		"INVALID_UUID_FORMAT", http.StatusBadRequest,
		"Invalid identifier", "The identifier must be a valid UUID.",
		"Identifier tidak valid", "Identifier harus berupa UUID yang valid.",
	)
	ErrInvalidIdempotencyKey = apiError(
		"INVALID_IDEMPOTENCY_KEY", http.StatusBadRequest,
		"Invalid idempotency key", "The idempotency key must be a canonical RFC 4122 version 4 UUID.",
		"Idempotency key tidak valid", "Idempotency key harus berupa UUID versi 4 RFC 4122 yang kanonis.",
	)

	// Resource and identity errors.
	ErrRecordNotFound = apiError(
		"RECORD_NOT_FOUND", http.StatusNotFound,
		"Record not found", "The requested record does not exist or is no longer available.",
		"Data tidak ditemukan", "Data yang diminta tidak tersedia atau sudah tidak dapat diakses.",
	)
	ErrUserNotFound = apiError(
		"USER_NOT_FOUND", http.StatusNotFound,
		"User not found", "The requested user account does not exist.",
		"Pengguna tidak ditemukan", "Akun pengguna yang diminta tidak tersedia.",
	)
	ErrEmailAlreadyExists = apiError(
		"EMAIL_ALREADY_EXISTS", http.StatusConflict,
		"Email already registered", "This email address is already used by another account.",
		"Email sudah terdaftar", "Alamat email ini sudah digunakan oleh akun lain.",
	)
	ErrUsernameAlreadyExists = apiError(
		"USERNAME_ALREADY_EXISTS", http.StatusConflict,
		"Username already registered", "This username is already used by another account.",
		"Username sudah terdaftar", "Username ini sudah digunakan oleh akun lain.",
	)
	ErrUsernameRegistrationInProgress = apiError(
		"USERNAME_REGISTRATION_IN_PROGRESS", http.StatusConflict,
		"Username registration in progress", "This username is reserved by another unexpired registration.",
		"Registrasi username sedang berlangsung", "Username ini sedang digunakan oleh registrasi lain yang belum kedaluwarsa.",
	)
	ErrDuplicateUsernameOrEmail = apiError(
		"DUPLICATE_USERNAME_OR_EMAIL", http.StatusConflict,
		"Account identity already registered", "The email address or username is already used by another account.",
		"Identitas akun sudah terdaftar", "Alamat email atau username sudah digunakan oleh akun lain.",
	)
	ErrDuplicateNIK = apiError(
		"NIK_ALREADY_EXISTS", http.StatusConflict,
		"NIK already registered", "This NIK is already linked to another account.",
		"NIK sudah terdaftar", "NIK ini sudah terhubung dengan akun lain.",
	)
	ErrConflict = apiError(
		"RESOURCE_STATE_CONFLICT", http.StatusConflict,
		"Resource state conflict", "The request conflicts with the current state of the resource.",
		"Konflik status data", "Permintaan bertentangan dengan status data saat ini.",
	)

	// Authentication, token, and registration PIN errors.
	ErrInvalidToken = apiError(
		"INVALID_TOKEN", http.StatusUnauthorized,
		"Invalid authentication token", "The authentication token is malformed, invalid, or has been revoked.",
		"Token autentikasi tidak valid", "Token autentikasi rusak, tidak valid, atau sudah dicabut.",
	)
	ErrInvalidCredentials = apiError(
		"INVALID_CREDENTIALS", http.StatusUnauthorized,
		"Invalid credentials", "The supplied login identity or password is incorrect.",
		"Kredensial tidak valid", "Identitas login atau password yang diberikan tidak benar.",
	)
	ErrTokenExpired = apiError(
		"TOKEN_EXPIRED", http.StatusUnauthorized,
		"Authentication token expired", "The authentication token has expired. Sign in again or refresh the token.",
		"Token autentikasi kedaluwarsa", "Token autentikasi sudah kedaluwarsa. Silakan login kembali atau perbarui token.",
	)
	ErrRegistrationPINInvalidOrExpired = apiError(
		"REGISTRATION_PIN_INVALID_OR_EXPIRED", http.StatusBadRequest,
		"Registration PIN invalid or expired", "The registration PIN, email, or challenge ID is incorrect, expired, or already used.",
		"PIN registrasi tidak valid atau kedaluwarsa", "PIN registrasi, email, atau challenge ID salah, kedaluwarsa, atau sudah digunakan.",
	)
	ErrPasswordResetPINInvalidOrExpired = apiError(
		"PASSWORD_RESET_PIN_INVALID_OR_EXPIRED", http.StatusBadRequest,
		"Password reset PIN invalid or expired", "The password reset PIN, email, or challenge ID is incorrect, expired, or already used.",
		"PIN reset password tidak valid atau kedaluwarsa", "PIN reset password, email, atau challenge ID salah, kedaluwarsa, atau sudah digunakan.",
	)
	ErrInvalidOTP        = ErrRegistrationPINInvalidOrExpired
	ErrInvalidResetToken = apiError(
		"PASSWORD_RESET_TOKEN_INVALID_OR_EXPIRED", http.StatusBadRequest,
		"Password reset token invalid or expired", "The password reset token is invalid, expired, already used, or no longer matches the account.",
		"Token reset password tidak valid atau kedaluwarsa", "Token reset password tidak valid, kedaluwarsa, sudah digunakan, atau tidak lagi sesuai dengan akun.",
	)
	ErrEmailNotVerified = apiError(
		"EMAIL_NOT_VERIFIED", http.StatusForbidden,
		"Email not verified", "Verify the account email address before signing in.",
		"Email belum diverifikasi", "Verifikasi alamat email akun sebelum login.",
	)
	ErrEmailAlreadyActive = apiError(
		"EMAIL_ALREADY_ACTIVE", http.StatusConflict,
		"Email already registered", "This email address belongs to an active account. Sign in instead of registering again.",
		"Email sudah terdaftar", "Alamat email ini sudah terhubung dengan akun aktif. Silakan login dan tidak melakukan registrasi ulang.",
	)
	ErrEmailSendFailed = apiError(
		"VERIFICATION_EMAIL_SEND_FAILED", http.StatusBadGateway,
		"Verification email could not be sent", "The email provider could not send the verification message. Try again later.",
		"Email verifikasi gagal dikirim", "Penyedia email tidak dapat mengirim pesan verifikasi. Silakan coba lagi nanti.",
	)

	// Roles and authorization.
	ErrInvalidRoleID = apiError(
		"INVALID_ROLE_ID", http.StatusBadRequest,
		"Invalid role identifier", "The role_id value must identify a valid role.",
		"Identifier role tidak valid", "Nilai role_id harus merujuk pada role yang valid.",
	)
	ErrRoleAlreadyExist = apiError(
		"ROLE_ALREADY_EXISTS", http.StatusConflict,
		"Role already exists", "A role with the same identifier or name already exists.",
		"Role sudah tersedia", "Role dengan identifier atau nama yang sama sudah tersedia.",
	)
	ErrRoleNotFound = apiError(
		"ROLE_NOT_FOUND", http.StatusNotFound,
		"Role not found", "The requested role does not exist.",
		"Role tidak ditemukan", "Role yang diminta tidak tersedia.",
	)
	ErrRoleNotAssigned = apiError(
		"ROLE_NOT_ASSIGNED", http.StatusConflict,
		"Role not assigned", "The requested role is not assigned to this user.",
		"Role belum diberikan", "Role yang diminta belum diberikan kepada pengguna ini.",
	)
	ErrRoleAlreadyAssigned = apiError(
		"ROLE_ALREADY_ASSIGNED", http.StatusConflict,
		"Role already assigned", "The requested role is already assigned to this user.",
		"Role sudah diberikan", "Role yang diminta sudah diberikan kepada pengguna ini.",
	)
	ErrRoleInUse = apiError(
		"ROLE_IN_USE", http.StatusConflict,
		"Role is in use", "The role cannot be removed while it is assigned to users.",
		"Role sedang digunakan", "Role tidak dapat dihapus selama masih diberikan kepada pengguna.",
	)
	ErrOnlySuperAdmin = apiError(
		"SUPER_ADMIN_REQUIRED", http.StatusForbidden,
		"Super admin access required", "Only a super administrator can perform this action.",
		"Akses super admin diperlukan", "Hanya super administrator yang dapat melakukan tindakan ini.",
	)
	ErrDoctorRoleRequired = apiError(
		"DOCTOR_ROLE_REQUIRED", http.StatusForbidden,
		"Doctor access required", "An active DOCTOR role is required to perform this action.",
		"Akses dokter diperlukan", "Role DOCTOR yang aktif diperlukan untuk melakukan tindakan ini.",
	)
	ErrSelfServicePatientRoleOnly = apiError(
		"SELF_SERVICE_PATIENT_ROLE_ONLY", http.StatusForbidden,
		"Role unavailable for self-service", "Only the PATIENT role can be selected or initialized through this self-service endpoint.",
		"Role tidak tersedia untuk layanan mandiri", "Hanya role PATIENT yang dapat dipilih atau diinisialisasi melalui endpoint layanan mandiri ini.",
	)
	ErrAccountInactive = apiError(
		"ACCOUNT_INACTIVE", http.StatusForbidden,
		"Account is not active", "This action requires an active account.",
		"Akun tidak aktif", "Tindakan ini memerlukan akun yang aktif.",
	)
	ErrHospitalMembershipRoleRequired = apiError(
		"HOSPITAL_MEMBERSHIP_ROLE_REQUIRED", http.StatusForbidden,
		"Hospital membership role required", "The account does not have an active role in the selected hospital.",
		"Role keanggotaan rumah sakit diperlukan", "Akun tidak memiliki role aktif pada rumah sakit yang dipilih.",
	)
	ErrAccountRoleNotFound = apiError(
		"ACCOUNT_ROLE_NOT_FOUND", http.StatusNotFound,
		"Account role not found", "The account does not have the requested role.",
		"Role akun tidak ditemukan", "Akun tidak memiliki role yang diminta.",
	)
	ErrUnauthorizedUpdate = apiError(
		"USER_UPDATE_FORBIDDEN", http.StatusForbidden,
		"User update not permitted", "You may only update your own user data.",
		"Pembaruan pengguna tidak diizinkan", "Anda hanya dapat memperbarui data pengguna milik sendiri.",
	)

	// Password and profile errors.
	ErrNewPasswordSame = apiError(
		"NEW_PASSWORD_SAME_AS_CURRENT", http.StatusBadRequest,
		"New password matches current password", "The new password must be different from the current password.",
		"Password baru sama dengan password saat ini", "Password baru harus berbeda dari password saat ini.",
	)
	ErrPasswordNotMatch = apiError(
		"CURRENT_PASSWORD_INCORRECT", http.StatusUnauthorized,
		"Current password incorrect", "The current password is incorrect.",
		"Password saat ini salah", "Password saat ini tidak benar.",
	)
	ErrProfileAlreadySet = apiError(
		"PROFILE_ALREADY_SET", http.StatusConflict,
		"Profile already completed", "The profile has already been completed and cannot be initialized again.",
		"Profil sudah dilengkapi", "Profil sudah dilengkapi dan tidak dapat diinisialisasi kembali.",
	)
	ErrProfileUpdateEmpty = apiError(
		"PROFILE_UPDATE_EMPTY", http.StatusBadRequest,
		"Profile update is empty", "Provide at least one supported profile field to update.",
		"Pembaruan profil kosong", "Isi minimal satu field profil yang didukung untuk diperbarui.",
	)
	ErrProfilePhotoInvalid = apiError(
		"PROFILE_PHOTO_INVALID", http.StatusBadRequest,
		"Profile photo is invalid", "Upload a valid JPEG or PNG image no larger than 10 MB and no larger than 4096 by 4096 pixels.",
		"Foto profil tidak valid", "Unggah gambar JPEG atau PNG yang valid dengan ukuran maksimal 10 MB dan dimensi maksimal 4096 kali 4096 piksel.",
	)
	ErrProfilePhotoNotFound = apiError(
		"PROFILE_PHOTO_NOT_FOUND", http.StatusNotFound,
		"Profile photo not found", "This account does not have a profile photo.",
		"Foto profil tidak ditemukan", "Akun ini belum memiliki foto profil.",
	)
	ErrRegistrationError = apiError(
		"REGISTRATION_FAILED", http.StatusInternalServerError,
		"Registration failed", "The server could not complete account registration. Try again later.",
		"Registrasi gagal", "Server tidak dapat menyelesaikan registrasi akun. Silakan coba lagi nanti.",
	)

	// Hospital and tenant errors.
	ErrHospitalNotFound = apiError(
		"HOSPITAL_NOT_FOUND", http.StatusNotFound,
		"Hospital not found", "The requested hospital does not exist or is not active.",
		"Rumah sakit tidak ditemukan", "Rumah sakit yang diminta tidak tersedia atau tidak aktif.",
	)
	ErrHospitalContextRequired = apiError(
		"HOSPITAL_CONTEXT_REQUIRED", http.StatusBadRequest,
		"Hospital context required", "Provide a hospital through the route or the X-Hospital-ID or X-Hospital-Code header.",
		"Konteks rumah sakit diperlukan", "Tentukan rumah sakit melalui route atau header X-Hospital-ID maupun X-Hospital-Code.",
	)
	ErrHospitalAdminRequired = apiError(
		"HOSPITAL_ADMIN_REQUIRED", http.StatusForbidden,
		"Hospital administrator access required", "Only a hospital administrator for this hospital or a super administrator may perform this action.",
		"Akses administrator rumah sakit diperlukan", "Hanya administrator rumah sakit ini atau super administrator yang dapat melakukan tindakan ini.",
	)
	ErrHospitalCodeAlreadyExists = apiError(
		"HOSPITAL_CODE_ALREADY_EXISTS", http.StatusConflict,
		"Hospital code already exists", "This hospital code is already used by another hospital.",
		"Kode rumah sakit sudah digunakan", "Kode rumah sakit ini sudah digunakan oleh rumah sakit lain.",
	)
	ErrHospitalNameAlreadyExists = apiError(
		"HOSPITAL_NAME_ALREADY_EXISTS", http.StatusConflict,
		"Hospital name already exists", "This hospital name is already registered.",
		"Nama rumah sakit sudah terdaftar", "Nama rumah sakit ini sudah terdaftar.",
	)
	ErrHospitalAlreadyExists = apiError(
		"HOSPITAL_ALREADY_EXISTS", http.StatusConflict,
		"Hospital already exists", "A hospital with the same code or name already exists.",
		"Rumah sakit sudah terdaftar", "Rumah sakit dengan kode atau nama yang sama sudah terdaftar.",
	)
	ErrInvalidHospitalCoordinates = apiError(
		"INVALID_HOSPITAL_COORDINATES", http.StatusBadRequest,
		"Invalid hospital coordinates", "Latitude must be between -90 and 90 and longitude between -180 and 180.",
		"Koordinat rumah sakit tidak valid", "Latitude harus berada di antara -90 dan 90 serta longitude di antara -180 dan 180.",
	)
	ErrInvalidHospitalFacilities = apiError(
		"INVALID_HOSPITAL_FACILITIES", http.StatusBadRequest,
		"Invalid hospital facilities", "The facilities value must be valid JSON data.",
		"Fasilitas rumah sakit tidak valid", "Nilai fasilitas harus berupa data JSON yang valid.",
	)
	ErrUserNotLinkedToHospital = apiError(
		"USER_NOT_LINKED_TO_HOSPITAL", http.StatusForbidden,
		"Hospital membership required", "This user is not an active member of the selected hospital.",
		"Keanggotaan rumah sakit diperlukan", "Pengguna ini bukan anggota aktif rumah sakit yang dipilih.",
	)

	// Doctor-hospital registration.
	ErrDoctorNotEligible = apiError(
		"DOCTOR_NOT_ELIGIBLE", http.StatusUnprocessableEntity,
		"Doctor is not eligible", "The doctor account must be active and verified, have the DOCTOR role, and contain a SIP number.",
		"Dokter belum memenuhi syarat", "Akun dokter harus aktif dan terverifikasi, memiliki role DOCTOR, serta memiliki nomor SIP.",
	)
	ErrDoctorInvitationNotFound = apiError(
		"DOCTOR_INVITATION_NOT_FOUND", http.StatusNotFound,
		"Doctor invitation not found", "The requested doctor-hospital invitation does not exist.",
		"Undangan dokter tidak ditemukan", "Undangan dokter ke rumah sakit yang diminta tidak tersedia.",
	)
	ErrDoctorInvitationExists = apiError(
		"DOCTOR_INVITATION_ALREADY_EXISTS", http.StatusConflict,
		"Doctor invitation already exists", "An open invitation or affiliation already exists for this doctor placement.",
		"Undangan dokter sudah tersedia", "Undangan aktif atau afiliasi sudah tersedia untuk penempatan dokter ini.",
	)
	ErrDoctorInvitationExpired = apiError(
		"DOCTOR_INVITATION_EXPIRED", http.StatusConflict,
		"Doctor invitation expired", "This doctor-hospital invitation has expired and can no longer be processed.",
		"Undangan dokter kedaluwarsa", "Undangan dokter ke rumah sakit ini sudah kedaluwarsa dan tidak dapat diproses.",
	)
	ErrInvalidDoctorInvitationState = apiError(
		"DOCTOR_INVITATION_STATE_CONFLICT", http.StatusConflict,
		"Doctor invitation state conflict", "The invitation cannot perform this action in its current state.",
		"Konflik status undangan dokter", "Undangan tidak dapat menjalankan tindakan ini pada status saat ini.",
	)
	ErrHospitalPlacementNotFound = apiError(
		"HOSPITAL_PLACEMENT_NOT_FOUND", http.StatusNotFound,
		"Hospital placement not found", "The selected department or room does not exist in this hospital.",
		"Penempatan rumah sakit tidak ditemukan", "Departemen atau ruangan yang dipilih tidak tersedia di rumah sakit ini.",
	)
	ErrDepartmentAlreadyExists = apiError(
		"DEPARTMENT_ALREADY_EXISTS", http.StatusConflict,
		"Department already exists", "A department with the same code already exists in this hospital.",
		"Departemen sudah tersedia", "Departemen dengan kode yang sama sudah tersedia di rumah sakit ini.",
	)
	ErrRoomAlreadyExists = apiError(
		"ROOM_ALREADY_EXISTS", http.StatusConflict,
		"Room already exists", "A room with the same code already exists in this hospital department.",
		"Ruangan sudah tersedia", "Ruangan dengan kode yang sama sudah tersedia di departemen rumah sakit ini.",
	)
	ErrDoctorScheduleConflict = apiError(
		"DOCTOR_SCHEDULE_CONFLICT", http.StatusConflict,
		"Doctor schedule conflict", "The proposed practice schedule overlaps another active doctor schedule.",
		"Jadwal dokter bertabrakan", "Jadwal praktik yang diajukan bertabrakan dengan jadwal dokter aktif lainnya.",
	)
	ErrInvalidContractPDF = apiError(
		"INVALID_CONTRACT_PDF", http.StatusBadRequest,
		"Invalid contract document", "The contract must be a valid PDF file no larger than 10 MB.",
		"Dokumen kontrak tidak valid", "Kontrak harus berupa file PDF yang valid dengan ukuran maksimal 10 MB.",
	)
	ErrStorageUnavailable = apiError(
		"DOCUMENT_STORAGE_UNAVAILABLE", http.StatusBadGateway,
		"Document storage unavailable", "The document storage provider could not complete the request. Try again later.",
		"Penyimpanan dokumen tidak tersedia", "Penyedia penyimpanan dokumen tidak dapat menyelesaikan permintaan. Silakan coba lagi nanti.",
	)
	ErrAffiliationNotFound = apiError(
		"DOCTOR_AFFILIATION_NOT_FOUND", http.StatusNotFound,
		"Doctor affiliation not found", "The requested doctor-hospital affiliation does not exist or is not active.",
		"Afiliasi dokter tidak ditemukan", "Afiliasi dokter dengan rumah sakit yang diminta tidak tersedia atau tidak aktif.",
	)
	ErrNotificationNotFound = apiError(
		"NOTIFICATION_NOT_FOUND", http.StatusNotFound,
		"Notification not found", "The requested notification does not exist or does not belong to this user.",
		"Notifikasi tidak ditemukan", "Notifikasi yang diminta tidak tersedia atau bukan milik pengguna ini.",
	)

	// Appointment and doctor schedule errors.
	ErrScheduleNotFound = apiError(
		"DOCTOR_SCHEDULE_NOT_FOUND", http.StatusNotFound,
		"Doctor schedule not found", "No active doctor schedule matches the requested hospital, doctor, and date.",
		"Jadwal dokter tidak ditemukan", "Tidak ada jadwal dokter aktif yang sesuai dengan rumah sakit, dokter, dan tanggal yang diminta.",
	)
	ErrScheduleChangeNotFound = apiError(
		"SCHEDULE_CHANGE_NOT_FOUND", http.StatusNotFound,
		"Schedule change not found", "The requested schedule change does not exist.",
		"Perubahan jadwal tidak ditemukan", "Perubahan jadwal yang diminta tidak tersedia.",
	)
	ErrScheduleChangeExists = apiError(
		"SCHEDULE_CHANGE_ALREADY_PENDING", http.StatusConflict,
		"Schedule change already pending", "A schedule change is already awaiting review for this affiliation.",
		"Perubahan jadwal masih menunggu", "Perubahan jadwal untuk afiliasi ini masih menunggu persetujuan.",
	)
	ErrScheduleChangeOwnApproval = apiError(
		"SCHEDULE_CHANGE_COUNTERPART_REVIEW_REQUIRED", http.StatusForbidden,
		"Counterparty review required", "A schedule change must be reviewed by the party that did not submit it.",
		"Persetujuan pihak lain diperlukan", "Perubahan jadwal harus ditinjau oleh pihak yang tidak mengajukannya.",
	)
	ErrScheduleChangeHasAppointments = apiError(
		"SCHEDULE_CHANGE_HAS_ACTIVE_APPOINTMENTS", http.StatusConflict,
		"Schedule has active appointments", "Cancel or reschedule every affected active appointment before approving this change.",
		"Jadwal memiliki appointment aktif", "Batalkan atau jadwalkan ulang seluruh appointment aktif yang terdampak sebelum menyetujui perubahan ini.",
	)
	ErrInvalidScheduleChangeState = apiError(
		"SCHEDULE_CHANGE_STATE_CONFLICT", http.StatusConflict,
		"Schedule change state conflict", "The schedule change cannot perform this action in its current state.",
		"Konflik status perubahan jadwal", "Perubahan jadwal tidak dapat menjalankan tindakan ini pada status saat ini.",
	)
	ErrAppointmentNotFound = apiError(
		"APPOINTMENT_NOT_FOUND", http.StatusNotFound,
		"Appointment not found", "The requested appointment does not exist or is not visible to this user.",
		"Appointment tidak ditemukan", "Appointment yang diminta tidak tersedia atau tidak dapat dilihat oleh pengguna ini.",
	)
	ErrAppointmentSlotUnavailable = apiError(
		"APPOINTMENT_SLOT_UNAVAILABLE", http.StatusConflict,
		"Appointment slot unavailable", "The selected slot is full, outside the practice schedule, or no longer available.",
		"Slot appointment tidak tersedia", "Slot yang dipilih sudah penuh, berada di luar jadwal praktik, atau sudah tidak tersedia.",
	)
	ErrAppointmentInvalidState = apiError(
		"APPOINTMENT_STATE_CONFLICT", http.StatusConflict,
		"Appointment state conflict", "The appointment cannot perform this action in its current state.",
		"Konflik status appointment", "Appointment tidak dapat menjalankan tindakan ini pada status saat ini.",
	)
	ErrAppointmentCutoffPassed = apiError(
		"APPOINTMENT_CHANGE_CUTOFF_PASSED", http.StatusConflict,
		"Appointment change cutoff passed", "The patient cancellation or reschedule deadline has passed.",
		"Batas waktu perubahan appointment terlewati", "Batas waktu pembatalan atau penjadwalan ulang oleh pasien sudah terlewati.",
	)
	ErrAppointmentOutsideCheckInWindow = apiError(
		"APPOINTMENT_OUTSIDE_CHECK_IN_WINDOW", http.StatusConflict,
		"Outside appointment check-in window", "Check-in is only available during the configured window around the appointment time.",
		"Di luar waktu check-in appointment", "Check-in hanya tersedia pada rentang waktu yang ditentukan di sekitar waktu appointment.",
	)
	ErrAppointmentInvalidVerification = apiError(
		"APPOINTMENT_VERIFICATION_INVALID_OR_USED", http.StatusBadRequest,
		"Appointment verification invalid", "The appointment verification code is incorrect or has already been used.",
		"Verifikasi appointment tidak valid", "Kode verifikasi appointment salah atau sudah digunakan.",
	)
	ErrCheckInLookupModeInvalid = apiError(
		"CHECK_IN_LOOKUP_MODE_INVALID", http.StatusBadRequest,
		"Check-in lookup mode invalid", "Use exactly one lookup mode: QR payload, appointment number with verification code, or patient identity.",
		"Mode pencarian check-in tidak valid", "Gunakan tepat satu mode pencarian: payload QR, nomor appointment dengan kode verifikasi, atau identitas pasien.",
	)
	ErrCheckInIdentityInsufficient = apiError(
		"CHECK_IN_IDENTITY_INSUFFICIENT", http.StatusBadRequest,
		"Patient identity is insufficient", "Provide at least two matching identity fields. A name must be accompanied by date of birth.",
		"Identitas pasien belum cukup", "Kirim sekurangnya dua data identitas yang cocok. Nama wajib disertai tanggal lahir.",
	)
	ErrAppointmentQRInvalid = apiError(
		"APPOINTMENT_QR_INVALID", http.StatusBadRequest,
		"Appointment QR is invalid", "The QR payload is invalid, expired, or belongs to another hospital.",
		"QR appointment tidak valid", "Payload QR tidak valid, sudah kedaluwarsa, atau berasal dari rumah sakit lain.",
	)
	ErrAppointmentVerificationCodeInvalid = apiError(
		"APPOINTMENT_VERIFICATION_CODE_INVALID", http.StatusBadRequest,
		"Appointment verification code is invalid", "The verification code does not match this appointment.",
		"Kode verifikasi appointment tidak valid", "Kode verifikasi tidak cocok dengan appointment ini.",
	)
	ErrCheckInTokenInvalidOrExpired = apiError(
		"CHECK_IN_TOKEN_INVALID_OR_EXPIRED", http.StatusBadRequest,
		"Check-in confirmation token is invalid", "Run the check-in lookup again because the confirmation token is invalid or has expired.",
		"Token konfirmasi check-in tidak valid", "Ulangi pencarian check-in karena token konfirmasi tidak valid atau sudah kedaluwarsa.",
	)
	ErrAppointmentAlreadyCheckedIn = apiError(
		"APPOINTMENT_ALREADY_CHECKED_IN", http.StatusConflict,
		"Appointment already checked in", "This appointment has already entered the hospital service queue.",
		"Appointment sudah check-in", "Appointment ini sudah masuk ke antrean pelayanan rumah sakit.",
	)
	ErrAppointmentCheckInExpired = apiError(
		"APPOINTMENT_CHECK_IN_EXPIRED", http.StatusConflict,
		"Appointment check-in has expired", "Late check-in is only allowed until the end of the appointment date in the schedule timezone.",
		"Waktu check-in appointment sudah berakhir", "Check-in terlambat hanya diizinkan sampai akhir tanggal appointment sesuai zona waktu jadwal.",
	)
	ErrAppointmentLateOverrideReasonRequired = apiError(
		"APPOINTMENT_LATE_OVERRIDE_REASON_REQUIRED", http.StatusBadRequest,
		"Late check-in reason required", "Provide an override reason to check in an appointment after the grace period or from NO_SHOW.",
		"Alasan override check-in terlambat diperlukan", "Isi alasan override untuk check-in setelah batas toleransi atau dari status NO_SHOW.",
	)
	ErrWalkInPatientDataRequired = apiError(
		"WALK_IN_PATIENT_DATA_REQUIRED", http.StatusBadRequest,
		"Walk-in patient data required", "Provide a patient record ID, MedikaOne ID, or the complete minimum identity for a new walk-in patient.",
		"Data pasien walk-in diperlukan", "Kirim ID patient record, ID MedikaOne, atau identitas minimum yang lengkap untuk pasien walk-in baru.",
	)
	ErrWalkInPatientModeInvalid = apiError(
		"WALK_IN_PATIENT_MODE_INVALID", http.StatusBadRequest,
		"Walk-in patient selection is ambiguous", "Use exactly one patient selection mode: patient record ID, MedikaOne ID, or complete patient identity.",
		"Pilihan pasien walk-in ambigu", "Gunakan tepat satu cara memilih pasien: ID patient record, ID MedikaOne, atau identitas pasien lengkap.",
	)
	ErrWalkInPatientNotFound = apiError(
		"WALK_IN_PATIENT_NOT_FOUND", http.StatusNotFound,
		"Walk-in patient not found", "No patient matches the supplied patient record ID or MedikaOne ID.",
		"Pasien walk-in tidak ditemukan", "Tidak ada pasien yang cocok dengan ID patient record atau ID MedikaOne yang diberikan.",
	)
	ErrWalkInPatientIdentityConflict = apiError(
		"WALK_IN_PATIENT_IDENTITY_CONFLICT", http.StatusConflict,
		"Walk-in patient identity conflicts", "The identity number already exists with a different date of birth; do not create or overwrite the patient record.",
		"Identitas pasien walk-in berkonflik", "Nomor identitas sudah tercatat dengan tanggal lahir berbeda; jangan membuat atau menimpa patient record.",
	)
	ErrWalkInCapacityFull = apiError(
		"WALK_IN_CAPACITY_FULL", http.StatusConflict,
		"Walk-in capacity is full", "The selected doctor schedule has no remaining capacity for a walk-in patient.",
		"Kapasitas walk-in penuh", "Jadwal dokter yang dipilih tidak memiliki sisa kapasitas untuk pasien walk-in.",
	)
	ErrWalkInCapacityOverrideForbidden = apiError(
		"WALK_IN_CAPACITY_OVERRIDE_FORBIDDEN", http.StatusForbidden,
		"Walk-in capacity override forbidden", "Only a hospital administrator may exceed the configured appointment capacity.",
		"Override kapasitas walk-in tidak diizinkan", "Hanya administrator rumah sakit yang dapat melebihi kapasitas appointment.",
	)
	ErrWalkInCapacityOverrideReasonRequired = apiError(
		"WALK_IN_CAPACITY_OVERRIDE_REASON_REQUIRED", http.StatusBadRequest,
		"Capacity override reason required", "Provide an auditable reason when exceeding the configured walk-in capacity.",
		"Alasan override kapasitas diperlukan", "Isi alasan yang dapat diaudit saat melebihi kapasitas walk-in.",
	)
	ErrPatientRecordNotFound = apiError(
		"PATIENT_RECORD_NOT_FOUND", http.StatusNotFound,
		"Patient record not found", "No patient record matches the supplied identity and date of birth.",
		"Patient record tidak ditemukan", "Tidak ada patient record yang cocok dengan identitas dan tanggal lahir yang diberikan.",
	)
	ErrPatientRecordAlreadyClaimed = apiError(
		"PATIENT_RECORD_ALREADY_CLAIMED", http.StatusConflict,
		"Patient record already claimed", "This patient record is already linked to another MedikaOne account.",
		"Patient record sudah diklaim", "Patient record ini sudah ditautkan ke akun MedikaOne lain.",
	)
	ErrPatientRecordIdentityMismatch = apiError(
		"PATIENT_RECORD_IDENTITY_MISMATCH", http.StatusForbidden,
		"Patient identity does not match", "The authenticated account information does not match the walk-in patient record.",
		"Identitas pasien tidak cocok", "Informasi akun yang sedang login tidak cocok dengan patient record walk-in.",
	)
	ErrAppointmentConsentRequired = apiError(
		"APPOINTMENT_DATA_CONSENT_REQUIRED", http.StatusBadRequest,
		"Data-sharing consent required", "Accept the current patient data-sharing consent before booking an appointment.",
		"Persetujuan berbagi data diperlukan", "Setujui ketentuan berbagi data pasien yang berlaku sebelum membuat appointment.",
	)
	ErrIdempotencyKeyRequired = apiError(
		"IDEMPOTENCY_KEY_REQUIRED", http.StatusBadRequest,
		"Idempotency key required", "Provide a valid UUID in the Idempotency-Key header.",
		"Idempotency key diperlukan", "Kirim UUID yang valid melalui header Idempotency-Key.",
	)
	ErrIdempotencyConflict = apiError(
		"IDEMPOTENCY_KEY_REUSED", http.StatusConflict,
		"Idempotency key already used", "This Idempotency-Key was already used with a different request payload.",
		"Idempotency key sudah digunakan", "Idempotency-Key ini sudah digunakan dengan payload permintaan yang berbeda.",
	)
	ErrMedicalEncounterNotFound = apiError(
		"MEDICAL_ENCOUNTER_NOT_FOUND", http.StatusNotFound,
		"Medical encounter not found", "The medical encounter does not exist or is not visible to this user.",
		"Pemeriksaan tidak ditemukan", "Data pemeriksaan tidak tersedia atau tidak dapat dilihat oleh pengguna ini.",
	)
	ErrExaminationInvalidState = apiError(
		"EXAMINATION_STATE_CONFLICT", http.StatusConflict,
		"Examination state conflict", "The examination action is not available in the appointment's current state.",
		"Konflik status pemeriksaan", "Tindakan pemeriksaan tidak tersedia pada status appointment saat ini.",
	)
	ErrVitalsDraftNotFound = apiError(
		"VITALS_DRAFT_NOT_FOUND", http.StatusConflict,
		"Vital signs draft not found", "Save a vital signs draft before finalizing it.",
		"Draft tanda vital tidak ditemukan", "Simpan draft tanda vital sebelum melakukan finalisasi.",
	)
	ErrVitalsRevisionNotFound = apiError(
		"VITALS_REVISION_NOT_FOUND", http.StatusConflict,
		"Finalized vital signs not found", "Finalize vital signs before creating a correction.",
		"Tanda vital final tidak ditemukan", "Finalisasi tanda vital sebelum membuat koreksi.",
	)
	ErrVitalsContentRequired = apiError(
		"VITALS_CONTENT_REQUIRED", http.StatusBadRequest,
		"Vital signs content required", "Provide at least one measurement or a reason for skipping vital signs.",
		"Data tanda vital diperlukan", "Isi minimal satu pengukuran atau alasan tanda vital dilewati.",
	)
	ErrBloodPressureInvalid = apiError(
		"BLOOD_PRESSURE_INVALID", http.StatusBadRequest,
		"Blood pressure is invalid", "Systolic blood pressure must be greater than diastolic blood pressure.",
		"Tekanan darah tidak valid", "Tekanan darah sistolik harus lebih besar daripada tekanan darah diastolik.",
	)
	ErrClinicalSOAPRequired = apiError(
		"CLINICAL_SOAP_REQUIRED", http.StatusBadRequest,
		"Complete SOAP note required", "Subjective, objective, assessment, and plan must all be completed before finishing the examination.",
		"Catatan SOAP lengkap diperlukan", "Subjective, objective, assessment, dan plan wajib dilengkapi sebelum menyelesaikan pemeriksaan.",
	)
	ErrConsultationDraftNotFound = apiError(
		"CONSULTATION_DRAFT_NOT_FOUND", http.StatusConflict,
		"Consultation draft not found", "Save a complete consultation draft before finishing the examination.",
		"Draft konsultasi tidak ditemukan", "Simpan draft konsultasi lengkap sebelum menyelesaikan pemeriksaan.",
	)
	ErrConsultationRevisionNotFound = apiError(
		"CONSULTATION_REVISION_NOT_FOUND", http.StatusConflict,
		"Finalized consultation not found", "Complete the consultation before creating a correction.",
		"Konsultasi final tidak ditemukan", "Selesaikan konsultasi sebelum membuat koreksi.",
	)
	ErrDiagnosisRequired = apiError(
		"DIAGNOSIS_REQUIRED", http.StatusBadRequest,
		"Diagnosis required", "Provide at least one diagnosis and exactly one primary diagnosis.",
		"Diagnosis diperlukan", "Isi minimal satu diagnosis dan tepat satu diagnosis utama.",
	)
	ErrMedicalAttachmentInvalid = apiError(
		"MEDICAL_ATTACHMENT_INVALID", http.StatusBadRequest,
		"Medical attachment is invalid", "Upload a valid PDF, JPEG, or PNG file no larger than 10 MB.",
		"Lampiran medis tidak valid", "Unggah file PDF, JPEG, atau PNG yang valid dengan ukuran maksimal 10 MB.",
	)
	ErrMedicalAttachmentNotFound = apiError(
		"MEDICAL_ATTACHMENT_NOT_FOUND", http.StatusNotFound,
		"Medical attachment not found", "The medical attachment does not exist or is not visible to this user.",
		"Lampiran medis tidak ditemukan", "Lampiran medis tidak tersedia atau tidak dapat dilihat oleh pengguna ini.",
	)
	ErrPrescriptionNotFound = apiError(
		"PRESCRIPTION_NOT_FOUND", http.StatusNotFound,
		"Prescription not found", "The prescription does not exist or is not visible to this user.",
		"Resep tidak ditemukan", "Resep tidak tersedia atau tidak dapat dilihat oleh pengguna ini.",
	)
	ErrPrescriptionNotAvailable = apiError(
		"PRESCRIPTION_NOT_AVAILABLE", http.StatusNotFound,
		"Issued prescription not available", "This examination does not have an issued prescription available for hospital access.",
		"Resep terbit belum tersedia", "Pemeriksaan ini belum memiliki resep terbit yang dapat diakses rumah sakit.",
	)
	ErrPrescriptionInvalidState = apiError(
		"PRESCRIPTION_STATE_CONFLICT", http.StatusConflict,
		"Prescription state conflict", "The prescription action is not available in the examination's current state.",
		"Konflik status resep", "Tindakan resep tidak tersedia pada status pemeriksaan saat ini.",
	)
	ErrPrescriptionPrimaryDiagnosisRequired = apiError(
		"PRESCRIPTION_PRIMARY_DIAGNOSIS_REQUIRED", http.StatusConflict,
		"Primary diagnosis required", "Save a primary diagnosis before creating a prescription.",
		"Diagnosis utama diperlukan", "Simpan diagnosis utama sebelum membuat resep.",
	)
	ErrPrescriptionDoctorSIPRequired = apiError(
		"PRESCRIPTION_DOCTOR_SIP_REQUIRED", http.StatusConflict,
		"Doctor SIP required", "Complete the prescribing doctor's SIP number before creating or issuing an electronic prescription.",
		"SIP dokter diperlukan", "Lengkapi nomor SIP dokter pemberi resep sebelum membuat atau menerbitkan resep elektronik.",
	)
	ErrPrescriptionItemsRequired = apiError(
		"PRESCRIPTION_ITEMS_REQUIRED", http.StatusBadRequest,
		"Prescription items required", "Add at least one medication item to the prescription draft.",
		"Item resep diperlukan", "Tambahkan minimal satu item obat ke draft resep.",
	)
	ErrPrescriptionScheduleRequired = apiError(
		"PRESCRIPTION_SCHEDULE_REQUIRED", http.StatusBadRequest,
		"Medication schedule required", "Each medication must specify frequency, interval, or as-needed use.",
		"Jadwal penggunaan obat diperlukan", "Setiap obat wajib memiliki frekuensi, interval, atau aturan bila perlu.",
	)
	ErrPrescriptionCompoundInvalid = apiError(
		"PRESCRIPTION_COMPOUND_INVALID", http.StatusBadRequest,
		"Compound prescription item invalid", "A compound item must contain components and cannot directly reference one catalogue medication; a non-compound item cannot contain components.",
		"Item resep racikan tidak valid", "Item racikan wajib memiliki komponen dan tidak boleh langsung merujuk satu obat katalog; item non-racikan tidak boleh memiliki komponen.",
	)
	ErrControlledMedicationUnsupported = apiError(
		"CONTROLLED_MEDICATION_UNSUPPORTED", http.StatusUnprocessableEntity,
		"Controlled medication unsupported", "Narcotic and psychotropic medications cannot be prescribed in this release.",
		"Obat terkendali belum didukung", "Obat narkotika dan psikotropika belum dapat diresepkan pada rilis ini.",
	)
	ErrPrescriptionDraftRequired = apiError(
		"PRESCRIPTION_DRAFT_REQUIRED", http.StatusConflict,
		"Prescription draft required", "Save a prescription draft containing at least one medication before issuing it.",
		"Draft resep diperlukan", "Simpan draft resep yang berisi minimal satu obat sebelum menerbitkannya.",
	)
	ErrPrescriptionAllergyReviewRequired = apiError(
		"PRESCRIPTION_ALLERGY_REVIEW_REQUIRED", http.StatusBadRequest,
		"Allergy review confirmation required", "Confirm that the patient's recorded allergies were reviewed before issuing the prescription.",
		"Konfirmasi tinjauan alergi diperlukan", "Konfirmasikan bahwa alergi pasien yang tercatat sudah ditinjau sebelum resep diterbitkan.",
	)
	ErrPrescriptionDecisionRequired = apiError(
		"PRESCRIPTION_DECISION_REQUIRED", http.StatusConflict,
		"Prescription decision required", "Issue a prescription or explicitly record no medication before completing the examination.",
		"Keputusan resep diperlukan", "Terbitkan resep atau catat secara eksplisit bahwa tidak ada obat sebelum menyelesaikan pemeriksaan.",
	)
	ErrPrescriptionCorrectionInvalidState = apiError(
		"PRESCRIPTION_CORRECTION_STATE_CONFLICT", http.StatusConflict,
		"Prescription cannot be corrected", "Only an issued prescription can be superseded by a correction.",
		"Resep tidak dapat dikoreksi", "Hanya resep yang sudah diterbitkan yang dapat digantikan oleh koreksi.",
	)
	ErrPrescriptionCancellationInvalidState = apiError(
		"PRESCRIPTION_CANCELLATION_STATE_CONFLICT", http.StatusConflict,
		"Prescription cannot be cancelled", "Only the currently issued prescription can be cancelled.",
		"Resep tidak dapat dibatalkan", "Hanya resep yang sedang berstatus terbit yang dapat dibatalkan.",
	)
	ErrPrescriptionConcurrentUpdate = apiError(
		"PRESCRIPTION_CONCURRENT_UPDATE", http.StatusConflict,
		"Prescription changed concurrently", "Reload the prescription because another update was completed first.",
		"Resep berubah secara bersamaan", "Muat ulang resep karena perubahan lain diselesaikan lebih dahulu.",
	)
	ErrPrescriptionPDFGenerationFailed = apiError(
		"PRESCRIPTION_PDF_GENERATION_FAILED", http.StatusInternalServerError,
		"Prescription PDF generation failed", "The prescription could not be issued because its PDF document could not be generated.",
		"Pembuatan PDF resep gagal", "Resep tidak dapat diterbitkan karena dokumen PDF-nya gagal dibuat.",
	)
	ErrPrescriptionVerificationInvalid = apiError(
		"PRESCRIPTION_VERIFICATION_INVALID", http.StatusNotFound,
		"Prescription verification invalid", "The verification token is invalid, cancelled, or has been superseded.",
		"Verifikasi resep tidak valid", "Token verifikasi tidak valid, resep dibatalkan, atau resep sudah digantikan revisi baru.",
	)
	ErrMedicationCatalogNotFound = apiError(
		"MEDICATION_CATALOG_NOT_FOUND", http.StatusNotFound,
		"Medication catalogue entry not found", "The medication is inactive, missing, or belongs to another hospital.",
		"Obat katalog tidak ditemukan", "Obat tidak aktif, tidak tersedia, atau berasal dari rumah sakit lain.",
	)
	ErrMedicationCatalogDuplicate = apiError(
		"MEDICATION_CATALOG_DUPLICATE", http.StatusConflict,
		"Medication catalogue code already exists", "The medication code is already used in this hospital catalogue.",
		"Kode katalog obat sudah digunakan", "Kode obat sudah digunakan dalam katalog rumah sakit ini.",
	)
)

// APIErrorCatalog exposes every static public error for contract tests and API
// documentation generation. Aliases are intentionally omitted.
func APIErrorCatalog() []response.CustomError {
	return []response.CustomError{
		ErrInternalServerError, ErrValidationError, ErrMalformedJSON, ErrRequestBodyRequired,
		ErrEndpointNotFound, ErrUnauthorized, ErrForbidden, ErrTooManyRequests,
		ErrPublicAuthRateLimitExceeded, ErrRegistrationPINCooldown,
		ErrRegistrationPINAttemptsExceeded, ErrLoginAttemptsExceeded,
		ErrPasswordResetRequestsExceeded, ErrPasswordResetPINAttemptsExceeded,
		ErrPasswordProcessingBusy, ErrEmailDeliveryBusy, ErrRequestTooLarge,
		ErrServiceUnavailable, ErrServiceNotReady, ErrInvalidEmail, ErrInvalidUsername, ErrInvalidPassword,
		ErrPasswordSimilarToUserInfo, ErrInvalidDateFormat, ErrInvalidUUIDFormat,
		ErrInvalidIdempotencyKey,
		ErrRecordNotFound, ErrUserNotFound, ErrEmailAlreadyExists, ErrUsernameAlreadyExists,
		ErrUsernameRegistrationInProgress, ErrDuplicateUsernameOrEmail, ErrDuplicateNIK,
		ErrConflict, ErrInvalidToken, ErrInvalidCredentials, ErrTokenExpired,
		ErrRegistrationPINInvalidOrExpired, ErrPasswordResetPINInvalidOrExpired,
		ErrInvalidResetToken, ErrEmailNotVerified, ErrEmailAlreadyActive, ErrEmailSendFailed,
		ErrInvalidRoleID, ErrRoleAlreadyExist, ErrRoleNotFound, ErrRoleNotAssigned,
		ErrRoleAlreadyAssigned, ErrRoleInUse, ErrOnlySuperAdmin, ErrDoctorRoleRequired,
		ErrSelfServicePatientRoleOnly, ErrAccountInactive, ErrHospitalMembershipRoleRequired,
		ErrAccountRoleNotFound,
		ErrUnauthorizedUpdate, ErrNewPasswordSame, ErrPasswordNotMatch, ErrProfileAlreadySet,
		ErrProfileUpdateEmpty, ErrProfilePhotoInvalid, ErrProfilePhotoNotFound,
		ErrRegistrationError, ErrHospitalNotFound, ErrHospitalContextRequired,
		ErrHospitalAdminRequired, ErrHospitalCodeAlreadyExists,
		ErrHospitalNameAlreadyExists, ErrHospitalAlreadyExists, ErrInvalidHospitalCoordinates,
		ErrInvalidHospitalFacilities, ErrUserNotLinkedToHospital, ErrDoctorNotEligible,
		ErrDoctorInvitationNotFound, ErrDoctorInvitationExists, ErrDoctorInvitationExpired,
		ErrInvalidDoctorInvitationState, ErrHospitalPlacementNotFound,
		ErrDepartmentAlreadyExists, ErrRoomAlreadyExists, ErrDoctorScheduleConflict,
		ErrInvalidContractPDF, ErrStorageUnavailable, ErrAffiliationNotFound,
		ErrNotificationNotFound, ErrScheduleNotFound, ErrScheduleChangeNotFound,
		ErrScheduleChangeExists, ErrScheduleChangeOwnApproval,
		ErrScheduleChangeHasAppointments, ErrInvalidScheduleChangeState,
		ErrAppointmentNotFound, ErrAppointmentSlotUnavailable, ErrAppointmentInvalidState,
		ErrAppointmentCutoffPassed, ErrAppointmentOutsideCheckInWindow,
		ErrAppointmentInvalidVerification, ErrCheckInLookupModeInvalid,
		ErrCheckInIdentityInsufficient, ErrAppointmentQRInvalid,
		ErrAppointmentVerificationCodeInvalid, ErrCheckInTokenInvalidOrExpired,
		ErrAppointmentAlreadyCheckedIn, ErrAppointmentCheckInExpired,
		ErrAppointmentLateOverrideReasonRequired, ErrWalkInPatientDataRequired,
		ErrWalkInPatientModeInvalid, ErrWalkInPatientNotFound,
		ErrWalkInPatientIdentityConflict,
		ErrWalkInCapacityFull, ErrWalkInCapacityOverrideForbidden,
		ErrWalkInCapacityOverrideReasonRequired, ErrPatientRecordNotFound,
		ErrPatientRecordAlreadyClaimed, ErrPatientRecordIdentityMismatch,
		ErrAppointmentConsentRequired,
		ErrIdempotencyKeyRequired, ErrIdempotencyConflict,
		ErrMedicalEncounterNotFound, ErrExaminationInvalidState,
		ErrVitalsDraftNotFound, ErrVitalsRevisionNotFound, ErrVitalsContentRequired, ErrBloodPressureInvalid,
		ErrClinicalSOAPRequired, ErrConsultationDraftNotFound, ErrConsultationRevisionNotFound, ErrDiagnosisRequired,
		ErrMedicalAttachmentInvalid, ErrMedicalAttachmentNotFound,
		ErrPrescriptionNotFound, ErrPrescriptionNotAvailable, ErrPrescriptionInvalidState,
		ErrPrescriptionPrimaryDiagnosisRequired, ErrPrescriptionItemsRequired,
		ErrPrescriptionDoctorSIPRequired,
		ErrPrescriptionScheduleRequired, ErrPrescriptionCompoundInvalid,
		ErrControlledMedicationUnsupported, ErrPrescriptionDraftRequired,
		ErrPrescriptionAllergyReviewRequired, ErrPrescriptionDecisionRequired,
		ErrPrescriptionCorrectionInvalidState, ErrPrescriptionCancellationInvalidState,
		ErrPrescriptionConcurrentUpdate, ErrPrescriptionPDFGenerationFailed,
		ErrPrescriptionVerificationInvalid, ErrMedicationCatalogNotFound,
		ErrMedicationCatalogDuplicate,
	}
}
