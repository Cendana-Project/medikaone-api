package constant

import "github.com/Cendana-Project/medikaone-api/internal/model/response"

type MessageCode string

const (
	MsgSuccess                   MessageCode = "SUCCESS"
	MsgFailedInquiryMiles        MessageCode = "FAILED_INQUIRY_MILES"
	MsgUnauthorized              MessageCode = "UNAUTHORIZED"
	MsgInvalidToken              MessageCode = "INVALID_TOKEN"
	MsgTokenExpired              MessageCode = "TOKEN_EXPIRED"
	MsgNoTokenProvided           MessageCode = "NO_TOKEN_PROVIDED"
	MsgInvalidTokenFormat        MessageCode = "INVALID_TOKEN_FORMAT"
	MsgTokenValidationFailed     MessageCode = "TOKEN_VALIDATION_FAILED"
	MsgValidationError           MessageCode = "VALIDATION_ERROR"
	MsgInternalServerError       MessageCode = "INTERNAL_SERVER_ERROR"
	MsgEndpointNotFound          MessageCode = "ENDPOINT_NOT_FOUND"
	MsgInvalidUUIDFormat         MessageCode = "INVALID_UUID_FORMAT"
	MsgRecordNotFound            MessageCode = "RECORD_NOT_FOUND"
	MsgInvalidRoleID             MessageCode = "INVALID_ROLE_ID"
	MsgRoleAlreadyExist          MessageCode = "ROLE_ALREADY_EXIST"
	MsgRoleNotFound              MessageCode = "ROLE_NOT_FOUND"
	MsgRoleNotAssigned           MessageCode = "ROLE_NOT_ASSIGNED"
	MsgRoleAlreadyAssigned       MessageCode = "ROLE_ALREADY_ASSIGNED"
	MsgRoleInUse                 MessageCode = "ROLE_IN_USE"
	MsgFieldNotFound             MessageCode = "FIELD_NOT_FOUND"
	MsgBuildingNotFound          MessageCode = "BUILDING_NOT_FOUND"
	MsgPhotoNotFound             MessageCode = "PHOTO_NOT_FOUND"
	MsgInvalidBuildingID         MessageCode = "INVALID_BUILDING_ID"
	MsgInvalidOwnerID            MessageCode = "INVALID_OWNER_ID"
	MsgInvalidUserID             MessageCode = "INVALID_USER_ID"
	MsgInvalidStartTime          MessageCode = "INVALID_START_TIME"
	MsgInvalidEndTime            MessageCode = "INVALID_END_TIME"
	MsgDateRequired              MessageCode = "DATE_REQUIRED"
	MsgInvalidDateFormatYYYYMMDD MessageCode = "INVALID_DATE_FORMAT_YYYYMMDD"
	// Domain-specific additions
	MsgReviewAlreadyExists      MessageCode = "REVIEW_ALREADY_EXISTS"
	MsgReviewNotEligible        MessageCode = "REVIEW_NOT_ELIGIBLE"
	MsgPhotoURLRequired         MessageCode = "PHOTO_URL_REQUIRED"
	MsgFieldHourAlreadyExists   MessageCode = "FIELD_HOUR_ALREADY_EXISTS"
	MsgSlotAlreadyExists        MessageCode = "SLOT_ALREADY_EXISTS"
	MsgNoSlotsCreated           MessageCode = "NO_SLOTS_CREATED"
	MsgInvalidFacility          MessageCode = "INVALID_FACILITY"
	MsgUserNotAuthenticated     MessageCode = "USER_NOT_AUTHENTICATED"
	MsgValidationFailed         MessageCode = "VALIDATION_FAILED"
	MsgUserUpdatedSuccess       MessageCode = "USER_UPDATED_SUCCESS"
	MsgMatchEventNotFound       MessageCode = "MATCH_EVENT_NOT_FOUND"
	MsgOnlySuperAdmin           MessageCode = "ONLY_SUPER_ADMIN"
	MsgMatchEventNotCancellable MessageCode = "MATCH_EVENT_NOT_CANCELLABLE"
	MsgUserNotFound             MessageCode = "USER_NOT_FOUND"
	MsgEmailNotVerified         MessageCode = "EMAIL_NOT_VERIFIED"
	MsgPasswordNotMatch         MessageCode = "PASSWORD_NOT_MATCH"
	MsgForbidden                MessageCode = "FORBIDDEN"
	MsgConflict                 MessageCode = "CONFLICT"
	MsgNotFound                 MessageCode = "NOT_FOUND"
)

