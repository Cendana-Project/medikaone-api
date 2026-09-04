# MedikaOne API

Backend monolitik MedikaOne berbasis Go, Gin, PostgreSQL, Redis, dan SMTP. PostgreSQL menyimpan data utama dan relasi tenant rumah sakit; Redis wajib tersedia untuk challenge PIN, rate limit, rotasi refresh token, session version, dan blacklist access token.

## Menjalankan secara lokal

Prasyarat:

- Go sesuai versi pada `go.mod`;
- PostgreSQL;
- Redis lokal atau Redis Cloud;
- akun SMTP jika alur registrasi/reset password ingin digunakan;
- GNU Make jika ingin menggunakan target pada `Makefile`.

Salin konfigurasi contoh:

```bash
cp config.yml.example config.yml
```

Sesuaikan DSN PostgreSQL dan Redis, lalu jalankan:

```bash
make dev
```

`make dev` menjalankan migration, seeder idempotent, lalu server. Alur yang sama
dapat dijalankan terpisah dengan `make setup` kemudian `make server`.

Alternatif PowerShell tanpa GNU Make:

```powershell
Copy-Item config.yml.example config.yml
go run . migrate --action up
go run . seed
go run . server
```

Perintah Make utama:

| Perintah | Fungsi |
| --- | --- |
| `make help` | Menampilkan seluruh command yang tersedia |
| `make server` / `make run` | Menjalankan API menggunakan `go run` |
| `make setup` | Menjalankan migration dan seeder lokal |
| `make dev` | Menjalankan setup lalu server untuk development lokal |
| `make build` | Membuat binary `medikaone` atau `medikaone.exe` |
| `make start` | Build lalu menjalankan binary server |
| `make migrate` / `make migrate-up` | Menjalankan seluruh pending migration |
| `make migrate-up-one` | Menjalankan satu pending migration |
| `make migrate-up-to VERSION=...` | Menjalankan migration sampai versi tertentu |
| `make migrate-status` | Menampilkan status Goose migration |
| `make migrate-create NAME=...` | Membuat file migration baru |
| `make seed` | Menjalankan seeder development secara idempotent |
| `make check` | Menjalankan `go vet` dan seluruh test |
| `make test-cover` | Menjalankan test dengan laporan coverage |
| `make test-race` | Menjalankan race detector dan coverage |

`make test-race` memerlukan compiler C yang kompatibel dengan CGO. Target ini
tetap dijalankan oleh CI Linux apabila compiler tersebut belum tersedia di Windows.

Target `dev` dan `setup` hanya untuk development lokal. Pada staging/production,
jalankan `make migrate-up` dari job/operator terpisah sebelum menjalankan web
server; jangan memberikan `DATABASE_ADMIN_DSN` kepada proses server.

DBeaver hanya berfungsi sebagai client database. Buat/connect database PostgreSQL melalui DBeaver, tetapi migration dan seeder tetap dijalankan dari command aplikasi di atas.

Server default berjalan pada `http://localhost:8080`. Endpoint operasional:

- `GET /ping` - pemeriksaan HTTP sederhana;
- `GET /_internal/livez` - liveness;
- `GET /_internal/readyz` - readiness PostgreSQL dan Redis;
- `GET /_internal/healthz` - alias readiness untuk hosting lama.

Kontrak kode serta pesan sukses/error API didokumentasikan di
[`docs/api-error-contract.md`](docs/api-error-contract.md). Client harus
melakukan branching menggunakan kode pada `message` dan menampilkan bahasa
yang sesuai dari `message_detail`.

## Konfigurasi

Untuk development gunakan `config.yml.example`. `config.yml` dan file `.env*` diabaikan Git agar secret tidak ikut ter-commit.

Environment variable utama untuk deployment:

