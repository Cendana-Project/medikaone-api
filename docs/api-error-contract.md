# Kontrak Pesan API

Semua response MedikaOne memakai `message` sebagai kode mesin yang stabil dan
`message_detail` sebagai teks bilingual untuk UI. Kode sukses dibuat spesifik
untuk setiap outcome endpoint; kode error dibuat spesifik untuk setiap kondisi
yang dapat ditindaklanjuti client.

## Response sukses

```json
{
  "message": "HOSPITAL_CREATED",
  "message_detail": {
    "title_eng": "Hospital created",
    "desc_eng": "Hospital created.",
    "title_idn": "Rumah sakit berhasil dibuat",
    "desc_idn": "Rumah sakit berhasil dibuat."
  },
  "data": {"id": "..."},
  "trace_id": "4d4f8638-1e52-4a67-9886-ab2f33a8c053",
  "timestamp": "2026-09-04T00:00:00Z"
}
```

Hasil retry idempoten mempunyai kode berbeda, misalnya
`APPOINTMENT_CREATION_REPLAYED`, sehingga client dapat membedakannya dari
resource yang benar-benar baru dibuat.

## Response error

Semua endpoint MedikaOne menggunakan bentuk error yang sama:

```json
{
  "message": "FIELD_REQUIRED",
  "message_detail": {
    "title_eng": "Required field",
    "desc_eng": "The \"challenge_id\" field is required.",
    "title_idn": "Field wajib diisi",
    "desc_idn": "Field \"challenge_id\" wajib diisi."
  },
  "data": null,
  "trace_id": "4d4f8638-1e52-4a67-9886-ab2f33a8c053",
  "timestamp": "2026-09-04T00:00:00Z"
}
```

## Aturan client

- Gunakan `message` untuk branching program karena nilainya merupakan kode
  stabil `UPPER_SNAKE_CASE`.
- Tampilkan `message_detail.desc_idn` atau `desc_eng` kepada pengguna sesuai
  locale aplikasi.
- Simpan `trace_id` saat melaporkan masalah ke backend.
- Jangan menentukan jenis error hanya berdasarkan status HTTP; satu status
  dapat mencakup beberapa kasus yang berbeda.

## Kelompok status

- `400`: body, format, field, PIN, token reset, atau consent tidak valid.
- `401`: kredensial atau token autentikasi tidak valid.
- `403`: akun terautentikasi tetapi tidak mempunyai izin.
- `404`: resource yang diminta tidak ditemukan.
- `409`: data duplikat atau state resource tidak mengizinkan aksi.
- `413`: file atau body request melebihi batas ukuran yang diizinkan.
- `429`: cooldown atau batas percobaan/permintaan terlampaui.
- `500`: kesalahan internal yang tidak boleh membocorkan detail implementasi.
- `502`/`503`: dependency atau kapasitas layanan sementara tidak tersedia.

## Contoh kode auth yang perlu ditangani client

| Kode | Tindakan client |
| --- | --- |
| `REGISTRATION_PIN_INVALID_OR_EXPIRED` | Minta pengguna mengecek data atau memulai registrasi baru. |
| `REGISTRATION_PIN_ATTEMPTS_EXCEEDED` | Hapus challenge lokal dan mulai registrasi baru. |
| `REGISTRATION_PIN_RESEND_COOLDOWN` | Nonaktifkan tombol resend sementara. |
| `LOGIN_ATTEMPTS_EXCEEDED` | Hentikan retry otomatis dan tunggu cooldown. |
| `PASSWORD_RESET_REQUEST_LIMIT_EXCEEDED` | Hentikan permintaan PIN reset sementara. |
| `PASSWORD_RESET_PIN_ATTEMPTS_EXCEEDED` | Hapus challenge reset dan minta PIN baru. |
| `PASSWORD_PROCESSING_BUSY` | Retry dengan backoff singkat. |
| `EMAIL_DELIVERY_BUSY` | Retry pengiriman email dengan backoff singkat. |

## Kode upload dan konflik jadwal

