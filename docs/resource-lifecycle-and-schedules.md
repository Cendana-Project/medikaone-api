# Direktori, lifecycle resource, dan jadwal dokter

Migration terbaru: `20260926100000_schedule_conflict_guard.sql`. Jalankan seluruh
migration pending melalui command migration terpisah sebelum deploy API.
Koleksi Bruno berada pada repository terpisah `Cendana-Project/bruno-medikaone`.

Konflik jadwal diperiksa memakai interval absolut untuk seluruh afiliasi aktif
dokter, termasuk rumah sakit lain. WIB, WITA, dan WIT dibandingkan sebagai UTC;
misalnya Senin 08:00 Asia/Jakarta bertabrakan dengan Senin 09:00 Asia/Makassar
pada durasi yang sama. Jadwal sekali memakai offset IANA pada tanggalnya. Jadwal
rutin yang menggunakan dua timezone berbeda dengan perubahan offset/DST ditolak
secara konservatif. Pemeriksaan diulang dalam transaksi yang dikunci per dokter
saat invite diterima, perubahan jadwal disetujui, dan afiliasi diaktifkan kembali.
Trigger database menolak insert/update aktif yang mencoba melewati jalur API.

## Identitas dokter dan direktori publik

`doctor_id` tetap UUID akun untuk relasi dan endpoint operasional. Field tambahan
`doctor_medikaone_id` berformat `MDO-` diikuti 16 karakter heksadesimal uppercase,
unik, immutable, dan dibuat otomatis untuk profil dokter lama maupun baru.
Respons yang membawa dokter menyertakan field ini; profil akun menggunakan
`doctor_profile.doctor_medikaone_id`, sedangkan hasil set-profile menaruhnya di
`profile.doctor_medikaone_id`. ID ini bukan pengganti SIP dan bukan credential.

PDF resep yang sudah tersimpan tidak diregenerasi otomatis setelah migration.
Respons JSON verifikasi resep tetap menyertakan MedikaOne ID dokter; PDF yang
diterbitkan sesudah pembaruan menggunakan format baru yang memuat ID tersebut.

| Method / path | Perilaku |
| --- | --- |
| `GET /v1/departments` | Katalog pilihan department aktif; query `q`, `hospital_id`, `limit`, `offset`. Gunakan `code` sebagai `department_code`. |
| `GET /v1/doctors` | Direktori dokter aktif; query `q`, `specialty`, `hospital_id`, `department_code`, `department_id`, `city`, `available_on`, `booking_mode`, `page`, `limit` (maksimum 100). Data berisi `items`, `page`, `limit`, `total`. |
| `GET /v1/doctors/:doctor_id` | Detail dokter dan afiliasi/jadwal aktif. Path menerima UUID atau MedikaOne ID dokter. |
| `GET /v1/hospitals` | Daftar rumah sakit aktif; query `search`, `city`, `limit` (1-100, default 20), `offset` (0-100000, default 0). |
| `GET /v1/hospitals/:hospital_id` | Detail rumah sakit berdasarkan UUID. `facilities` adalah JSON, bukan base64. |
| `GET /v1/recommendations/doctors` | Pasien terautentikasi; dokter dengan afiliasi dan jadwal aktif, diurutkan berdasarkan rating rumah sakit, jumlah jadwal, lalu nama. |
| `GET /v1/recommendations/hospitals` | Pasien terautentikasi; rumah sakit dengan dokter dan jadwal aktif, default diurutkan berdasarkan rating. |

Endpoint direktori publik dapat dipakai Website maupun Mobile. Direktori dokter
menampilkan identitas profesional, tanpa email, telepon pribadi, NIK, atau DOB.
Resource nonaktif/diarsipkan tidak muncul di direktori.

## Update dan delete

