# Direktori, lifecycle resource, dan jadwal dokter

Migration terbaru: `20260928043302_schedule_deactivation.sql`. Jalankan seluruh
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
| `GET /v1/departments` | Master pilihan department/poli Indonesia; query `q`, `category`, `hospital_id`, `limit`, `offset`. Gunakan `id` untuk create/update department rumah sakit dan `code` sebagai filter `department_code`. |
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
| `PATCH /v1/hospitals/:hospital_id/departments/:department_id` | ADMIN tenant/SUPER_ADMIN; pilih `master_department_id`. Perubahan ditolak bila department sudah mempunyai riwayat. |
| `DELETE /v1/hospitals/:hospital_id/departments/:department_id` | ADMIN tenant/SUPER_ADMIN; nonaktifkan department jika tidak sedang digunakan. |
| `PATCH /v1/hospitals/:hospital_id/rooms/:room_id` | ADMIN tenant/SUPER_ADMIN; field `department_id`, `code`, `name`. Perpindahan department ditolak bila merusak referensi riwayat. |
| `DELETE /v1/hospitals/:hospital_id/rooms/:room_id` | ADMIN tenant/SUPER_ADMIN; nonaktifkan room jika tidak sedang digunakan. |
| `PATCH /v1/hospitals/:hospital_id/doctor-invitations/:invitation_id` | ADMIN tenant/SUPER_ADMIN; ubah undangan PENDING yang belum kedaluwarsa. |
| `DELETE /v1/hospitals/:hospital_id/doctor-invitations/:invitation_id` | ADMIN tenant/SUPER_ADMIN; arsipkan undangan non-ACCEPTED. Kontrak dan audit dipertahankan. |
| `GET /v1/doctor/hospital-affiliations/:affiliation_id` | Dokter mengambil detail satu afiliasi miliknya. |
| `GET /v1/hospitals/:hospital_id/doctor-affiliations/:affiliation_id` | ADMIN tenant/SUPER_ADMIN mengambil detail satu afiliasi dalam rumah sakitnya. |
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

Create department hanya menerima `master_department_id` hasil
`GET /v1/departments`; kode dan nama diturunkan oleh backend. Daftar lengkap dan
pemisahan antara ID master dengan `department_id` milik rumah sakit dijelaskan di
[`department-master.md`](department-master.md).

PATCH invitation menggunakan JSON dengan field opsional `department_id`,
`room_id`, `message`, `schedules`. Field yang tidak dikirim dipertahankan.
`room_id`/`message` string kosong mengosongkan nilai; `schedules: []` menghapus
seluruh jadwal usulan. Dokter penerima dan rumah sakit pengirim tetap.
Undangan tanpa jadwal awal tetap diperbolehkan.

Untuk mengunggah kontrak baru, endpoint PATCH yang sama menerima
`multipart/form-data`: `contract` adalah file PDF opsional, sedangkan
`department_id`, `room_id`, `message`, dan `schedules` adalah field teks opsional.
`schedules` berisi array JSON tanpa komentar; omit atau `null` mempertahankan
jadwal, dan `[]` mengosongkannya. Biarkan HTTP client membuat Content-Type beserta
boundary multipart. Tidak mengirim `contract` mempertahankan kontrak saat ini.
Penggantian hanya berlaku pada invitation PENDING yang belum kedaluwarsa.
`contract_filename` dan URL `version=original` mengacu ke PDF terbaru setelah
berhasil; metadata kontrak lama masuk ke audit UPDATED dan file lama dipertahankan.
Jika transaksi update gagal, file baru dibersihkan tanpa mengubah kontrak lama.
Ukuran berlebih menghasilkan HTTP 413 `FILE_TOO_LARGE` atau `REQUEST_TOO_LARGE`,
sedangkan PDF tidak valid menghasilkan HTTP 400 `INVALID_CONTRACT_PDF`.

`department_id` wajib dipilih dari list department rumah sakit; nilai kosong atau
placement dari rumah sakit lain menghasilkan HTTP 404
`HOSPITAL_PLACEMENT_NOT_FOUND`.
Undangan diarsipkan hilang dari daftar/detail, dan endpoint URL kontraknya tidak
lagi tersedia. Undangan ACCEPTED tetap dipertahankan; hentikan afiliasi dokter
melalui endpoint delete afiliasi bila ingin mengakhiri penempatannya.

