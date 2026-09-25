# Panduan Agent MedikaOne API

Dokumen ini adalah panduan kerja utama untuk coding agent yang mengubah repo
MedikaOne API. Berlaku untuk seluruh tree repo, kecuali ada `AGENTS.md` yang lebih
dekat dengan file yang sedang diubah. Gunakan kode, migration, dan test sebagai
sumber kebenaran terakhir. Jika panduan ini berbeda dengan implementasi aktif,
periksa riwayat Git dan perbarui panduan bersamaan dengan perubahan kode.

## Tujuan repo

MedikaOne API adalah backend monolitik untuk aplikasi pasien/dokter dan website
rumah sakit. Stack utama:

- Go 1.25, Cobra, Gin, GORM, dan pgx;
- PostgreSQL dengan migration Goose;
- Redis untuk challenge PIN, rate limit, rotasi refresh token, session version,
  blacklist access token, dan maintenance guard;
- SMTP untuk PIN dan notifikasi email;
- private Supabase Storage untuk kontrak dokter, lampiran medis, foto profil,
  gambar rumah sakit, dan dokumen resep.

Redis adalah dependency keamanan. Jangan memperlakukannya sebagai cache yang
boleh gagal diam-diam. Supabase Auth tidak digunakan; autentikasi tetap dimiliki
MedikaOne.

## Langkah awal setiap tugas

1. Jalankan `git status --short --branch` dan baca perubahan yang sudah ada.
   Jangan menghapus, memformat, memasukkan ke commit, atau menimpa perubahan dan
   file tidak terlacak milik pengguna yang tidak terkait dengan tugas.
2. Periksa `git log -5 --oneline` agar perubahan mengikuti keadaan branch saat
   ini, bukan asumsi dari percakapan lama.
3. Cari implementasi dengan `rg`; jangan menebak nama route, field, status, atau
   tabel.
4. Baca dokumen domain yang terkait sebelum mengubah kontrak API:
   - [`README.md`](README.md)
   - [`docs/api-error-contract.md`](docs/api-error-contract.md)
   - [`docs/resource-lifecycle-and-schedules.md`](docs/resource-lifecycle-and-schedules.md)
   - [`docs/hospital-directory.md`](docs/hospital-directory.md)
   - [`docs/supabase-migration.md`](docs/supabase-migration.md)
5. Periksa status repo Bruno secara terpisah:
   `git -C bruno/bruno-medikaone status --short --branch`.

Jangan membuka, mencetak, menyalin ke dokumentasi, atau melakukan commit atas
secret dari `config.yml`, `.env*`, `medikaone-api.env`, CI, hosting, atau chat.
Gunakan `config.yml.example` dan nilai placeholder untuk dokumentasi. Jika sebuah
secret pernah tampil di percakapan atau log, tetap perlakukan sebagai rahasia.

## Peta kode

| Lokasi | Tanggung jawab |
| --- | --- |
| `main.go`, `cmd/` | Entry point dan command `server`, `migrate`, `seed`, fingerprint, serta reset staging yang dijaga. |
| `internal/bootstrap/` | Wiring dependency, startup, migration, dan graceful shutdown. |
| `internal/config/` | Binding YAML/environment, default, dan validasi konfigurasi per command. |
| `internal/transport/http/` | Controller HTTP dan registry route Gin. `http.transport.go` adalah sumber route aktif. |
| `internal/transport/middleware/` | Auth, tenant context, role, permission, rate limit, dan maintenance middleware. |
| `internal/service/` | Aturan bisnis dan orkestrasi use case. |
| `internal/repository/` | Query PostgreSQL, transaction, locking, dan persistence. |
| `internal/model/entity/` | Model tabel PostgreSQL. |
| `internal/model/request/` | DTO input dan tag validasi. |
| `internal/model/response/` | Payload API publik. |
| `internal/constant/` | Permission, kode/pesan sukses, dan katalog error bilingual. |
| `internal/util/` | Renderer response/error, logging, dan helper umum. |
| `internal/scheduleconflict/` | Perhitungan konflik jadwal lintas afiliasi dan timezone. |
| `internal/seeder/` | Seed development yang idempoten dan integration test database. |
| `migration/db/` | Migration SQL Goose berurutan. |
| `docs/` | Kontrak dan keputusan operasional yang tidak cocok diletakkan di komentar kode. |
| `scripts/` | Helper migration/operasi, termasuk PowerShell. |
| `bruno/bruno-medikaone/` | Repo Git terpisah untuk koleksi Bruno Mobile dan Website. |

