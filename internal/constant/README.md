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

Test katalog akan gagal jika ada kode duplikat, format kode tidak standar,
status HTTP tidak valid, atau terjemahan tidak lengkap.
