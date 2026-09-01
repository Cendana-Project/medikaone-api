# Configuration

`config.go` membaca `config.yml` opsional dan environment variables, menerapkan default yang aman, lalu memvalidasi konfigurasi sebelum sebuah command dijalankan. Nama environment variable mengikuti key YAML dalam uppercase/underscore; binding lengkap berada di `bindEnvVariables`.

Konfigurasi contoh resmi berada di [`../../config.yml.example`](../../config.yml.example). Jangan menambahkan secret atau provider yang tidak dipakai aplikasi ke struktur konfigurasi.