Alur request normal adalah route/middleware -> controller -> service -> repository
atau dependency eksternal. Controller mengurai transport dan context; service
menegakkan aturan bisnis; repository menangani query, lock, dan transaction.
Jangan melewati layer hanya untuk mempercepat implementasi jika akibatnya aturan
otorisasi, audit, atau atomisitas tersebar.

## Aturan kontrak HTTP

### Route dan otorisasi

Tiga kelompok utama berada di `internal/transport/http/http.transport.go`:

- route publik, seperti direktori dokter/rumah sakit dan auth;
- `protected`, yang selalu memakai `AuthRequired` lalu role/permission sesuai
  use case;
- `tenant`, yang selalu memakai `AuthRequired` dan `TenantContext`, lalu
  permission rumah sakit.

Setiap route baru harus memasang middleware eksplisit. Jangan mengambil
`hospital_id`, user ID, role, atau permission otoritatif dari JSON. Ambil identitas
dari path/context yang sudah diverifikasi dan pastikan repository tetap melakukan
scope query ke actor/tenant yang benar.

### Response dan error

Response MedikaOne memakai envelope:

```json
{
  "message": "RESOURCE_CREATED",
  "message_detail": {
    "title_eng": "Resource created",
    "desc_eng": "Resource created.",
    "title_idn": "Resource berhasil dibuat",
    "desc_idn": "Resource berhasil dibuat."
  },
  "data": {},
  "trace_id": "uuid",
  "timestamp": "RFC3339"
}
```

- `message` adalah kode mesin `UPPER_SNAKE_CASE` yang stabil dan spesifik untuk
  outcome endpoint.
- `message_detail` wajib lengkap dalam Bahasa Indonesia dan Inggris.
- Tambahkan pesan sukses di `internal/constant/message.constant.go`.
- Tambahkan error yang dapat ditindaklanjuti client di
  `internal/constant/custom_error.constant.go` dan katalog errornya.
- Service mengembalikan `response.CustomError` atau error yang membungkusnya.
  Semua error HTTP dirender melalui `internal/util/error.go`; jangan membuat
  bentuk error ad hoc di controller.
- Gunakan `message` untuk branching client dan status HTTP untuk kelas hasil.
  Jangan mengubah kode lama tanpa alasan kompatibilitas yang jelas.
- Endpoint list harus konsisten dengan kontraknya. Bila response berbentuk page,
  pertahankan `items` dan metadata. Bila kontraknya array, hasil kosong harus `[]`,
  bukan `null` atau object tunggal.

Validasi transport menggunakan DTO dan tag validator, lalu aturan lintas field
diterapkan di service. Bedakan field yang tidak dikirim, `null`, string kosong,
dan nilai eksplisit ketika PATCH memang mempunyai semantik berbeda.

## Invariant domain yang harus dipertahankan

### Identitas, auth, dan profil

- UUID `doctor_id` adalah identifier relasional internal. Setiap response publik
  yang memuat dokter juga harus membawa `doctor_medikaone_id` dengan format
  `MDO-` diikuti 16 karakter heksadesimal uppercase.
- SIP dokter wajib unik tanpa membedakan kapitalisasi/whitespace.
- Refresh token one-time-use dan retry memakai `idempotency_key` yang sama hanya
  untuk operasi yang sama. Perubahan password dan delete account mencabut sesi.
- Role global `PATIENT`/`DOCTOR` dapat dipilih melalui self-service. Role tenant
  privileged hanya boleh diberikan melalui flow admin rumah sakit.
- Email tidak boleh diubah tanpa flow verifikasi khusus.
- Penghapusan akun mempertahankan data klinis/audit yang diwajibkan dan harus
  mencegah penghapusan admin terakhir tanpa pengganti.

