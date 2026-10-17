package constant

import "net/http"

const MsgSpecialtiesListed MessageCode = "SPECIALTIES_LISTED"

var ErrMasterSpecialtyNotFound = apiError("MASTER_SPECIALTY_NOT_FOUND", http.StatusNotFound,
	"Specialty not found", "Select an available active specialty from the catalogue.",
	"Spesialisasi tidak ditemukan", "Pilih spesialisasi aktif yang tersedia dari katalog.")
