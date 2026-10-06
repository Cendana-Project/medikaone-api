# Kalender slot dokter per rumah sakit

```http
GET /v1/appointments/availability/grouped?doctor_id=<uuid>&date_from=2026-10-10&date_to=2026-10-16
Authorization: Bearer <patient-access-token>
```

Endpoint kalender booking pasien memakai `AuthRequired` dan permission global
`appointment.view`, sama dengan availability lama. Role PATIENT default memiliki
izin tersebut. Response sukses HTTP 200 memakai message
`APPOINTMENT_AVAILABILITY_GROUPED` dan envelope bilingual standar.

## Query

| Parameter | Kontrak |
| --- | --- |
| `doctor_id` | Wajib UUID dokter; bukan MedikaOne ID. Dokter harus memenuhi eligibility direktori publik (aktif, tidak dihapus, terverifikasi, mempunyai SIP dan role DOCTOR aktif). |
| `hospital_id` | Opsional UUID; hilangkan untuk semua tempat praktik dokter. RS yang tidak mempunyai jadwal eligible menghasilkan kalender kosong, bukan error. |
| `date_from` | Opsional `YYYY-MM-DD`; default tanggal hari ini UTC, sama dengan endpoint lama. Untuk kalender lokal, client sebaiknya mengirim tanggal eksplisit. |
| `date_to` | Opsional `YYYY-MM-DD`; default sama dengan `date_from`. |

Nilai yang dikirim kosong/berulang atau query URL malformed ditolak HTTP 400.
Rentang inklusif harus berurutan dan maksimal 31 tanggal; awal rentang tidak
boleh melewati horizon 90 hari. Tanggal lampau tetap dapat ditampilkan sebagai
slot CLOSED. Jika akhir rentang melewati horizon, slot di luar horizon tetap
ditampilkan CLOSED. Dokter tidak ditemukan/tidak eligible menghasilkan 404
`DOCTOR_NOT_FOUND`. Dokter eligible tanpa jadwal mendapat 200 dengan kalender
kosong dan identitas dokter tetap tersedia.

## Bentuk data

`data` berisi `doctor_id`, `doctor_medikaone_id`, `doctor_name`, `date_from`,
`date_to`, dan `dates`. Semua tanggal dalam rentang selalu dikembalikan:

```json
{
  "date": "2026-10-11",
  "has_available_slots": false,
  "hospitals": []
}
```

Setiap `dates[].hospitals[]` berisi `hospital_id`, `hospital_name`,
`has_available_slots`, dan `slots`. Beberapa jadwal/afiliasi pada RS yang sama
digabung menjadi satu hospital. Rumah sakit muncul jika mempunyai sesi pada
tanggal itu, termasuk jika semua pilihannya FULL/CLOSED.

Setiap slot memuat:

- `schedule_id`, `affiliation_id`, `department_id`, `department_name`, serta
  `room_id`/`room_name` jika tersedia;
- `schedule_type`: RECURRING atau SPECIFIC;
- `booking_mode`, `slot_duration_minutes`, `capacity`, `available_capacity`;
- `start_time` dan `end_time` dalam HH:mm lokal, `timezone` IANA, serta
  `start_at` dan `end_at` berupa timestamp UTC RFC3339;
- `is_bookable`, `status`, dan nullable `unavailable_reason`.

FIXED_SLOT mengembalikan satu pilihan per interval `slot_duration_minutes` dan
kapasitas per slot. SESSION_QUEUE mengembalikan satu pilihan untuk seluruh sesi
dengan kapasitas sesi; nilai normalisasi `slot_duration_minutes` tidak membagi
sesi antrean. Jadwal specific hanya muncul pada `schedule_date` miliknya.

