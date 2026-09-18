package constant

import "net/http"

const (
	MsgHospitalImagesListed    MessageCode = "HOSPITAL_IMAGES_LISTED"
	MsgHospitalImageUploaded   MessageCode = "HOSPITAL_IMAGE_UPLOADED"
	MsgHospitalImageUpdated    MessageCode = "HOSPITAL_IMAGE_UPDATED"
	MsgHospitalImageDeleted    MessageCode = "HOSPITAL_IMAGE_DELETED"
	MsgHospitalReviewsListed   MessageCode = "HOSPITAL_REVIEWS_LISTED"
	MsgHospitalReviewRetrieved MessageCode = "HOSPITAL_REVIEW_RETRIEVED"
	MsgHospitalReviewSaved     MessageCode = "HOSPITAL_REVIEW_SAVED"
	MsgHospitalReviewDeleted   MessageCode = "HOSPITAL_REVIEW_DELETED"
)

var (
	ErrHospitalImageInvalid        = apiError("HOSPITAL_IMAGE_INVALID", http.StatusBadRequest, "Invalid hospital image", "Use a JPEG or PNG up to 10 MB and 4096 by 4096 pixels.", "Foto rumah sakit tidak valid", "Gunakan JPEG atau PNG maksimal 10 MB dan 4096 x 4096 piksel.")
	ErrHospitalImageNotFound       = apiError("HOSPITAL_IMAGE_NOT_FOUND", http.StatusNotFound, "Hospital image not found", "The image is unavailable for this hospital.", "Foto rumah sakit tidak ditemukan", "Foto tidak tersedia pada rumah sakit ini.")
	ErrHospitalGalleryFull         = apiError("HOSPITAL_GALLERY_FULL", http.StatusConflict, "Hospital gallery is full", "A hospital can have at most 20 images.", "Galeri rumah sakit penuh", "Rumah sakit dapat memiliki maksimal 20 foto.")
	ErrHospitalReviewNotFound      = apiError("HOSPITAL_REVIEW_NOT_FOUND", http.StatusNotFound, "Hospital review not found", "No active review was found for your account at this hospital.", "Ulasan rumah sakit tidak ditemukan", "Akun Anda belum memiliki ulasan aktif di rumah sakit ini.")
	ErrHospitalReviewVisitRequired = apiError("HOSPITAL_REVIEW_VISIT_REQUIRED", http.StatusForbidden, "Completed visit required", "Only verified patients with a completed appointment at this hospital can review it.", "Kunjungan selesai diperlukan", "Hanya pasien terverifikasi dengan appointment selesai di rumah sakit ini yang dapat memberi ulasan.")
)
