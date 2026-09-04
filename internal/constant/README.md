# Constants

Folder ini memusatkan context key, role slug, permission slug, pesan sukses,
dan `CustomError` API.

Aturan error publik:

- `message` berisi kode stabil `UPPER_SNAKE_CASE`, bukan kalimat untuk UI.
- `message_detail` wajib mempunyai judul dan deskripsi dalam bahasa Inggris
  serta Indonesia.
- Satu kondisi bisnis menggunakan satu kode yang spesifik. Jangan mengubah
  `BaseResponse.Message` secara manual di handler atau middleware.
- Status HTTP dan error statis baru harus ditambahkan melalui `apiError` di
  `custom_error.constant.go`, lalu didaftarkan pada `APIErrorCatalog`.
- Error validasi field dibuat melalui helper `New*Error` agar nama field ikut
  disebut tanpa menciptakan kode baru untuk setiap DTO.

Aturan response sukses:

- Setiap outcome endpoint menggunakan `MessageCode` yang spesifik dari
  `message.constant.go`; jangan gunakan `SUCCESS` generik pada handler baru.
- Buat response melalui `NewSuccessResponse(code)` agar kode dan detail
  bilingual selalu berasal dari katalog yang sama.
- Retry idempoten menggunakan kode berbeda dari operasi yang membuat resource
  baru.

Test katalog akan gagal jika ada kode duplikat, format kode tidak standar,
status HTTP tidak valid, atau terjemahan tidak lengkap.