| Method / path | Akses / perilaku |
| --- | --- |
| `PATCH /v1/hospitals/:hospital_id` | ADMIN tenant atau SUPER_ADMIN; mengubah field rumah sakit yang diberikan. |
| `DELETE /v1/hospitals/:hospital_id` | SUPER_ADMIN; soft-delete rumah sakit, menonaktifkan resource operasional. Ditolak jika ada appointment aktif. |
| `PATCH /v1/hospitals/:hospital_id/departments/:department_id` | ADMIN tenant/SUPER_ADMIN; field `code`, `name`. |
| `DELETE /v1/hospitals/:hospital_id/departments/:department_id` | ADMIN tenant/SUPER_ADMIN; nonaktifkan department jika tidak sedang digunakan. |
| `PATCH /v1/hospitals/:hospital_id/rooms/:room_id` | ADMIN tenant/SUPER_ADMIN; field `department_id`, `code`, `name`. Perpindahan department ditolak bila merusak referensi riwayat. |
| `DELETE /v1/hospitals/:hospital_id/rooms/:room_id` | ADMIN tenant/SUPER_ADMIN; nonaktifkan room jika tidak sedang digunakan. |
| `PATCH /v1/hospitals/:hospital_id/doctor-invitations/:invitation_id` | ADMIN tenant/SUPER_ADMIN; ubah undangan PENDING yang belum kedaluwarsa. |
| `DELETE /v1/hospitals/:hospital_id/doctor-invitations/:invitation_id` | ADMIN tenant/SUPER_ADMIN; arsipkan undangan non-ACCEPTED. Kontrak dan audit dipertahankan. |
| `DELETE /v1/hospitals/:hospital_id/doctors/:doctor_id` | ADMIN tenant/SUPER_ADMIN; arsipkan afiliasi dokter di rumah sakit ini, tidak menghapus akun atau afiliasi rumah sakit lain. Ditolak bila ada appointment aktif. |
| `DELETE /v1/notifications/:notification_id` | Akun terautentikasi; arsipkan notifikasi milik sendiri. |
| `DELETE /v1/account` | Akun terautentikasi; JSON `current_password` wajib. Soft-delete akun dan cabut semua sesi. |

Route update/delete pada tabel di atas dengan path `/hospitals/:hospital_id`
menerima UUID atau kode rumah sakit;
GET detail rumah sakit publik tetap memakai UUID. PATCH rumah sakit menerima
`code`, `name`, `address`, `city`, `province`, `country`, `latitude`, `longitude`,
`phone`, `description`, dan `facilities`. Field yang tidak dikirim dipertahankan;
`description: ""` mengosongkan deskripsi dan `facilities: null` mengosongkan
fasilitas. Fasilitas selain null harus berupa object atau array JSON.

DELETE department ikut menonaktifkan seluruh room di dalamnya. Department/room
tidak dapat dihapus saat dipakai afiliasi aktif dokter yang akunnya masih aktif,
undangan PENDING yang belum kedaluwarsa untuk dokter aktif, atau appointment
aktif. Afiliasi yang dihapus hilang dari daftar dan seluruh jadwalnya dinonaktifkan;
undangan baru dapat dibuat kembali untuk penempatan yang sama.

PATCH invitation menggunakan JSON dengan field opsional `department_id`,
`room_id`, `message`, `schedules`. Field yang tidak dikirim dipertahankan.
`room_id`/`message` string kosong mengosongkan nilai; `schedules: []` menghapus
seluruh jadwal usulan. Dokter penerima, rumah sakit pengirim, dan berkas kontrak
tidak diganti melalui PATCH. Undangan tanpa jadwal awal tetap diperbolehkan.
`department_id` wajib dipilih dari list department rumah sakit; nilai kosong atau
placement dari rumah sakit lain menghasilkan HTTP 404
`HOSPITAL_PLACEMENT_NOT_FOUND`.
Undangan diarsipkan hilang dari daftar/detail, dan endpoint URL kontraknya tidak
lagi tersedia. Undangan ACCEPTED tetap dipertahankan; hentikan afiliasi dokter
melalui endpoint delete afiliasi bila ingin mengakhiri penempatannya.