var MessageCatalog = map[MessageCode]response.MessageDetail{
	MsgSuccess:                   {TitleEng: "SUCCESS", DescEng: "Operation completed successfully", TitleIdn: "SUKSES", DescIdn: "Operasi berhasil diselesaikan"},
	MsgFailedInquiryMiles:        {TitleEng: "Failed Inquiry Miles", DescEng: "Unable to retrieve miles information", TitleIdn: "Gagal Inquiry Miles", DescIdn: "Tidak dapat mengambil informasi miles"},
	MsgUnauthorized:              {TitleEng: "Unauthorized", DescEng: "You do not have permission to access this resource", TitleIdn: "Tidak Diizinkan", DescIdn: "Anda tidak memiliki izin untuk mengakses resource ini"},
	MsgInvalidToken:              {TitleEng: "Invalid token", DescEng: "The provided token is not valid", TitleIdn: "Token tidak valid", DescIdn: "Token yang diberikan tidak valid"},
	MsgTokenExpired:              {TitleEng: "Token has expired", DescEng: "Please login again to get a new token", TitleIdn: "Token kadaluarsa", DescIdn: "Silakan login kembali untuk mendapatkan token baru"},
	MsgNoTokenProvided:           {TitleEng: "No token provided", DescEng: "Authentication token is required", TitleIdn: "Token tidak tersedia", DescIdn: "Token autentikasi diperlukan"},
	MsgInvalidTokenFormat:        {TitleEng: "Invalid token format", DescEng: "Token format is not recognized", TitleIdn: "Format token tidak valid", DescIdn: "Format token tidak dikenali"},
	MsgTokenValidationFailed:     {TitleEng: "Token validation failed", DescEng: "Unable to validate the provided token", TitleIdn: "Validasi token gagal", DescIdn: "Tidak dapat memvalidasi token yang diberikan"},
	MsgValidationError:           {TitleEng: "Validation Error", DescEng: "The provided data does not meet validation requirements", TitleIdn: "Kesalahan Validasi", DescIdn: "Data yang diberikan tidak memenuhi persyaratan validasi"},
	MsgInternalServerError:       {TitleEng: "Internal Server Error", DescEng: "An unexpected error occurred on the server", TitleIdn: "Kesalahan Server Internal", DescIdn: "Terjadi kesalahan tak terduga pada server"},
	MsgEndpointNotFound:          {TitleEng: "Endpoint Not Found", DescEng: "The requested endpoint does not exist", TitleIdn: "Endpoint Tidak Ditemukan", DescIdn: "Endpoint yang diminta tidak ada"},
	MsgInvalidUUIDFormat:         {TitleEng: "Invalid UUID Format", DescEng: "The provided UUID format is not valid", TitleIdn: "Format UUID Tidak Valid", DescIdn: "Format UUID yang diberikan tidak valid"},
	MsgRecordNotFound:            {TitleEng: "Record Not Found", DescEng: "The requested record could not be found", TitleIdn: "Data Tidak Ditemukan", DescIdn: "Data yang diminta tidak dapat ditemukan"},
	MsgInvalidRoleID:             {TitleEng: "Invalid Role ID", DescEng: "The provided role ID is not valid", TitleIdn: "Role ID Tidak Valid", DescIdn: "Role ID yang diberikan tidak valid"},
	MsgRoleAlreadyExist:          {TitleEng: "Role Already Exists", DescEng: "A role with this name already exists", TitleIdn: "Role Sudah Ada", DescIdn: "Role dengan nama ini sudah ada"},
	MsgRoleNotFound:              {TitleEng: "Role Not Found", DescEng: "The requested role could not be found", TitleIdn: "Role Tidak Ditemukan", DescIdn: "Role yang diminta tidak dapat ditemukan"},
	MsgRoleNotAssigned:           {TitleEng: "Role Not Assigned", DescEng: "This role is not assigned to any user", TitleIdn: "Role Tidak Terpasang", DescIdn: "Role ini tidak ditugaskan ke user manapun"},
	MsgRoleAlreadyAssigned:       {TitleEng: "Role Already Assigned", DescEng: "This role is already assigned to the user", TitleIdn: "Role Sudah Terpasang", DescIdn: "Role ini sudah ditugaskan ke user"},
	MsgRoleInUse:                 {TitleEng: "Role In Use", DescEng: "Cannot delete role that is currently in use", TitleIdn: "Role Sedang Digunakan", DescIdn: "Tidak dapat menghapus role yang sedang digunakan"},
	MsgFieldNotFound:             {TitleEng: "Field Not Found", DescEng: "The requested field could not be found", TitleIdn: "Lapangan Tidak Ditemukan", DescIdn: "Lapangan yang diminta tidak dapat ditemukan"},
	MsgBuildingNotFound:          {TitleEng: "Building Not Found", DescEng: "The requested building could not be found", TitleIdn: "Gedung Tidak Ditemukan", DescIdn: "Gedung yang diminta tidak dapat ditemukan"},
	MsgPhotoNotFound:             {TitleEng: "Photo Not Found", DescEng: "The requested photo could not be found", TitleIdn: "Foto Tidak Ditemukan", DescIdn: "Foto yang diminta tidak dapat ditemukan"},
	MsgInvalidBuildingID:         {TitleEng: "Invalid Building ID", DescEng: "The provided building ID is not valid", TitleIdn: "Building ID Tidak Valid", DescIdn: "Building ID yang diberikan tidak valid"},
	MsgInvalidOwnerID:            {TitleEng: "Invalid Owner ID", DescEng: "The provided owner ID is not valid", TitleIdn: "Owner ID Tidak Valid", DescIdn: "Owner ID yang diberikan tidak valid"},
	MsgInvalidUserID:             {TitleEng: "Invalid User ID", DescEng: "The provided user ID is not valid", TitleIdn: "User ID Tidak Valid", DescIdn: "User ID yang diberikan tidak valid"},
	MsgInvalidStartTime:          {TitleEng: "Invalid Start Time", DescEng: "The provided start time is not valid", TitleIdn: "Waktu Mulai Tidak Valid", DescIdn: "Waktu mulai yang diberikan tidak valid"},
	MsgInvalidEndTime:            {TitleEng: "Invalid End Time", DescEng: "The provided end time is not valid", TitleIdn: "Waktu Selesai Tidak Valid", DescIdn: "Waktu selesai yang diberikan tidak valid"},
	MsgDateRequired:              {TitleEng: "Date Required", DescEng: "Date field is required for this operation", TitleIdn: "Tanggal Diperlukan", DescIdn: "Field tanggal diperlukan untuk operasi ini"},
	MsgInvalidDateFormatYYYYMMDD: {TitleEng: "Invalid Date Format", DescEng: "Date must be in YYYY-MM-DD format", TitleIdn: "Format Tanggal Tidak Valid", DescIdn: "Tanggal harus dalam format YYYY-MM-DD"},
	MsgReviewAlreadyExists:       {TitleEng: "Review Already Exists", DescEng: "You have already reviewed this item", TitleIdn: "Ulasan sudah ada", DescIdn: "Anda sudah mengulas item ini"},
	MsgReviewNotEligible:         {TitleEng: "Not Eligible to Review", DescEng: "You are not eligible to review this item", TitleIdn: "Tidak memenuhi syarat untuk mengulas", DescIdn: "Anda tidak memenuhi syarat untuk mengulas item ini"},
	MsgPhotoURLRequired:          {TitleEng: "Photo URL Required", DescEng: "Photo URL is mandatory for this operation", TitleIdn: "URL foto wajib diisi", DescIdn: "URL foto wajib diisi untuk operasi ini"},
	MsgFieldHourAlreadyExists:    {TitleEng: "Field Hour Already Exists", DescEng: "Field hour schedule already exists for this time", TitleIdn: "Jadwal lapangan sudah ada", DescIdn: "Jadwal lapangan sudah ada untuk waktu ini"},
	MsgSlotAlreadyExists:         {TitleEng: "Time Slot Already Exists", DescEng: "Time slot already exists for this period", TitleIdn: "Slot waktu sudah ada", DescIdn: "Slot waktu sudah ada untuk periode ini"},
	MsgNoSlotsCreated:            {TitleEng: "No Slots Created", DescEng: "No time slots were created for this event", TitleIdn: "Tidak ada slot yang dibuat", DescIdn: "Tidak ada slot waktu yang dibuat untuk event ini"},
	MsgInvalidFacility:           {TitleEng: "Invalid Facility", DescEng: "The provided facility is not valid", TitleIdn: "Fasilitas tidak valid", DescIdn: "Fasilitas yang diberikan tidak valid"},
	MsgUserNotAuthenticated:      {TitleEng: "User Not Authenticated", DescEng: "User authentication is required", TitleIdn: "User tidak terautentikasi", DescIdn: "Autentikasi user diperlukan"},
	MsgValidationFailed:          {TitleEng: "Validation Failed", DescEng: "Input validation failed", TitleIdn: "Validasi gagal", DescIdn: "Validasi input gagal"},
	MsgUserUpdatedSuccess:        {TitleEng: "User Updated Successfully", DescEng: "User information has been updated", TitleIdn: "User berhasil diperbarui", DescIdn: "Informasi user telah diperbarui"},
	MsgMatchEventNotFound:        {TitleEng: "Match Event Not Found", DescEng: "The requested match event could not be found", TitleIdn: "Event pertandingan tidak ditemukan", DescIdn: "Event pertandingan yang diminta tidak dapat ditemukan"},
	MsgOnlySuperAdmin:            {TitleEng: "Only Super Admin", DescEng: "This action requires super admin privileges", TitleIdn: "Hanya Super Admin", DescIdn: "Aksi ini memerlukan hak akses super admin"},
	MsgMatchEventNotCancellable:  {TitleEng: "Match Event Not Cancellable", DescEng: "This match event cannot be cancelled", TitleIdn: "Event pertandingan tidak dapat dibatalkan", DescIdn: "Event pertandingan ini tidak dapat dibatalkan"},
	MsgUserNotFound:              {TitleEng: "User Not Found", DescEng: "The requested user could not be found", TitleIdn: "User Tidak Ditemukan", DescIdn: "User yang diminta tidak dapat ditemukan"},
	MsgEmailNotVerified:          {TitleEng: "Email Not Verified", DescEng: "Please verify your email address", TitleIdn: "Email Belum Diverifikasi", DescIdn: "Silakan verifikasi alamat email Anda"},
	MsgPasswordNotMatch:          {TitleEng: "Password Not Match", DescEng: "The provided password is incorrect", TitleIdn: "Password Tidak Cocok", DescIdn: "Password yang diberikan tidak benar"},
	MsgForbidden: {
		TitleEng: "Forbidden",
		DescEng:  "You do not have access to this resource",
		TitleIdn: "Dilarang",
		DescIdn:  "Anda tidak memiliki akses ke resource ini",
	},
	MsgConflict: {
		TitleEng: "Conflict",
		DescEng:  "The request conflicts with the current state of the resource",
		TitleIdn: "Konflik",
		DescIdn:  "Permintaan bertentangan dengan kondisi resource saat ini",
	},
	MsgNotFound: {
		TitleEng: "Not Found",
		DescEng:  "The requested resource could not be found",
		TitleIdn: "Tidak Ditemukan",
		DescIdn:  "Resource yang diminta tidak ditemukan",
	},
}

func GetMessageDetail(code MessageCode) response.MessageDetail {
	if d, ok := MessageCatalog[code]; ok {
		return d
	}
	return response.MessageDetail{}
}