### Tenant, department, room, undangan, dan afiliasi

- Query otorisasi hanya boleh memakai user, role, rumah sakit, membership, dan
  resource yang masih aktif/tidak dihapus.
- Department dan room yang dipilih pada undangan harus berasal dari rumah sakit
  pada route tersebut. Department kosong/tidak valid untuk placement harus
  menghasilkan `HOSPITAL_PLACEMENT_NOT_FOUND`.
- `GET /v1/departments` adalah katalog pilihan department publik yang diagregasi
  berdasarkan code. Untuk membuat undangan, client tetap memakai UUID hasil
  `GET /v1/hospitals/{hospital_id}/departments`.
- `GET /v1/doctor/hospital-invitations` adalah list dan harus mendukung banyak
  undangan. Jangan mereduksinya menjadi object tunggal.
- Undangan dapat dibuat tanpa jadwal awal. Accept harus memeriksa konflik lagi
  sebelum membuat afiliasi aktif.
- PATCH invitation hanya berlaku untuk invitation `PENDING` yang belum
  kedaluwarsa. Doctor dan contract tidak diganti melalui PATCH.
- Reject invitation menerima message opsional dan menyimpannya pada event/audit.
- Invitation `ACCEPTED` tidak dihapus; akhiri hubungan melalui lifecycle
  afiliasi. Delete resource adalah archive/soft delete jika riwayat harus tetap
  ada. Dependensi operasional aktif harus menghasilkan konflik yang jelas.

### Jadwal dokter

- Input jadwal rutin memakai `day_of_week` sebagai array angka unik `0..6`.
  Backend memperluas setiap hari menjadi row jadwal tersendiri. Batas jumlah
  berlaku setelah ekspansi.
- Jadwal sekali memakai `schedule_date` lokal dan tidak memerlukan hari rutin.
- `FIXED_SLOT` memakai `slot_duration_minutes` untuk membagi sesi. Durasi harus
  membagi rentang praktik tepat. `capacity` tetap didukung dan default `1` per
  slot.
- `SESSION_QUEUE` memakai `capacity` untuk seluruh sesi. Client tidak perlu
  mengirim `slot_duration_minutes`; response boleh memuat nilai normalisasi.
- Timezone adalah IANA timezone dan harus konsisten dalam satu snapshot.
- Konflik dicek terhadap seluruh jadwal aktif dokter di semua rumah sakit dan
  semua afiliasi, bukan hanya rumah sakit yang sedang diubah. Gunakan package
  `internal/scheduleconflict` dan pertahankan database trigger sebagai defense in
  depth. Jangan kembali ke perbandingan string jam lokal yang mengabaikan
  timezone.
- Periksa konflik ketika proposal dibuat/diubah, ketika invitation diterima, dan
  ketika pihak lawan menyetujui proposal. Proposal yang gagal saat approval tetap
  `PENDING` agar dapat ditolak atau diperbaiki sesuai flow yang tersedia.
- Schedule change rutin adalah operasi `REPLACE`: setelah approval, seluruh
  jadwal aktif untuk `affiliation_id` itu diganti snapshot baru. Afiliasi rumah
  sakit lain tidak ikut diganti.
- Specific schedule adalah `ADD`; delete schedule adalah `REMOVE`. Keduanya
  memakai approval pihak lawan dan tidak mengganti seluruh snapshot.

### Direktori dan rekomendasi pasien

- Direktori publik dokter mendukung pencarian/filter mobile seperti specialty,
  hospital, department code/ID, city, tanggal tersedia, dan booking mode.
- Direktori rumah sakit hanya menampilkan rumah sakit aktif dan dapat difilter
  dengan department serta atribut direktori yang terdokumentasi.
- `GET /v1/recommendations/doctors` dan
  `GET /v1/recommendations/hospitals` hanya untuk pasien terautentikasi.
- Rekomendasi hanya boleh memakai dokter/rumah sakit/department/afiliasi/jadwal
  yang aktif. Urutan dan arti filter harus tetap deterministik; perubahan ranking
  harus disertai test dan pembaruan dokumentasi.
- `available_on` menunjukkan adanya jadwal praktik aktif yang cocok. Nilai itu
  bukan jaminan kapasitas appointment masih tersedia.

