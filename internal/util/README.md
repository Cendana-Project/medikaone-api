# Util

- `bind.go` melakukan strict JSON decoding dan membedakan body kosong, JSON
  rusak, field asing, multiple JSON values, serta tipe field yang salah.
- `validation.go` mendaftarkan validator DTO.
- `password.go` menerapkan kebijakan password.
- `error.go` dan `common.go` menjaga kode mesin, pesan bilingual, trace ID,
  timestamp, dan status HTTP tetap konsisten.
- `log.go` menambahkan request/user context ke log tanpa mencetak credential.