| Kode | HTTP | Tindakan client |
| --- | --- | --- |
| `FILE_TOO_LARGE` | 413 | Pilih PDF yang lebih kecil. Pesan menyertakan batas byte aplikasi jika diketahui; batas provider storage juga dipetakan ke kode ini. |
| `REQUEST_TOO_LARGE` | 413 | Kurangi ukuran keseluruhan request multipart/body. Ukuran file bukan satu-satunya bagian body. |
| `INVALID_CONTRACT_PDF` | 400 | Kirim file PDF yang valid; nama/ekstensi PDF saja tidak cukup. |
| `DOCTOR_SCHEDULE_CONFLICT` | 409 | Pilih waktu lain atau tinjau jadwal aktif dan proposal yang masih pending; pengajuan konflik tidak dibuat. |
| `DOCTOR_SCHEDULE_NOT_FOUND` | 404 | Muat ulang jadwal aktif; scope penonaktifan tidak mempunyai target yang tersedia. |
| `SCHEDULE_CHANGE_ALREADY_PENDING` | 409 | Selesaikan pengajuan yang berbenturan terlebih dahulu. Deactivate ALL berbenturan dengan semua pengajuan pada afiliasi itu; penonaktifan satu hari berbenturan dengan REPLACE atau removal/deactivation hari yang sama. |
| `SCHEDULE_CHANGE_COUNTERPART_REVIEW_REQUIRED` | 403 | Pengajuan dokter harus ditinjau rumah sakit dan sebaliknya. |
| `SCHEDULE_CHANGE_HAS_ACTIVE_APPOINTMENTS` | 409 | Batalkan/reschedule appointment aktif pada jadwal yang ditargetkan sebelum approval; seluruh pengajuan tetap PENDING tanpa perubahan parsial. |
| `SCHEDULE_CHANGE_STATE_CONFLICT` | 409 | Muat ulang status pengajuan dan jadwal. Snapshot deactivation yang targetnya sudah nonaktif/tidak sesuai tidak dapat disetujui. |

Kontrak menerima maksimal `SUPABASE_STORAGE_MAX_FILE_SIZE_BYTES`, dibatasi
paling tinggi 10 MiB (10.485.760 byte). Body multipart mempunyai tambahan
anggaran 64 KiB untuk field dan metadata form. File tepat pada batas file tetap
valid jika metadata masih dalam anggaran. JSON tetap dibatasi 10 MiB.
Kegagalan approval karena konflik tetap mempertahankan proposal sebagai PENDING.

## Kode check-in dan walk-in

| Kode | Tindakan client |
| --- | --- |
| `CHECK_IN_LOOKUP_MODE_INVALID` | Kirim tepat satu mode: QR, nomor+kode, atau identitas. |
| `CHECK_IN_IDENTITY_INSUFFICIENT` | Minta minimal dua fakta; nama harus bersama tanggal lahir. |
| `APPOINTMENT_VERIFICATION_CODE_INVALID` | Periksa kembali kode milik appointment. |
| `APPOINTMENT_QR_INVALID` | Minta pasien membuka ulang detail appointment atau gunakan lookup manual. |
| `CHECK_IN_TOKEN_INVALID_OR_EXPIRED` | Ulangi lookup; grant hanya berlaku lima menit dan terikat pada petugas. |
| `APPOINTMENT_LATE_OVERRIDE_REASON_REQUIRED` | Tampilkan input alasan sebelum konfirmasi terlambat/NO_SHOW. |
| `APPOINTMENT_CHECK_IN_EXPIRED` | Tolak check-in karena tanggal appointment sudah lewat. |
| `APPOINTMENT_ALREADY_CHECKED_IN` | Muat antrean/detail terkini; jangan mengulang mutasi. |
| `WALK_IN_CAPACITY_FULL` | Pilih slot/sesi lain atau eskalasi keputusan ke admin. |
| `WALK_IN_PATIENT_MODE_INVALID` | Kirim tepat satu cara memilih pasien; jangan mencampur ID dan identitas lengkap. |
| `WALK_IN_PATIENT_NOT_FOUND` | Periksa kembali patient record ID atau MedikaOne ID pasien. |
| `WALK_IN_PATIENT_IDENTITY_CONFLICT` | Jangan menimpa data; eskalasi verifikasi karena nomor identitas memiliki tanggal lahir berbeda. |
| `WALK_IN_CAPACITY_OVERRIDE_FORBIDDEN` | Login sebagai admin; resepsionis tidak boleh override. |
| `WALK_IN_CAPACITY_OVERRIDE_REASON_REQUIRED` | Admin harus memberikan alasan audit. |
| `PATIENT_RECORD_NOT_FOUND` | Periksa identitas dan tanggal lahir yang dimasukkan pasien. |
| `PATIENT_RECORD_ALREADY_CLAIMED` | Hentikan klaim mandiri dan eskalasi verifikasi kepemilikan. |
| `PATIENT_RECORD_IDENTITY_MISMATCH` | Samakan profil akun dengan identitas record melalui proses terverifikasi. |