| Variable | Keterangan |
| --- | --- |
| `ENV` | `development`, `test`, `staging`, atau `production` |
| `SERVER_PORT` | Port HTTP; jika kosong aplikasi juga menerima `PORT` dari hosting |
| `SERVER_CORS_ALLOWED_ORIGINS` | Origin frontend eksak, dipisahkan koma; wildcard ditolak |
| `SERVER_CLIENT_IP_HEADER` | Header IP dari reverse proxy tepercaya; gunakan `X-Forwarded-For` pada Render |
| `DATABASE_DSN` | DSN proses web dengan user runtime least-privilege; boleh memakai endpoint Neon pooled, dan staging/production wajib `sslmode=verify-full` |
| `DATABASE_ADMIN_DSN` | DSN owner/migration direct khusus command migration/reset; tidak diperlukan dan jangan diberikan kepada proses web |
| `STAGING_DATABASE_FINGERPRINT` | Fingerprint `DATABASE_ADMIN_DSN` yang wajib cocok sebelum reset staging |
| `REDIS_CACHE_DSN` | Redis DSN; staging/production wajib `rediss://` dengan password/ACL dan server Redis wajib memakai `maxmemory-policy=noeviction` |
| `JWT_SECRET` | Secret acak minimal 32 karakter |
| `JWT_ACCESS_TTL` | Default `15m` |
| `JWT_REFRESH_TTL` | Default `720h` dan harus lebih panjang dari access TTL |
| `SMTP_ENABLED` | `true` untuk mengirim PIN |
| `SMTP_HOST`, `SMTP_PORT` | Endpoint SMTP |
| `SMTP_USERNAME`, `SMTP_PASSWORD` | Kredensial SMTP |
| `SMTP_FROM`, `SMTP_FROM_NAME` | Sender yang telah diverifikasi oleh provider |
| `SMTP_TIMEOUT` | Batas satu percobaan SMTP; default `15s` dan harus menyisakan headroom minimal `5s` terhadap `SERVER_WRITE_TIMEOUT` |
| `STORAGE_ENABLED`, `STORAGE_PROVIDER` | Aktifkan private object storage; provider yang didukung saat ini `supabase` |
| `SUPABASE_URL`, `SUPABASE_SECRET_KEY` | URL project dan server-only secret key Supabase; jangan pernah kirim secret key ke client |
| `SUPABASE_STORAGE_BUCKET` | Bucket private kontrak dokter; default `doctor-contracts` |
| `SUPABASE_MEDICAL_STORAGE_BUCKET` | Bucket private lampiran rekam medis; default `medical-records` |
| `SUPABASE_STORAGE_MAX_FILE_SIZE_BYTES` | Maksimum upload; aplikasi membatasi paling tinggi 10 MB |
| `SUPABASE_STORAGE_SIGNED_URL_TTL` | Masa berlaku URL download private; default `5m` |
| `AUTH_PIN_TTL`, `AUTH_PIN_MAX_ATTEMPTS` | Masa berlaku dan batas percobaan PIN |
| `AUTH_PIN_RESEND_COOLDOWN` | Cooldown registrasi/resend PIN |
| `AUTH_MAX_ACTIVE_SESSIONS` | Batas refresh-session aktif per akun (default `10`) |
| `AUTH_PUBLIC_IP_RATE_LIMIT`, `AUTH_PUBLIC_IP_RATE_WINDOW` | Batas gabungan endpoint auth publik per alamat IP |
| `AUTH_LOGIN_RATE_LIMIT`, `AUTH_LOGIN_RATE_WINDOW` | Batas login per identity |
| `AUTH_FORGOT_PASSWORD_RATE_LIMIT`, `AUTH_FORGOT_PASSWORD_RATE_WINDOW` | Batas permintaan reset password |

Nilai pool, timeout server, dan timeout SMTP lain tersedia di `config.yml.example`. AWS dan Stripe tidak dikonfigurasi karena tidak digunakan repo ini.

Validasi konfigurasi mengikuti command: `server` memerlukan konfigurasi web lengkap; `seed`/migration lokal hanya memerlukan bagian database; migration/reset staging juga memerlukan Redis dan write timeout untuk drain; `database-fingerprint` hanya memerlukan admin DSN; dan `migrate --action=create` hanya membuat file lokal tanpa membuka koneksi database. Karena email bersifat opt-in, deployment env-only harus menetapkan `SMTP_ENABLED=true` secara eksplisit saat ingin mengirim PIN. Alias lama `JWT_ACCESS_TTL_MINUTES` dan `JWT_REFRESH_TTL_DAYS` tetap diterima bila nilai duration baru tidak diisi, tetapi sebaiknya migrasikan ke `JWT_ACCESS_TTL` dan `JWT_REFRESH_TTL`.

### Pemisahan koneksi database