Detail invitation dokter dan rumah sakit mempertahankan field identitas lama
dan menambahkan `hospital` sebagai satu object berisi identitas, alamat, kontak,
deskripsi, fasilitas, jam operasional, timezone, dan rating rumah sakit.
Invitation yang sudah diterima juga memuat `affiliation_id`, sehingga client
dapat langsung membuka detail afiliasi setelah accept. Sebelum afiliasi dibuat,
field tersebut tidak dikirim. Detail afiliasi memakai bentuk afiliasi yang sama
dengan list, ditambah object
`hospital` dan object `invitation` untuk penawaran asal serta nama kontraknya.
Object `invitation` pada detail afiliasi sengaja tidak memuat `message`; pesan
penawaran hanya tersedia melalui detail invitation.

Endpoint kontrak dokter
`GET /v1/doctor/hospital-invitations/:invitation_or_affiliation_id/contract`
menerima UUID invitation atau UUID affiliation hasil accept. Kedua bentuk tetap
di-scope ke dokter dari bearer token; UUID milik dokter lain menghasilkan not
found.

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
jadwal bertanggal mendatang sebelum tanggal praktik tiba. Kedua bentuk dapat
dipakai dalam input `schedules` undangan dan ditampilkan dalam
`schedule_groups[].schedules` pada response undangan atau proposal perubahan jadwal.

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

Proposal penggantian jadwal rutin memakai `operation: REPLACE` dan itemnya tidak
boleh memiliki `schedule_date`. Approval hanya mengganti jadwal rutin aktif
(`schedule_date IS NULL`) pada afiliasi tersebut; seluruh specific schedule tetap
aktif. ADD mempertahankan jadwal lain, sedangkan REMOVE hanya menonaktifkan target.
Persetujuan tetap memeriksa konflik dokter lintas rumah sakit, tanggal, specific
schedule yang dipertahankan, serta appointment aktif pada schedule yang benar-benar
terdampak. Schedule lama dipertahankan untuk referensi riwayat.

Setiap afiliasi hanya boleh mempunyai satu proposal rutin `REPLACE` yang
`PENDING`. Specific schedule memakai `ADD`, sehingga beberapa tanggal specific
boleh diajukan dan menunggu persetujuan secara bersamaan, termasuk ketika satu
proposal rutin masih pending. `REMOVE` juga dapat pending untuk beberapa target
berbeda, tetapi target schedule yang sama tidak dapat diajukan dua kali.

`GET /v1/doctor/hospital-affiliations` dan
`GET /v1/hospitals/:hospital_id/doctors` selalu memisahkan keadaan saat ini dari
proposal. `schedule_groups[].schedules` pada afiliasi hanya berisi row jadwal yang
masih aktif dan setiap item memiliki `status: ACTIVE`; jadwal dapat dibooking hanya ketika afiliasi dan
resource terkait juga aktif. `pending_schedule_changes` selalu berupa array dan
berisi seluruh proposal `PENDING` beserta snapshot jadwal usulan; hasil kosong
adalah `[]`. Selama proposal menunggu,
jadwal aktif tidak berubah. Setelah approval `REPLACE`, proposal hilang dan
jadwal rutin pada `schedule_groups[].schedules` berisi snapshot baru dengan ID
baru, sementara specific schedule aktif tetap memakai ID lamanya. Untuk `ADD`, jadwal usulan
ditambahkan; untuk `REMOVE`, `target_schedule_id` dinonaktifkan.
ID pada item snapshot pending adalah ID item proposal, bukan `schedule_id` aktif;
ID tersebut tidak boleh dipakai untuk booking atau delete schedule.

### Konflik ketika pengajuan dibuat

ADD/REPLACE memeriksa jadwal aktif dan semua proposal ADD/REPLACE PENDING yang
belum kedaluwarsa milik dokter di seluruh afiliasi. Pemeriksaan serta penyimpanan
berada dalam transaksi dengan lock per dokter, sehingga dua pengajuan bersamaan
pada waktu yang sama tidak dapat keduanya berhasil. Pending REMOVE tidak
membebaskan waktu sampai disetujui. REPLACE mengabaikan hanya jadwal rutin aktif
afiliasi yang sedang diganti; specific schedule yang dipertahankan tetap dicek.