Urutan tanggal menaik, hospital berdasarkan nama case-insensitive lalu ID,
slot berdasarkan waktu mulai absolut lalu schedule ID. Tanggal adalah tanggal
praktik lokal untuk masing-masing jadwal, bukan tanggal UTC dari `start_at`.
Ini berlaku juga untuk slot setelah tengah malam WITA/WIT yang timestamp UTC-nya
masih pada tanggal sebelumnya. Frontend mempertahankan timezone pada label.

## Status dan kapasitas

| Status | `is_bookable` | `unavailable_reason` |
| --- | --- | --- |
| AVAILABLE | true | null |
| FULL | false | CAPACITY_FULL |
| CLOSED | false | PAST, MINIMUM_LEAD_TIME, atau OUTSIDE_BOOKING_HORIZON |

Evaluasi berurutan: mulai <= sekarang adalah PAST; mulai <= sekarang + 2 jam
adalah MINIMUM_LEAD_TIME; mulai > sekarang + 90 x 24 jam adalah
OUTSIDE_BOOKING_HORIZON. Sesudah lolos waktu, sisa kapasitas nol berarti FULL;
selainnya AVAILABLE. CLOSED mendapat prioritas atas FULL. Ini mengikuti batas
availability lama; tepat H+2 jam belum ditampilkan sebagai bookable.

`available_capacity` adalah kapasitas tersisa, minimum nol, termasuk pada slot
CLOSED. Angka positif tidak berarti slot dapat dipilih; gunakan `is_bookable`.
`has_available_slots` true hanya jika terdapat minimal satu pilihan AVAILABLE
dalam hospital/tanggal tersebut. FULL tidak digabung dengan slot lain; nomor
jadwal, placement, jam, dan tanggal tetap utuh.

Kapasitas dihitung dari reservasi dengan status CONFIRMED, CHECKED_IN,
WAITING_VITALS, WAITING_DOCTOR, atau IN_CONSULTATION. Status terminal tidak
mengurangi kapasitas, sesuai booking/availability lama. Hanya jadwal aktif pada
afiliasi, dokter, hospital, department, dan room yang eligible digunakan;
proposal pending tidak menghasilkan slot. Query mengambil identity, semua
jadwal, dan jumlah reservasi secara batch; tidak ada query per slot/hari.

## Penggunaan frontend dan booking

1. Ambil 7 tanggal atau maksimal 31 tanggal untuk kalender.
2. Pilih `dates[]` yang sesuai, lalu render `hospitals[].slots[]`.
3. FIXED_SLOT dapat dilabeli `13:30`, SESSION_QUEUE `09:00-12:00`; gunakan
   `available_capacity` untuk `Sisa N`. FULL dilabeli `Full`, CLOSED dilabeli
   sesuai alasannya. Hanya pilihan `is_bookable=true` boleh dipilih.
4. Warna pilihan dan tombol Lihat lebih lengkap dikelola frontend; endpoint
   mengembalikan semua pilihan pada rentang yang diminta tanpa pagination slot.
5. POST `/v1/appointments` tetap memakai `schedule_id`, `appointment_date` dari
   `dates[].date`, dan `start_time` lokal (wajib FIXED_SLOT; SESSION_QUEUE boleh
   kosong atau awal sesi), beserta alasan kunjungan dan consent seperti biasa.

Availability tidak menahan kapasitas dan tidak memeriksa bentrok appointment
pribadi pasien. Booking tetap memvalidasi kapasitas, konflik pasien, eligibility,
dan waktu secara atomik. Untuk dokter yang sama, jadwal aktif di RS berbeda tidak
boleh bertumpuk secara waktu absolut; grouping tidak mengubah invariant itu.

GET `/v1/appointments/availability` tetap memakai response array lama dan hanya
mengisi `slots` dengan pilihan bookable. Perhitungan ekspansi/kapasitas digunakan
bersama oleh kedua endpoint. Tidak ada perubahan schema atau migration baru;
fixture seeder aktif yang ada dapat dipakai dengan tanggal praktik yang sesuai.
Bruno: `Mobile/Appointment/8 Grouped Doctor Availability`.