### Appointment, pemeriksaan, dan resep

- Create/reschedule appointment dan walk-in harus mempertahankan idempotensi.
  Jangan gunakan ulang idempotency key untuk operasi berbeda.
- Alokasi slot, kapasitas, nomor appointment/antrean, walk-in, check-in, dan klaim
  patient record memakai transaction serta advisory/row lock. Jangan memecah
  validasi dan write menjadi query terpisah yang membuka race double booking.
- Patient record dipisahkan dari akun auth. Claim harus memverifikasi identitas
  dan tidak boleh mengambil alih record yang sudah dimiliki orang lain.
- Identitas pasien sensitif tetap berada dalam body `POST`, bukan query string.
- Rekam medis final, revisi vital/konsultasi, resep issued, dokumen, dan audit
  dipertahankan. Koreksi harus menambah revisi/audit sesuai model, bukan menimpa
  histori final.
- File medis dan kontrak berada di bucket private. API hanya memberi signed URL
  dengan TTL yang dikonfigurasi. Jangan mengembalikan service key atau object URL
  privat permanen.

## Database dan migration

- Buat migration di `migration/db/` melalui
  `go run . migrate --action create --name <snake_case_name>` atau target Make.
- Nama file memakai timestamp Goose yang lebih besar dari migration terbaru.
- Migration `Up` baru harus aman terhadap data yang sudah ada. Sejumlah migration
  domain sengaja irreversible; jangan menambah downgrade destruktif untuk data
  klinis.
- Bila menambah tabel, column, constraint, trigger, atau function, perbarui model,
  repository, test database, dokumentasi, dan grant runtime minimum yang relevan.
- Proses web memakai `DATABASE_DSN` dengan role DML least-privilege. Migration
  staging/production memakai `DATABASE_ADMIN_DSN` pada proses terpisah. Jangan
  memberi credential owner kepada proses server.
- Supabase Shared Session Pooler port 5432 didukung untuk migration. Transaction
  Pooler port 6543 ditolak. Staging/production memakai `sslmode=verify-full`.
- Jangan menjalankan reset, truncate, restore, atau migration eksperimental pada
  database non-disposable. Reset staging hanya melalui command yang dijaga dan
  confirmation token yang diminta command.
- Seeder development harus idempoten. Jika kontrak baru memerlukan fixture agar
  flow bisa diuji, perbarui seeder dan integration test bersamaan.

## Cara mengimplementasikan perubahan API

Untuk perubahan endpoint, kerjakan seluruh bagian yang memang terdampak:

1. Temukan route, middleware, controller, service interface/implementation,
   repository, DTO request, dan DTO response yang ada.
2. Tetapkan otorisasi, tenant scope, state transition, transaksi, lock,
   idempotensi, pagination, dan bentuk empty result sebelum menulis kode.
3. Tambahkan atau ubah entity/migration bila persistence berubah.
4. Tambahkan kode sukses/error bilingual yang spesifik.
5. Implementasikan repository dengan context dan scope aktif yang benar.
6. Implementasikan aturan bisnis di service dan mapping HTTP di controller.
7. Daftarkan route beserta middleware paling ketat yang sesuai.
8. Perbarui seeder bila flow demo atau data referensi berubah.
9. Tambahkan test yang membuktikan behavior penting: izin, tenant isolation,
   lifecycle, konflik, transaction/concurrency, atau mapping error. Hindari test
   yang hanya menyalin implementasi tanpa menangkap risiko.
10. Perbarui dokumentasi domain dan Bruno untuk setiap kontrak yang terlihat
    client.

Jika response dokter berubah, cari seluruh response nested yang memuat dokter,
bukan hanya endpoint direktori. Jika lifecycle atau jadwal berubah, periksa
invitation, affiliation, appointment availability, recommendation, dan approval
flow agar representasinya konsisten.

## Testing dan validasi

Pemeriksaan minimum untuk perubahan Go:

```powershell
gofmt -w <file-go-yang-diubah>
go test ./... -count=1
go vet ./...
git diff --check
```

