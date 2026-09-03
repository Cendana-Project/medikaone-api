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
- `413`: request melebihi batas 10 MB.
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

Pesan sukses didefinisikan di `internal/constant/message.constant.go`. Error
statis didefinisikan di `internal/constant/custom_error.constant.go` dan
dirender hanya melalui `internal/util/error.go`.