Proses web harus menggunakan user PostgreSQL yang hanya mempunyai hak DML per tabel yang benar-benar diperlukan dan **tidak** mempunyai hak `CREATE`, `TRUNCATE`, kepemilikan tabel, atau keanggotaan role owner pada schema `public`. Riwayat migration `goose_db_version` wajib read-only. Pada `ENV=staging` atau `ENV=production`, startup memverifikasi target database/user/schema dan batas privilege tersebut; server gagal menyala jika role runtime terlalu kuat.

Jalankan migration sebagai owner terlebih dahulu, lalu buat/grant user runtime melalui DBeaver. Contoh berikut tidak mengandung credential nyata; sesuaikan nama database dan role owner:

```sql
CREATE ROLE medikaone_app LOGIN PASSWORD 'GANTI_DENGAN_SECRET_ACAK_PANJANG';

GRANT CONNECT ON DATABASE medikaone TO medikaone_app;
GRANT USAGE ON SCHEMA public TO medikaone_app;

-- Bersihkan grant lama yang terlalu luas sebelum memberi allowlist runtime.
REVOKE ALL PRIVILEGES ON ALL TABLES IN SCHEMA public FROM medikaone_app;
REVOKE ALL PRIVILEGES ON ALL SEQUENCES IN SCHEMA public FROM medikaone_app;

GRANT SELECT, INSERT, UPDATE ON TABLE public.users TO medikaone_app;
GRANT SELECT ON TABLE public.roles, public.permissions, public.role_permissions TO medikaone_app;
GRANT SELECT, INSERT ON TABLE public.user_roles TO medikaone_app;
GRANT SELECT, INSERT, UPDATE ON TABLE public.patient_profiles, public.doctor_profiles TO medikaone_app;
GRANT SELECT, INSERT ON TABLE public.hospitals, public.hospital_user_roles TO medikaone_app;
GRANT SELECT, INSERT, UPDATE ON TABLE public.user_hospitals TO medikaone_app;
GRANT SELECT ON TABLE public.goose_db_version TO medikaone_app;

REVOKE INSERT, UPDATE, DELETE, TRUNCATE ON TABLE public.goose_db_version FROM medikaone_app;
REVOKE CREATE ON SCHEMA public FROM medikaone_app;
REVOKE CREATE ON SCHEMA public FROM PUBLIC;

SELECT has_schema_privilege('medikaone_app', 'public', 'CREATE') AS must_be_false;
```

Jalankan blok tersebut sebagai `migration_owner`/owner database dan pastikan `medikaone_app` bukan anggota role owner mana pun. Grant sengaja berupa allowlist tanpa `ALTER DEFAULT PRIVILEGES`: setiap migration yang menambah tabel harus disertai review lalu grant minimum eksplisit untuk tabel baru. Struktur saat ini memakai UUID sehingga proses web tidak memerlukan privilege sequence.

Pada Neon gunakan dua connection string yang menuju branch/database yang sama:

- `DATABASE_DSN`: endpoint **pooled** (hostname biasanya berakhiran `-pooler`) dengan user `medikaone_app`;
- `DATABASE_ADMIN_DSN`: endpoint **direct/non-pooler** dengan role owner, dan hanya tersedia pada proses migration/reset.

