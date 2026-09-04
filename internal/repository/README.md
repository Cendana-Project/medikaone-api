# Repository

Repository aktif adalah `user`, `role`, `hospital`, `doctor_hospital`, dan
`appointment`. Seluruhnya memakai pool GORM yang sama. Operasi lintas
user/membership/role menggunakan repository hasil `WithTx` agar satu workflow
memakai transaksi yang sama.

Repository appointment memakai advisory lock dan transaksi untuk alokasi slot,
nomor appointment, nomor antrean, walk-in, check-in, serta klaim patient record.
Jangan memindahkan validasi kapasitas atau perubahan status ke query terpisah
tanpa lock karena dapat menyebabkan double booking.

Query otorisasi wajib memfilter user aktif/tidak dihapus, role aktif/tidak dihapus, hospital aktif/tidak dihapus, dan membership aktif/tidak dihapus.