Contoh: rutin Rabu 09:00–12:00 menghalangi specific pada Rabu 10:00–11:00.
Specific PENDING tanggal 30 September 2026 09:00–10:00 juga menghalangi request
kedua untuk tanggal dan jam tersebut. Jadwal 10:00–11:00 boleh mengikuti
09:00–10:00 jika tidak berbenturan dengan jadwal/proposal lain. Perbandingan
memakai timezone IANA, bukan hanya teks tanggal/jam lokal.

Konflik menghasilkan 409 `DOCTOR_SCHEDULE_CONFLICT` sejak POST, tanpa membuat
proposal baru. Approval memeriksa ulang, dengan mengecualikan proposalnya sendiri.
Proposal duplikat yang telanjur ada sebelum perbaikan tidak dihapus otomatis;
tolak salah satunya sebelum menyetujui proposal lain. Proposal ditolak atau
kedaluwarsa tidak lagi memblokir pengajuan baru.

### Ringkasan jadwal untuk tampilan

Semua object response jadwal memakai `schedule_groups`: undangan
list/detail/create/update/accept, afiliasi
list/detail, detail dokter publik, serta proposal jadwal create/list dan proposal
nested dalam `pending_schedule_changes`. Jika tidak ada jadwal, nilainya `[]`.
Setiap group juga memuat `schedules` berupa array object jadwal lengkap. Frontend
dapat membaca `schedule_groups[i].schedules[j]` untuk ID, hari/tanggal, jam,
timezone, status, dan aturan booking tanpa mencocokkan ID ke array lain.
Array `schedules` di luar group **tidak dikirim lagi**, termasuk pada setiap
proposal pending. Ini perubahan kontrak response: client yang membaca
`affiliation.schedules` harus beralih ke
`affiliation.schedule_groups.flatMap(group => group.schedules)`; proposal memakai
path yang sama di dalam masing-masing `pending_schedule_changes`.
`item_ids` tetap tersedia pada group. Body request create/update tetap memakai
bentuk `schedules` yang sudah ada. Tidak ada migration database tambahan.

Contoh satu group untuk empat hari rutin:

```json
{
  "type": "RECURRING",
  "status": "ACTIVE",
  "item_ids": [
    "11111111-1111-4111-8111-111111111111",
    "22222222-2222-4222-8222-222222222222",
    "33333333-3333-4333-8333-333333333333",
    "44444444-4444-4444-8444-444444444444"
  ],
  "schedules": [
    {"id":"11111111-1111-4111-8111-111111111111","status":"ACTIVE","day_of_week":[1],"start_time":"19:00","end_time":"20:00","timezone":"Asia/Jakarta","booking_mode":"FIXED_SLOT","slot_duration_minutes":30,"capacity":1},
    {"id":"22222222-2222-4222-8222-222222222222","status":"ACTIVE","day_of_week":[2],"start_time":"19:00","end_time":"20:00","timezone":"Asia/Jakarta","booking_mode":"FIXED_SLOT","slot_duration_minutes":30,"capacity":1},
    {"id":"33333333-3333-4333-8333-333333333333","status":"ACTIVE","day_of_week":[3],"start_time":"19:00","end_time":"20:00","timezone":"Asia/Jakarta","booking_mode":"FIXED_SLOT","slot_duration_minutes":30,"capacity":1},
    {"id":"44444444-4444-4444-8444-444444444444","status":"ACTIVE","day_of_week":[4],"start_time":"19:00","end_time":"20:00","timezone":"Asia/Jakarta","booking_mode":"FIXED_SLOT","slot_duration_minutes":30,"capacity":1}
  ],
  "day_of_week": [1, 2, 3, 4],
  "day_label": "Senin–Kamis",
  "time_label": "19:00–20:00",
  "display_label": "Senin–Kamis, 19:00–20:00",
  "start_time": "19:00",
  "end_time": "20:00",
  "timezone": "Asia/Jakarta",
  "booking_mode": "FIXED_SLOT",
  "slot_duration_minutes": 30,
  "capacity": 1
}
```

Group hanya menggabungkan jadwal rutin dengan jam mulai/selesai, timezone,
status, booking mode, durasi slot, dan kapasitas identik. Urutan hari
Senin–Minggu; hari tidak berurutan memakai koma, misalnya `Senin, Rabu, Jumat`.
Sesi 09:00–10:00 dan 10:00–11:00 tetap dua group karena mempunyai batas sesi
berbeda. Untuk specific: `type: SPECIFIC`, `day_of_week: []`, `schedule_date`
tetap ada, dan label tanggal misalnya `30 Sep 2026, 09:00–10:00`.