Keduanya harus memakai satu host target, tanpa daftar multi-host/fallback, serta `sslmode=verify-full` pada staging/production. Guard migration/reset menolak DSN admin dengan hostname yang dikenali sebagai pooled (`-pooler` atau `pgbouncer`) maupun DSN fallback, lalu memastikan target efektifnya sama dengan DSN aplikasi. Karena provider lain dapat memakai pola hostname berbeda, operator tetap wajib memilih endpoint direct. Penanda `-pooler` Neon saja yang dinormalisasi saat membandingkan host. Neon juga [merekomendasikan koneksi direct untuk migration](https://neon.com/docs/connect/connection-pooling).

### Redis

Untuk Redis Cloud, aktifkan TLS pada database Redis dan gunakan connection string `rediss://...`. Jangan mengganti skema menjadi `rediss://` jika TLS belum diaktifkan pada provider. Atur **Data eviction policy** menjadi `noeviction`; pada staging/production aplikasi membaca `INFO memory` saat startup dan gagal tertutup jika policy tidak dapat diverifikasi atau nilainya berbeda. Tetap pasang alarm kapasitas: dengan `noeviction`, Redis yang penuh akan menolak write baru dan alur login/challenge/session dapat terganggu.

Database gratis Redis Cloud ditujukan untuk development/prototipe, dibatasi sekitar 30 MB, dan tidak menyediakan persistence. Kehilangan data Redis akan membatalkan challenge, rate-limit state, dan sesi/refresh token aktif sehingga pengguna perlu login ulang. Gunakan ini untuk staging gratis, bukan sebagai jaminan availability atau durability production. Lihat dokumentasi resmi tentang [free database](https://redis.io/docs/latest/operate/rc/databases/create-database/create-free-database/), [eviction policy](https://redis.io/docs/latest/operate/rc/databases/configuration/data-eviction-policies/), dan [persistence](https://redis.io/docs/latest/operate/rc/databases/configuration/data-persistence/).

Namespace Redis dipisahkan berdasarkan environment serta host, port, database, dan `search_path` PostgreSQL tanpa memasukkan credential. Perubahan dari versi lama menambahkan port ke identitas namespace, sehingga deploy upgrade ini sengaja membuat sesi/challenge lama tidak terlihat dan pengguna perlu login ulang. Karena free Redis tidak persistent, restart provider saat maintenance dapat menghilangkan sentinel; jangan menjalankan reset ketika Redis tidak stabil, dan ulangi command sejenis bila readiness atau proses reset terputus.

Jika `SMTP_ENABLED=false`, server tetap dapat dijalankan untuk endpoint non-email, tetapi registrasi akan gagal mengirim challenge dan alur forgot-password tidak akan mengirim PIN. PIN hanya disimpan sebagai hash sehingga tidak dapat diambil dari Redis untuk pengujian manual.

## Alur registrasi

Registrasi tidak lagi membuat row user sebelum pemilik email membuktikan PIN. Ini mencegah password dari registrasi palsu tertinggal pada akun `pending`.

1. `POST /v1/auth/register` menerima `email`, `username`, `phone`, dan `password`, lalu mengembalikan `challenge_id`.
2. `POST /v1/auth/verify-pin` menerima `email`, `challenge_id`, dan `pin`. User baru dibuat dan token diterbitkan hanya jika PIN valid.
3. `POST /v1/auth/resend-pin` menerima `email` dan `challenge_id` yang sama.

Contoh request verifikasi:

```json
{
  "email": "user@example.com",
  "challenge_id": "challenge-dari-register",
  "pin": "123456"
}
```

`challenge_id` wajib dikirim pada verify dan resend. Challenge terikat pada payload registrasi tertentu sehingga PIN tidak dapat dipakai untuk payload registrasi lain.

Endpoint auth lain:

- `POST /v1/auth/login`
- `POST /v1/auth/login/hospital`
- `POST /v1/auth/refresh`
- `POST /v1/auth/password/forgot`
- `POST /v1/auth/password/verify-pin`
- `POST /v1/auth/password/reset`
- `PUT /v1/auth/password` (Bearer access token)
- `POST /v1/auth/logout` (Bearer access token; mencabut satu keluarga sesi, body `refresh_token` opsional sebagai pemeriksaan konsistensi)
- `POST /v1/auth/logout-all` (Bearer access token)

Reset password menggunakan tiga tahap. `POST /v1/auth/password/forgot` menerima `email` dan selalu mengembalikan bentuk respons yang sama, termasuk `challenge_id`, agar keberadaan akun tidak bocor.

Kirim PIN dari email ke `POST /v1/auth/password/verify-pin`:

```json
{
  "challenge_id": "challenge-dari-password-forgot",
  "email": "user@example.com",
  "pin": "123456"
}
```

Jika PIN valid, field `data` pada respons berisi grant reset yang singkat masa berlakunya:

```json
{
  "status": "pin_verified",
  "reset_token": "token-reset-sekali-pakai",
  "expires_in": 600
}
```

Kirim `challenge_id` dan `reset_token` tersebut bersama password baru ke `POST /v1/auth/password/reset`:

```json
{
  "challenge_id": "challenge-dari-password-forgot",
  "reset_token": "token-reset-sekali-pakai",
  "new_password": "Password-Baru-Yang-Kuat!"
}
```

`reset_token` hanya dapat digunakan sekali dan tidak boleh disimpan di log, local storage, atau cache. Respons verifikasi dikirim dengan `Cache-Control: no-store`. PIN tidak lagi diterima oleh endpoint reset; client harus menyelesaikan tahap verifikasi terlebih dahulu. Token reset otomatis tidak berlaku jika password user berubah melalui alur lain. Reset yang berhasil mencabut seluruh access token dan refresh token lama milik user. Setelah versi tiga tahap ini pertama kali di-deploy, proses reset yang dimulai pada versi lama harus diulang dari endpoint `forgot`.

Refresh token bersifat one-time-use. Setiap operasi refresh wajib membawa `idempotency_key` berupa UUIDv4 acak yang baru:

```json
{
  "refresh_token": "refresh-token-saat-ini",
  "idempotency_key": "0f2663f8-8151-48f1-8aa2-0b99ebfbd86d"
}
```

Client harus memakai key yang **sama** ketika mengulang operasi refresh yang responsnya tidak diterima, tetapi membuat UUIDv4 baru untuk refresh berikutnya. Key berbeda untuk refresh token lama yang sama diperlakukan sebagai replay dan dapat mencabut keluarga sesi. Refresh dan access token lama yang diterbitkan sebelum claim family `fid` tersedia sengaja tidak kompatibel untuk refresh/logout setelah upgrade; lakukan login ulang. Logout biasa mencabut seluruh refresh token pada keluarga perangkat tersebut secara atomik, sedangkan password reset/change dan logout-all menaikkan session version sehingga seluruh access/refresh token lama tidak berlaku lagi.

Pengiriman email forgot-password memakai antrean terbatas di dalam proses aplikasi. Antrean ini tidak durable: respons `pin_sent` berarti pekerjaan diterima tanpa menjamin SMTP telah mengirimnya, dan restart/crash dapat menghilangkan pekerjaan yang belum diproses. Untuk production, pindahkan pekerjaan email ke transactional outbox atau antrean durable dengan worker terpisah.

## Tenant rumah sakit

Operasi tenant menggunakan `:hospital_id` pada path atau header `X-Hospital-ID`/`X-Hospital-Code` untuk endpoint yang mendukungnya. Nilai `hospital_id` pada JSON diabaikan; server selalu menggunakan tenant yang telah diotorisasi.

- Hanya `SUPER_ADMIN` aktif yang dapat membuat hospital atau hospital admin.
- Hospital admin aktif hanya dapat membuat staff pada hospital tempat ia masih menjadi member aktif.
- Endpoint staff menerima `DOCTOR`, `NURSE`, `RECEPTIONIST`, atau `BOD`; role `ADMIN` harus melalui endpoint khusus super admin.

## Check-in resepsionis dan walk-in

Check-in bukan self-service. Hanya petugas rumah sakit dengan permission
`appointment.checkin` yang dapat mencari appointment dan mengonfirmasi
kedatangan. Pencarian tersedia melalui QR opsional, pasangan nomor appointment
dan kode verifikasi, atau minimal dua fakta identitas. Pencarian identitas memakai
body `POST` agar data pribadi tidak masuk query string/access log.

Alur yang direkomendasikan:

1. `POST /v1/hospitals/:hospital_id/appointments/check-in/lookup` mengembalikan
   kandidat yang telah dimasking dan `check_in_token` berumur lima menit serta
   terikat pada petugas, rumah sakit, dan appointment.
2. Petugas memeriksa identitas pasien lalu memanggil
   `POST /v1/hospitals/:hospital_id/appointments/:appointment_id/check-in`.
3. Status menjadi `WAITING_VITALS`, attendance menjadi `PRESENT`, dan nomor
   antrean yang telah dibuat saat booking diaktifkan.

Window normal adalah 30 menit sebelum hingga 15 menit sesudah waktu mulai.
Sesudah batas tersebut atau dari status `NO_SHOW`, check-in tetap dapat
dilakukan sampai pukul 23:59:59 pada tanggal appointment menurut timezone
jadwal, termasuk setelah sesi praktik berakhir, tetapi `override_reason` wajib
diisi dan tersimpan pada audit trail. Endpoint satu tahap lama tetap tersedia
sementara untuk kompatibilitas client.

Resepsionis/admin dapat membuat appointment walk-in hari berjalan melalui
`POST /v1/hospitals/:hospital_id/walk-in-appointments`. Pasien dapat ditunjuk
dengan `patient_record_id`, `medikaone_id`, atau data identitas lengkap. Consent
dicatat sebagai `RECEPTIONIST_INFORMED`, appointment langsung masuk
`WAITING_VITALS`, dan mendapat nomor antrean berikutnya tanpa prioritas.
Kapasitas tetap berlaku; hanya admin/super admin dapat memakai
`capacity_override=true` dan wajib memberikan alasan.

Pasien tanpa akun disimpan pada `patient_records`. Setelah mempunyai akun
PATIENT, pasien dapat memanggil `POST /v1/patient-records/claim` dengan identitas
dan tanggal lahir. Klaim memerlukan kecocokan NIK/MedikaOne ID; untuk
PASSPORT/OTHER juga diperlukan kecocokan email atau telepon akun. Semua
appointment guest pada record tersebut kemudian ditautkan ke akun secara
atomik.

## Pemeriksaan dan rekam medis

Alur pemeriksaan menggunakan status appointment yang sudah ada:
`WAITING_VITALS` -> `WAITING_DOCTOR` -> `IN_CONSULTATION` -> `COMPLETED`.
Perawat menyimpan draft vital terlebih dahulu dan finalisasi dilakukan dalam
transaksi yang sama dengan perpindahan ke antrean dokter. Skala nyeri tidak
dikumpulkan. BMI dihitung oleh database dari tinggi dan berat sehingga client
tidak dapat mengirim nilai BMI sendiri.

Dokter menyimpan catatan SOAP (`subjective`, `objective`, `assessment`, dan
`plan`) sebagai draft. Penyelesaian appointment ditolak sampai seluruh SOAP dan
tepat satu diagnosis utama tersedia. Catatan yang telah final bersifat
append-only: koreksi vital atau konsultasi membuat revision baru dengan alasan
wajib dan mempertahankan revision sebelumnya. Trigger PostgreSQL juga menolak
update/delete terhadap revision final dan diagnosis terkait, sehingga aturan
ini tidak hanya bergantung pada handler API. Admin rumah sakit dapat membuat
koreksi konsultasi melalui endpoint tenant; perawat hanya dapat mengoreksi
vital. `internal_note` tetap khusus dokter: koreksi admin mempertahankan nilainya
tanpa dapat membaca atau menimpanya.

Riwayat lintas rumah sakit hanya tersedia bagi dokter yang ditugaskan pada
appointment ber-consent dan pasien pemilik record. Resepsionis tidak mendapat
permission pemeriksaan. Dokter dapat membuka detail encounter historis pasien
yang sama, tetapi `internal_note` dokter sebelumnya tidak pernah diteruskan.
Setiap baca, perubahan, koreksi, upload, dan pembuatan signed URL dicatat pada
`medical_record_audit_events`.

Lampiran PDF, JPEG, dan PNG disimpan dalam bucket Supabase private
`medical-records`, maksimum 10 MB, memakai object path per rumah sakit, pasien,
dan encounter. API hanya mengembalikan signed URL berdurasi pendek setelah
otorisasi; object storage tidak boleh dibuat public. Bucket rekam medis wajib
terpisah dari bucket kontrak dokter.

## Seeder dan reset staging

Credential akun fixture privileged memang sengaja hardcoded untuk mempermudah development/staging saat ini. Ini adalah keputusan desain sementara, bukan secret production; command `seed` menolak berjalan pada `ENV=production`.

Seeder juga menerima akun environment-managed melalui env-only `SUPERADMIN_EMAIL`, `SUPERADMIN_PASSWORD`, `SUPERADMIN_FIRST_NAME`, dan `SUPERADMIN_LAST_NAME`. Jika email diisi, password wajib diisi. Email canonical `superadmin@medikaone.id` mengganti detail/password fixture tersebut; email lain menambahkan satu akun superadmin fixture di samping akun development hardcoded yang memang dipertahankan by design. Berikan variabel ini hanya kepada job seed/reset, bukan proses web, dan perlakukan password-nya sebagai secret yang dikelola serta dirotasi.

Seed idempotent biasa hanya untuk `ENV=development` atau `ENV=test`, dan command menolak DSN non-loopback/fallback agar label environment yang salah tidak memasang akun demo pada database remote:

```bash
make seed
```

Reset seluruh staging menghapus seluruh data aplikasi, menjalankan migration `Up`, lalu seed ulang:

```bash
make staging-reset-all CONFIRM=RESET-ALL-STAGING-DATA
```

Reset fixture demo saja:

```bash
make staging-reset-seed CONFIRM=RESET-DEMO-STAGING-DATA
```

Nilai `CONFIRM` wajib ditulis pada command line invocation yang sama. Makefile menolak nilai yang berasal dari environment, jadi jangan menyimpan atau mengekspor `CONFIRM` pada service maupun job.

Reset demo hanya menghapus user yang memiliki `seed_key` internal milik fixture; email atau code publik tidak pernah dipakai sebagai bukti kepemilikan. Data turunan akun fixture ikut terhapus melalui foreign key, sedangkan user nyata, role assignment mereka, membership mereka, dan hospital tetap dipertahankan. Hospital/RBAC bawaan disinkronkan berdasarkan identitas internal yang sama.

Migration tidak menandai fixture lama berdasarkan email/code karena itu dapat salah mengklaim data nyata. Setelah upgrade database lama, lakukan satu kali `staging-reset-all` untuk membentuk provenance fixture dari keadaan kosong. `staging-reset-seed` akan gagal aman jika menemukan row lama tanpa `seed_key` yang memakai identitas fixture; selesaikan konflik secara manual atau lakukan full reset jika staging memang boleh dikosongkan.

Sebelum reset pertama, hitung fingerprint menggunakan konfigurasi staging lalu simpan hasilnya sebagai environment variable `STAGING_DATABASE_FINGERPRINT` pada job/operator reset terpisah, bukan pada proses web:

```bash
make staging-db-fingerprint
```

Kedua reset staging menolak berjalan kecuali `ENV=staging`, `DATABASE_DSN` dan `DATABASE_ADMIN_DSN` menunjuk lokasi/database/routing efektif yang sama, fingerprint pgx admin (host, database, user admin, dan runtime parameter) cocok, koneksi aktual berada pada schema `public` dengan user admin yang dikonfigurasi, dan frasa konfirmasinya cocok persis. User database aplikasi boleh berbeda dari user migration agar runtime tetap least-privilege. Untuk Neon, perbandingan endpoint hanya menormalkan penanda hostname `-pooler`; database, port, dan runtime parameter tetap harus sama. DSN admin wajib mempunyai tepat satu target tanpa fallback. Ini mencegah query DSN mengalihkan target maupun DSN production yang tidak sengaja ditempel pada service staging.

`staging-reset-all` mempertahankan schema dan riwayat Goose. Ia menjalankan clear transaksional atas allowlist tabel aplikasi, migration `Up`, lalu satu transaksi final yang melakukan `TRUNCATE` lagi dan seed ulang. Truncate kedua membuang write dari client database langsung atau deployment lama yang masih sempat masuk selama migration. Setelah seed berhasil dan maintenance masih aktif, seluruh challenge, rate limit, session, blacklist, dan refresh state dalam namespace autentikasi Redis target dibersihkan; semua client harus login ulang dan challenge lama tidak dapat membuat ulang user. Dependency tabel yang tidak dikenal membuat truncate gagal; reset tidak memperluas penghapusan secara diam-diam. Urutan clear-before-migrate dapat memulihkan staging lama yang migration uniqueness-nya terhalang data duplikat. Keseluruhan urutan tidak atomik sebagai satu unit, sehingga staging dapat kosong bila proses terputus setelah clear.

Sebelum operasi pertama yang dapat commit, migration production/staging dan kedua reset mengubah maintenance lease menjadi sentinel Redis bertipe tanpa TTL. Jika operasi gagal atau proses mati, readiness tetap HTTP 503 dan request aplikasi tetap ditolak selama Redis mempertahankan data sentinel tersebut. Redis tanpa persistence tidak dapat memberi jaminan fail-closed lintas restart provider; setelah gangguan Redis saat maintenance, periksa keadaan database dan ulangi command sejenis sebelum membuka traffic. Rerun command sejenis (`migrate`, `staging-reset-all`, atau `staging-reset-seed`) dapat mengambil alih sentinel jenisnya setelah memperoleh lock database. Sebagai jalur pemulihan eksplisit dengan konfirmasi paling kuat, `staging-reset-all` juga boleh mengambil alih kegagalan migration/reset-seed agar data legacy yang menghalangi migration dapat dibersihkan; arah sebaliknya tetap ditolak. Jangan menghapus key maintenance secara manual. Perubahan data `staging-reset-seed` sendiri tetap atomik sehingga kegagalan seed me-rollback data fixture.

Migration `Up` pada staging/production dan kedua reset staging terlebih dahulu mengambil maintenance lease Redis yang diperbarui berkala. Semua instance yang memakai namespace environment/database Redis yang sama menolak request aplikasi baru dengan HTTP 503, lalu command menunggu request aktif (yang juga memperbarui TTL counter) selesai sebelum menyentuh database. Endpoint `/ping` dan `/_internal/livez` tetap tersedia, sedangkan readiness `/_internal/readyz` dan `/_internal/healthz` bernilai 503 selama maintenance. Setelah itu operasi memakai transaction-scoped advisory lock yang sama pada satu koneksi direct khusus; heartbeat membatalkan operasi jika koneksi penjaga (dan karena itu lock) hilang, dan operasi paralel gagal cepat. Pastikan command terpisah memakai `REDIS_CACHE_DSN` yang sama dengan web staging agar proses drain benar-benar mencakup semua instance.

Migration hardening memasang trigger database yang membuat `seed_key` immutable dan hanya dapat dibuat oleh owner/migration role. Dengan demikian role web tidak dapat menandai user atau hospital nyata sebagai fixture agar ikut terhapus oleh reset demo.

Migration hardening terbaru sengaja irreversible. Command `migrate down`, `down-to`, dan `reset` generik ditolak, dan bagian Goose `Down` migration tersebut juga gagal dengan pesan eksplisit karena mengembalikan constraint lama dapat merusak data soft-delete yang valid.

## Quality checks

```bash
make fmt
make check
make test-race
make vuln
```

CI memeriksa format, `go vet`, race-enabled tests, `govulncheck`, serta integration test pada service PostgreSQL disposable. Test database destruktif hanya aktif dengan DSN test dan token konfirmasi khusus. Regression test migration juga menolak operasi penghancur schema/table di bagian Goose `Up`.

## Catatan deployment

- Jangan memasukkan credential ke Git atau log.
- Gunakan sender email yang telah diverifikasi; `SMTP_FROM` palsu akan ditolak provider seperti SendGrid.
- Setelah credential pernah dibagikan di chat/log, rotasi password database, password Redis, API key SMTP, dan seluruh token/JWT secret sebelum penggunaan nyata.
- Untuk server staging, set `ENV=staging`; jangan memakai `production` jika ingin menggunakan command reset staging yang dijaga.
- Kode baru membaca kolom dari migration `20260901010000_harden_user_hospitals.sql`, jadi migration wajib selesai sebelum server baru menerima traffic.
- Kontrak auth `/v1` berubah (`challenge_id`, refresh `idempotency_key`, dan claim token baru). Koordinasikan backend dan client sebagai hard cutover, jangan menjalankan versi lama dan baru bersamaan, lalu minta semua pengguna login ulang.
- Proses web Render hanya menjalankan server: build command `go build -o medikaone-api .` dan start command `./medikaone-api server`. Berikan `DATABASE_DSN` least-privilege kepada web service dan **jangan** menyimpan `DATABASE_ADMIN_DSN` di environment web.
- Jalankan `make migrate-up` secara terpisah dari mesin/operator tepercaya, CI job terisolasi, atau mekanisme deployment terpisah. Proses tersebut saja yang menerima `DATABASE_ADMIN_DSN` direct dan Redis staging. Jangan menggabungkan migration dengan start command memakai `&&`: restart/scale web tidak boleh otomatis memperoleh kredensial owner atau menjalankan DDL. Render mendokumentasikan [alur deploy](https://render.com/docs/deploys); untuk paket gratis yang tidak menyediakan pre-deploy command, jalankan migration manual sebelum deploy web.
- Migration tersebut sengaja berhenti bila menemukan email/username/NIK aktif atau code/name hospital aktif yang duplikat menurut aturan uniqueness barunya. Perbaiki konflik data secara eksplisit lalu ulangi migration; migration tidak memilih row secara diam-diam. Identifier milik row yang sudah soft-delete dapat dipakai ulang.
- Render Free memblok koneksi SMTP keluar pada port `25`, `465`, dan `587`; untuk SendGrid gunakan port `2525`. Nilai `587` di `config.yml.example` hanya contoh umum untuk development/provider lain. Gunakan `SMTP_TIMEOUT=15s` dengan `SERVER_WRITE_TIMEOUT=30s` atau kombinasi lain yang menyisakan headroom minimal lima detik.
- Pada Render set `SERVER_CLIENT_IP_HEADER=X-Forwarded-For`. Jangan mengaktifkan pembacaan header ini bila aplikasi juga dapat diakses langsung tanpa reverse proxy tepercaya.