## Kode resep dan katalog obat

| Kode | Tindakan client |
| --- | --- |
| `PRESCRIPTION_PRIMARY_DIAGNOSIS_REQUIRED` | Simpan diagnosis utama pada draft konsultasi sebelum membuka resep. |
| `PRESCRIPTION_DOCTOR_SIP_REQUIRED` | Lengkapi SIP pada profil dokter sebelum membuat atau menerbitkan resep. |
| `PRESCRIPTION_ITEMS_REQUIRED` | Tambahkan minimal satu item obat atau gunakan flow `NO_MEDICATION`. |
| `PRESCRIPTION_ALLERGY_REVIEW_REQUIRED` | Tampilkan alergi pasien dan minta dokter mengonfirmasi peninjauan. |
| `PRESCRIPTION_DECISION_REQUIRED` | Terbitkan resep atau catat `NO_MEDICATION` sebelum complete examination. |
| `PRESCRIPTION_COMPOUND_INVALID` | Cocokkan komponen dengan tipe item racikan/non-racikan. |
| `CONTROLLED_MEDICATION_UNSUPPORTED` | Hentikan flow elektronik dan ikuti prosedur khusus rumah sakit. |
| `PRESCRIPTION_CONCURRENT_UPDATE` | Muat ulang resep sebelum mengulangi koreksi/issue. |
| `PRESCRIPTION_VERIFICATION_INVALID` | Jangan gunakan dokumen; token salah, dibatalkan, atau sudah digantikan. |
| `MEDICATION_CATALOG_NOT_FOUND` | Muat ulang katalog aktif rumah sakit. |
| `MEDICATION_CATALOG_DUPLICATE` | Gunakan kode katalog yang berbeda. |

## Kode master department/poli

| Kode | Tindakan client |
| --- | --- |
| `MASTER_DEPARTMENT_NOT_FOUND` | Muat ulang `GET /v1/departments` lalu minta admin memilih master poli aktif. |
| `DEPARTMENT_ALREADY_EXISTS` | Gunakan department rumah sakit yang sudah ada; jangan membuat master yang sama dua kali. |
| `HOSPITAL_PLACEMENT_NOT_FOUND` | Muat ulang department/room milik rumah sakit sebelum membuat invitation atau placement. |

Pesan sukses didefinisikan di `internal/constant/message.constant.go`. Error
statis didefinisikan di `internal/constant/custom_error.constant.go` dan
dirender hanya melalui `internal/util/error.go`.

Filter lokasi pada `GET /v1/doctors` dan `GET /v1/recommendations/doctors`
menggunakan HTTP 400 `INVALID_FIELD_VALUE` untuk koordinat tidak berpasangan,
angka tidak valid/nonfinite, latitude di luar -90..90, longitude di luar
-180..180, atau radius di luar (0, 5000] km/tanpa koordinat. Parameter lokasi
kosong atau berulang juga ditolak. Client harus memperbaiki input sebelum retry.