GNU Make dapat menjalankan `make check`, tetapi PowerShell Windows tidak selalu
memiliki `make`. Padanan command yang didukung:

```powershell
go run . migrate --action up
go run . seed
go run . server
```

CI juga memeriksa `gofmt`, `go vet`, `go test -race -cover ./...`, dan
`govulncheck`. Integration test database hanya boleh memakai database disposable
dengan `TEST_DATABASE_DSN` dan confirmation reset test yang benar. Gunakan Redis
database test terpisah melalui `TEST_REDIS_DSN`.

Sesuaikan validasi dengan risiko:

- perubahan query/migration/seeder: jalankan integration test PostgreSQL;
- perubahan Redis/auth/session: jalankan test dengan instance Redis test;
- perubahan concurrency appointment: jalankan test transaction/race terkait;
- perubahan upload/storage: uji MIME, magic bytes, ukuran, ownership, dan signed
  URL tanpa menyimpan credential;
- perubahan docs/Bruno YAML: parse semua YAML yang diubah dan periksa saved
  example terhadap response backend.

Jangan menganggap unit test yang lolos membuktikan migration dapat diterapkan.
Untuk perubahan schema, jalankan seluruh migration dari database kosong atau
fixture disposable dan jalankan seeder setelahnya.

## Bruno adalah repo terpisah

`bruno/bruno-medikaone` mempunyai `.git` dan branch sendiri. Perubahan di sana
tidak muncul pada `git status` repo backend. Untuk setiap API yang ditambah atau
diubah:

- perbarui request pada client yang relevan, `Mobile`, `Website`, atau keduanya;
- tambahkan semua header/query/path/body field dengan deskripsi native;
- beri komentar `//` pada setiap field body request untuk status wajib/opsional,
  enum, format, batas, dan sumber variable;
- tambahkan/update saved example dengan JSON response valid tanpa komentar;
- gunakan dummy token, UUID, URL, dan timestamp;
- simpan variable hasil response yang diperlukan request berikutnya;
- perbarui `flows/` atau README folder bila urutan/state transition berubah;
- jangan pernah menyimpan password, token nyata, database DSN, atau Supabase
  secret key di koleksi.

Validasi kedua repo dan commit secara terpisah. Jika pengguna sudah meminta push,
push branch backend dan branch Bruno, lalu laporkan dua commit SHA dan hasil test.
Jangan memasukkan repo Bruno ke commit backend karena folder tersebut di-ignore.

## Git dan penyelesaian tugas

- Ikuti branch aktif kecuali pengguna meminta branch/worktree lain.
- Commit hanya file yang terkait tugas. Periksa `git diff --stat`, `git diff`, dan
  `git status` sebelum commit.
- Gunakan pesan commit singkat dengan pola yang sudah dipakai repo, misalnya
  `feat(schedule): ...`, `fix(invitation): ...`, atau `docs(agent): ...`.
- Jangan force-push, rewrite history, atau menghapus perubahan pengguna.
- Push hanya ketika telah diotorisasi oleh pengguna. Otorisasi yang sudah
  diberikan dalam percakapan tetap berlaku untuk rangkaian kerja yang sama.
- Setelah push, bandingkan HEAD lokal dan remote untuk memastikan commit terkirim.

Tugas API dianggap selesai bila kode, schema, seeder, kontrak response/error,
otorisasi, test relevan, dokumentasi, dan Bruno sudah sinkron; pemeriksaan yang
sesuai risiko lulus; serta status kedua repo hanya menyisakan perubahan pengguna
yang tidak terkait.

## Sumber kebenaran cepat

- Route aktif: `internal/transport/http/http.transport.go`
- Kontrak error: `internal/constant/custom_error.constant.go` dan
  `docs/api-error-contract.md`
- Pesan sukses: `internal/constant/message.constant.go`
- Konfigurasi resmi: `config.yml.example`
- Migration aktif: `migration/db/`
- Aturan lifecycle/jadwal: `docs/resource-lifecycle-and-schedules.md`
- Direktori/rekomendasi: `docs/hospital-directory.md`
- Migrasi Supabase: `docs/supabase-migration.md`
- Contoh request/response: `bruno/bruno-medikaone/`
