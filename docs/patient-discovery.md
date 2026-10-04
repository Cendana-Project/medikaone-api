# Pencarian dokter dan rumah sakit untuk pasien

Filter berikut berlaku pada endpoint publik `GET /v1/doctors`,
`GET /v1/hospitals` serta endpoint pasien `GET /v1/recommendations/doctors`
dan `GET /v1/recommendations/hospitals`. Ranking yang sudah ada tetap berlaku
setelah filter, termasuk praktik terdekat jika koordinat pasien dikirim.

## Pengalaman dan jenis kelamin dokter

| Query dokter | Arti |
| --- | --- |
| `gender=L` / `gender=P` | Laki-laki / perempuan; opsional. |
| `min_experience_years=5` | Minimal 5 tahun penuh, inklusif; integer 0–100. |
| `max_experience_years=15` | Maksimal 15 tahun penuh, inklusif; integer 0–100 dan >= minimum. |

Dokter mengisi `practice_started_on` (`YYYY-MM-DD`, dari 1900-01-01 sampai
hari ini UTC). Field ini opsional pada `POST /v1/auth/set-profile` untuk role
DOCTOR, di dalam `profile`, dan `PATCH /v1/profile`, di dalam `doctor_profile`.
PATCH menghilangkan field mempertahankan nilai, `null` menghapus nilai,
sedangkan string kosong/tanggal tidak valid ditolak. Field hanya bisa diubah
oleh pemilik profil yang memiliki role DOCTOR; aturan SIP dan DOB tetap berlaku.

`experience_years` dihitung dari tahun kalender penuh antara tanggal tersebut
dan tanggal UTC saat query. Nilainya berasal dari isian dokter, bukan verifikasi
sertifikasi atau tanggal pembuatan akun. Client tidak mengirim `experience_years`.
List/detail/rekomendasi dokter mengembalikan `gender`, `practice_started_on`,
dan `experience_years`; profil pribadi dokter juga memuat kedua field pengalaman.
Response onboarding menyertakan field pengalaman bila tanggal diisi; GET/PATCH
profil pribadi tetap mengembalikan field pengalaman nullable di `doctor_profile`.
Data yang belum diisi bernilai `null`, tidak dihitung sebagai nol, dan tidak
memenuhi filter pengalaman/gender yang relevan. Nilai nol tetap valid untuk
dokter yang mulai praktik kurang dari setahun lalu.

```json
{
  "doctor_profile": {
    "practice_started_on": "2015-07-01"
  }
}
```

## Ketersediaan dokter dan rumah sakit

| Query | Arti |
| --- | --- |
| `available_on=2026-10-10` | Tanggal praktik lokal `YYYY-MM-DD`, hari ini UTC sampai satu tahun ke depan. |
| `available_from=09:00` dan `available_to=12:00` | Rentang jam lokal jadwal; harus berpasangan, format `HH:mm`, akhir > awal pada hari yang sama. Membutuhkan `available_on`. |
| `booking_mode=FIXED_SLOT` / `SESSION_QUEUE` | Membatasi jenis booking pada jadwal yang sama. |
| `only_available=true` | Harus ada slot/sesi yang masih bisa dibooking; membutuhkan `available_on`. Default `false`. |
| `open_now=true` / `false` | Khusus RS: sedang buka / tutup menurut jam operasional dan timezone RS saat request. Jam tidak diketahui tidak masuk kedua filter. |

`available_on` tanpa `only_available=true` tetap memeriksa keberadaan jadwal
aktif, bukan kapasitas. Jika jam dikirim pada mode ini, rentang praktik harus
beririsan dengan rentang pencarian. Dengan `only_available=true`, minimal satu
slot FIXED_SLOT atau keseluruhan sesi SESSION_QUEUE harus muat di rentang jam
tersebut. Contoh: sesi antrean 09:00–12:00 tidak memenuhi pencarian 10:00–11:00,
sementara slot tetap 10:00–10:30 memenuhi pencarian tersebut.

Bookable berarti mulai lebih dari 2 jam dari sekarang, paling jauh 90 × 24 jam,
dan kapasitas belum habis. Jadwal rutin dan khusus aktif dihitung; proposal
pending, afiliasi/RS/poli/ruang nonaktif, dan jadwal yang berakhir tidak dihitung.
Tanggal dalam batas filter satu tahun tetapi di luar horizon booking akan
menghasilkan hasil kosong saat `only_available=true`.

Kapasitas FIXED_SLOT dihitung per `schedule_id + appointment_date + scheduled_start_at`;
SESSION_QUEUE memakai kapasitas seluruh sesi. Appointment berstatus CONFIRMED,
CHECKED_IN, WAITING_VITALS, WAITING_DOCTOR, dan IN_CONSULTATION mengurangi kapasitas.
Status terminal seperti CANCELLED tidak menguranginya. Aturan ini mengikuti API
availability dan booking. Search tidak menahan slot; booking tetap memvalidasi
kapasitas secara atomik karena bisa berubah setelah hasil pencarian diterima.

Pada RS, dokter yang tersedia harus berada di poli pilihan pada RS tersebut.
Pada dokter dengan banyak afiliasi, filter lokasi, poli, rumah sakit, tanggal,
jam, dan kapasitas harus terpenuhi pada afiliasi yang sama. Semua filter
diterapkan sebelum pagination; `data.items/total` dokter dan `data` array RS
tetap seperti sebelumnya. `open_now` tidak menunjukkan apakah RS buka pada
`available_on`; keduanya adalah filter independen yang digabung dengan AND.

```http
GET /v1/doctors?gender=P&min_experience_years=5&max_experience_years=20&available_on=2026-10-10&available_from=09:00&available_to=12:00&only_available=true&page=1&limit=20
GET /v1/recommendations/doctors?latitude=-6.21&longitude=106.81&available_on=2026-10-10&only_available=true
GET /v1/hospitals?department_code=POLI-MATA&available_on=2026-10-10&available_from=09:00&available_to=12:00&only_available=true&open_now=true&limit=20&offset=0
GET /v1/recommendations/hospitals?department_code=POLI-MATA&available_on=2026-10-10&only_available=true
```

Sesuaikan tanggal contoh sebelum menjalankan request. Filter baru yang kosong,
berulang, atau tidak valid menghasilkan HTTP 400 `INVALID_FIELD_VALUE`;
`available_on` yang wajib tetapi tidak dikirim menghasilkan `FIELD_REQUIRED`.
Pesan mengikuti envelope bilingual standar.

## Migration dan seeder

Jalankan migration `20261004065141_doctor_practice_experience.sql` sebelum
menjalankan server baru. Migration hanya menambah kolom nullable; data dokter
lama tidak diberi pengalaman buatan. Startup memeriksa versi schema ini.
Gunakan workflow migration repo dan `DATABASE_ADMIN_DSN` untuk production.
Seeder development mengisi tanggal mulai praktik berbeda pada tiga dokter demo
dan tetap idempoten. Bruno Mobile/Website memuat filter, saved examples, dan
request pembaruan profil terkait. Payload create schedule tetap sama.