Jadwal aktif memakai `schedule_groups` pada afiliasi; jadwal pending memakai
`pending_schedule_changes[i].schedule_groups` sehingga setiap pengajuan dan
statusnya tetap terpisah. `item_ids` pada group aktif mengacu ke ID schedule
asli, tetapi pada proposal mengacu ke ID item proposal yang belum bookable.
Object di `schedule_groups[i].schedules` mempunyai identitas yang sama; status
yang belum diisi pada row mengikuti status group tanpa mengubah array sumber.
Row rutin dalam group diurutkan Senin–Minggu, lalu ID sebagai pembeda stabil.
Row specific tetap memiliki `schedule_date` dan `day_of_week: []`. Pada proposal
DEACTIVATE, object nested juga menyertakan `target_schedule_id`; ID object
sendiri tetap ID item proposal.
Group bukan resource baru dan tidak memiliki satu ID untuk booking/delete.
Status snapshot invitation dapat tidak ada; status invitation tetap berada pada
object induk. Endpoint availability, jadwal hari ini, dan appointment tetap
menyajikan satu sesi/kunjungan per item sesuai tanggalnya, bukan snapshot rutin.

## Penonaktifan seluruh jadwal atau satu hari rutin

| Endpoint | Otorisasi |
| --- | --- |
| `POST /v1/doctor/hospital-affiliations/:affiliation_id/schedules/deactivate` | Dokter pemilik afiliasi, permission `doctor_schedule.propose`. |
| `POST /v1/hospitals/:hospital_id/doctor-affiliations/:affiliation_id/schedules/deactivate` | Tenant rumah sakit afiliasi, permission `doctor_schedule.propose`. |

Body untuk seluruh jadwal rutin dan khusus aktif dalam satu afiliasi:

```json
{"scope":"ALL","reason":"Menghentikan seluruh jadwal praktik di rumah sakit ini"}
```

Body untuk seluruh sesi rutin pada satu hari lokal, misalnya Senin:

```json
{"scope":"RECURRING_DAY","day_of_week":1,"reason":"Tidak praktik rutin pada hari Senin"}
```

`scope` wajib, case-sensitive, dengan dua nilai di atas. `day_of_week` pada
endpoint ini merupakan **satu angka**, bukan array: `0` Minggu hingga `6` Sabtu.
Field wajib pada `RECURRING_DAY`; hilangkan saat `ALL` (null diperlakukan sebagai
tidak dikirim). `reason` opsional, maksimal 1000 karakter. Pilihan satu hari
menargetkan semua sesi rutin pada hari itu di afiliasi tersebut; jadwal khusus
pada tanggal yang jatuh di hari yang sama tidak ikut dinonaktifkan. Afiliasi
rumah sakit lain tidak terpengaruh. Tidak ada target aktif menghasilkan 404
`DOCTOR_SCHEDULE_NOT_FOUND`.

Kedua endpoint mengembalikan HTTP **202 Accepted**, message
`DOCTOR_SCHEDULE_DEACTIVATION_REQUESTED` atau
`HOSPITAL_SCHEDULE_DEACTIVATION_REQUESTED`, dan object schedule change lengkap.
Field baru pada proposal adalah:

```json
{
  "operation": "DEACTIVATE",
  "status": "PENDING",
  "deactivation_scope": "RECURRING_DAY",
  "deactivation_day_of_week": 1,
  "schedule_groups": [
    {
      "type": "RECURRING",
      "status": "PENDING",
      "day_of_week": [1],
      "day_label": "Senin",
      "time_label": "08:00–12:00",
      "display_label": "Senin, 08:00–12:00",
      "start_time": "08:00",
      "end_time": "12:00",
      "timezone": "Asia/Jakarta",
      "booking_mode": "FIXED_SLOT",
      "slot_duration_minutes": 30,
      "capacity": 1,
      "item_ids": ["55555555-5555-4555-8555-555555555555"],
      "schedules": [
        {
          "id": "55555555-5555-4555-8555-555555555555",
          "target_schedule_id": "88888888-8888-4888-8888-888888888888",
          "status": "PENDING",
          "day_of_week": [1],
          "start_time": "08:00",
          "end_time": "12:00",
          "timezone": "Asia/Jakarta",
          "booking_mode": "FIXED_SLOT",
          "slot_duration_minutes": 30,
          "capacity": 1
        }
      ]
    }
  ]
}
```