DELETE tidak menghapus rekam medis, resep yang sudah diterbitkan, atau provenance
kontrak. Kode `RESOURCE_IN_USE` (409) berarti appointment atau resource aktif
yang bergantung harus diselesaikan terlebih dahulu. Penghapusan akun juga
ditolak dengan `LAST_ADMINISTRATOR` (409) jika merupakan satu-satunya admin aktif
global atau pada sebuah rumah sakit aktif. Akun yang dihapus tidak dapat login,
refresh, atau menggunakan access token lama.

## Array hari dan jadwal sekali saja

Ini perubahan kontrak: angka scalar `day_of_week: 1` diganti array. Jadwal rutin:

```json
{
  "day_of_week": [1, 3, 5],
  "start_time": "08:00",
  "end_time": "12:00",
  "timezone": "Asia/Jakarta",
  "booking_mode": "FIXED_SLOT",
  "slot_duration_minutes": 30,
  "capacity": 1
}
```

`0` adalah Minggu dan `6` Sabtu. Hari harus unik. Satu input dengan beberapa hari
dipecah menjadi schedule terpisah agar setiap `schedule_id` tetap menunjuk satu
kejadian mingguan. Karena itu respons rutin memakai array satu elemen, misalnya
`[1]`, `[3]`, dan `[5]`, masing-masing dengan ID sendiri.

Jadwal sekali saja memakai tanggal lokal spesifik dan tanpa hari rutin:

```json
{
  "day_of_week": [],
  "schedule_date": "2026-12-01",
  "start_time": "13:00",
  "end_time": "17:00",
  "timezone": "Asia/Jakarta",
  "booking_mode": "SESSION_QUEUE",
  "slot_duration_minutes": 30,
  "capacity": 20
}
```

Gunakan tanggal mendatang. `day_of_week` tidak boleh berisi hari bila
`schedule_date` diisi. Slot availability hanya muncul untuk tanggal tersebut
dan tidak berulang pada minggu berikutnya. Detail dokter dapat menampilkan
jadwal bertanggal mendatang sebelum tanggal praktik tiba. Kedua bentuk dapat dipakai
dalam `schedules` undangan dan proposal perubahan jadwal.

| Method / path | Hasil |
| --- | --- |
| `POST /v1/doctor/specific-schedules` | Dokter mengajukan ADD satu jadwal, menunggu persetujuan rumah sakit. |
| `POST /v1/hospitals/:hospital_id/specific-schedules` | Rumah sakit mengajukan ADD satu jadwal, menunggu persetujuan dokter. |
| `DELETE /v1/doctor/schedules/:schedule_id` | Dokter mengajukan REMOVE, HTTP 202, menunggu persetujuan rumah sakit. |
| `DELETE /v1/hospitals/:hospital_id/schedules/:schedule_id` | Rumah sakit mengajukan REMOVE, HTTP 202, menunggu persetujuan dokter. |

Body POST adalah `{ "affiliation_id": "UUID", "reason": "opsional", "schedule": {...} }`.
DELETE tidak memiliki body. Semua route ini membutuhkan permission
`doctor_schedule.propose` pada scope yang sesuai. Respons adalah proposal dengan
`status: PENDING` dan `operation: ADD` atau `REMOVE`; gunakan endpoint
`schedule-change-requests/:change_id/approve` atau `/reject` yang sudah tersedia.
`target_schedule_id` menunjuk schedule yang akan dihapus pada REMOVE.

Proposal penggantian daftar jadwal yang sudah ada memakai `operation: REPLACE`.
ADD mempertahankan jadwal lain, sedangkan REMOVE hanya menonaktifkan target.
Persetujuan tetap memeriksa konflik dokter lintas rumah sakit, tanggal, serta
appointment aktif. Schedule lama dipertahankan untuk referensi riwayat.
