# Repository

Repository aktif adalah `user`, `role`, dan `hospital`. Seluruhnya memakai pool GORM yang sama. Operasi lintas user/membership/role menggunakan repository hasil `WithTx` agar satu workflow memakai transaksi yang sama.

Query otorisasi wajib memfilter user aktif/tidak dihapus, role aktif/tidak dihapus, hospital aktif/tidak dihapus, dan membership aktif/tidak dihapus.