Contoh tersebut adalah potongan payload; metadata proposal lengkap juga
disertakan. Scope `ALL` tidak mengirim `deactivation_day_of_week`. ID item
adalah UUID proposal, sedangkan `target_schedule_id` menunjuk jadwal aktif yang
akan dinonaktifkan. Snapshot dan target disimpan saat pengajuan; approval tidak
memilih ulang atau memperluas target secara diam-diam. `schedule_groups[i].item_ids`
dan `schedule_groups[i].schedules[j].id` tetap menunjuk item proposal. Target
jadwal aktif tersedia pada `schedule_groups[i].schedules[j].target_schedule_id`.
Frontend harus membaca `operation` untuk memberi
label **pengajuan penonaktifan**, bukan menganggapnya calon jadwal baru.

Proposal muncul pada list schedule change dan
`pending_schedule_changes` di list/detail afiliasi. Jadwal aktif tetap tampil
pada `schedule_groups[].schedules` afiliasi dan tetap bookable sampai approval.
Gunakan endpoint approve/reject yang sudah ada; dokter menyetujui pengajuan rumah sakit dan rumah sakit
menyetujui pengajuan dokter. Pengaju sendiri mendapat 403
`SCHEDULE_CHANGE_COUNTERPART_REVIEW_REQUIRED`.

Approval menonaktifkan seluruh target dalam satu transaksi. Jadwal hilang dari
list aktif, direktori/availability hanya memperhitungkan jadwal yang masih aktif,
dan riwayat appointment serta snapshot proposal tetap ada. Afiliasi tetap aktif.
Snapshot proposal yang sudah disetujui berstatus `APPROVED`, yang merupakan status
pengajuan dan tidak berarti jadwal target masih aktif. Tidak ada reaktivasi
otomatis; jadwal baru dapat diajukan melalui flow rutin/khusus yang sudah ada.

- Appointment target berstatus `CONFIRMED`, `CHECKED_IN`, `WAITING_VITALS`,
  `WAITING_DOCTOR`, atau `IN_CONSULTATION` menolak seluruh approval dengan 409
  `SCHEDULE_CHANGE_HAS_ACTIVE_APPOINTMENTS`. Batalkan atau reschedule appointment
  terlebih dahulu. Appointment pada jadwal yang tidak ditargetkan tidak menghalangi.
- Target yang sudah nonaktif atau tidak lagi sesuai scope menolak seluruh approval
  dengan 409 `SCHEDULE_CHANGE_STATE_CONFLICT`; tidak ada perubahan parsial.
- `ALL` berbenturan dengan setiap proposal pending lain pada afiliasi yang sama.
  Selama pending, `ALL` juga memblokir pengajuan perubahan baru pada afiliasi itu.
- `RECURRING_DAY` berbenturan dengan `REPLACE`, `DEACTIVATE ALL`, deactivation hari
  yang sama, dan `REMOVE` sesi rutin pada hari yang sama. `ADD`, `REMOVE` specific,
  serta perubahan hari lain tetap diizinkan dengan validasi konflik jadwal biasa.
- Benturan proposal menghasilkan 409 `SCHEDULE_CHANGE_ALREADY_PENDING`. Aturan
  berlaku dua arah, tidak bergantung urutan pengajuan. Proposal kedaluwarsa tidak
  memblokir pengajuan baru setelah expiry diproses; TTL tetap tujuh hari. Jika
  proposal kedaluwarsa masih dikunci transaksi review lain, request dapat menerima
  konflik pending sementara dan dapat diulang setelah transaksi tersebut selesai.
- Approval gagal karena appointment/target tidak valid mempertahankan `PENDING`.
  Reject atau expiry tidak menonaktifkan jadwal.

Migration baru wajib diterapkan sebelum deploy; startup memerlukan versi
`20260928043302`. Seeder yang ada sudah menyediakan jadwal aktif untuk kedua flow;
tidak menambahkan pending deactivation otomatis agar demo booking tetap tersedia.
