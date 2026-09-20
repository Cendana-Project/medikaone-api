# Pindah PostgreSQL Neon ke Supabase

Backend memakai PostgreSQL biasa, GORM/pgx, dan auth MedikaOne. Migrasi database
tidak mengubah auth menjadi Supabase Auth. `SUPABASE_URL` dan
`SUPABASE_SECRET_KEY` digunakan client Storage. `SUPABASE_PUBLISHABLE_KEY` dan
`SUPABASE_JWKS_URL` tidak digunakan backend ini dan tidak menggantikan password
PostgreSQL. Jangan menimpa konfigurasi aktif dengan nilai placeholder.

## Koneksi yang didukung

Salin host, port, dan username dari **Connect** pada project tujuan.

| Mode | Format username | Port | Pemakaian |
| --- | --- | --- | --- |
| Direct | `medikaone_app` / `postgres` | 5432 | Server dan migration, jika endpoint dapat dijangkau jaringan |
| Shared Session Pooler | `medikaone_app.PROJECT_REF` / `postgres.PROJECT_REF` | 5432 | Server dan migration melalui IPv4 |
| Transaction Pooler | — | 6543 | Ditolak; backend membutuhkan prepared statements dan sesi admin yang tetap |

Hostname direct adalah `db.PROJECT_REF.supabase.co`; shared pooler memakai host
`*.pooler.supabase.com` yang disalin dari dashboard. Username shared pooler memuat
project reference; role aktual PostgreSQL tidak memuat suffix tersebut. Pemeriksaan
koneksi tetap mencocokkan database, session_user/current_user, schema `public`, dan
privilege runtime. Role runtime tidak boleh owner/admin.

Staging/production memerlukan `sslmode=verify-full`. Jika CA endpoint belum
dipercaya oleh mesin, gunakan `sslrootcert` yang sesuai. Password dengan karakter
khusus harus di-URL-encode. Direct dan session endpoint project yang sama dapat
dipakai bersama untuk app/admin; database dan runtime parameter seperti search_path
harus sama. Project berbeda selalu memiliki identitas target dan namespace Redis
berbeda, walaupun memakai host pooler dan nama database yang sama.

Sertifikat CA publik Supabase tersedia di
[`certs/supabase-prod-ca-2021.crt`](../certs/supabase-prod-ca-2021.crt).
Dari root repo, gunakan query parameter berikut pada **kedua** URL PostgreSQL:

```text
?sslmode=verify-full&sslrootcert=certs%2Fsupabase-prod-ca-2021.crt
```

Jika URL sudah memiliki query parameter, gunakan `&` untuk menambahkan parameter
yang belum ada. Path relatif tersebut juga dapat dipakai Render bila start command
berjalan dari root repo; untuk working directory lain gunakan path absolut file
sertifikat di deployment. Jangan menyalin path Windows lokal ke Render.
Sumber, checksum, dan masa berlaku CA dicatat di [`certs/README.md`](../certs/README.md).

## Persiapan target

1. Gunakan project tujuan yang tidak memiliki tabel MedikaOne yang bentrok. Project
   Storage yang sudah ada dapat dipakai setelah memeriksa schema aplikasi.
2. Nonaktifkan **Enable Data API** bila project tidak menggunakan REST/GraphQL data
   Supabase. Akses tabel MedikaOne tetap melalui backend. Jika Data API diperlukan
   aplikasi lain, pertahankan konfigurasi project tersebut dan lindungi tabel
   MedikaOne sebelum transaksi restore di-commit: cabut grant dari `PUBLIC`,
   `anon`, `authenticated`, dan `service_role`; aktifkan RLS dengan policy khusus
   `medikaone_app`. Allowlist grant runtime tetap membatasi operasi per tabel.
   Cabut juga akses sequence migration dan EXECUTE fungsi aplikasi dari role API;
   jangan mengubah fungsi extension atau schema internal Supabase. RLS saja tidak
   cukup untuk `service_role`, karena role tersebut memiliki BYPASSRLS.
3. Gunakan owner untuk restore/migration. Buat login `medikaone_app` dengan password
   terpisah dan grant runtime dari bagian **Pemisahan koneksi database** di README.
   Pada Supabase sesuaikan `GRANT CONNECT ON DATABASE` menjadi `postgres` jika itu
   nama database pada Connect. Role runtime bukan anggota role owner. Bila role
   dibuat setelah migration, jalankan seluruh grant runtime, termasuk tabel baru.
4. Pertahankan konfigurasi Storage yang lama jika project Storage tetap sama.
   Memindahkan PostgreSQL tidak menyalin object Storage. Jika Storage ikut berpindah
   project, salin file/bucket privat beserta object path sebelum mengganti URL/key.

## Jika data Neon dipertahankan

Backup awal boleh diambil saat aplikasi berjalan; backup akhir untuk cutover
memerlukan penghentian semua penulisan ke Neon (server dan worker).

1. Pastikan source Neon masih dapat diakses. Bila quota menutup akses, pulihkan akses
   atau gunakan backup yang tersedia. File dump kosong/gagal tidak boleh di-restore.
2. Ekspor schema aplikasi `public` beserta seluruh data dengan `pg_dump` format custom,
   `--no-owner --no-privileges`. Gunakan binary pg_dump yang mendukung versi source.
   Sertakan tabel `goose_db_version`; UUID, relasi, dan hash password tetap dibawa.
3. Periksa daftar isi archive (`pg_restore --list`) dan bentrok pada target. Jangan
   menghapus schema `auth`, `storage`, atau schema internal Supabase. Sesuaikan daftar
   restore untuk schema `public`/extension yang sudah ada, bukan memakai DROP SCHEMA
   CASCADE atau restore `--clean` tanpa pemeriksaan.
4. Restore archive yang sudah diperiksa memakai owner, `--no-owner --no-privileges`,
   `--exit-on-error`, dan transaksi tunggal bila sesuai ukuran dump. Bila restore
   gagal, selesaikan penyebabnya sebelum mencoba migration.
5. Bandingkan jumlah row tabel utama dan referensi, cek riwayat Goose, lalu jalankan
   migration pending pada target. Full restore dilakukan sebelum migration baru,
   agar tabel lama dan riwayatnya tidak bentrok.

Neon dapat memasang `pgcrypto` di schema `public`, sedangkan Supabase memasangnya
di `extensions`. Periksa referensi extension dalam hasil `pg_restore
--section=pre-data`: default `doctor_profiles.medikaone_id` dari Neon dapat memakai
`public.gen_random_bytes(8)`. Pada target tersebut, sesuaikan menjadi
`extensions.gen_random_bytes(8)` pada DDL saja. Jangan memindahkan extension
Supabase atau melakukan penggantian teks pada bagian COPY/data. Restore pre-data,
data, post-data, dan pembatasan akses dalam satu transaksi; kegagalan harus
me-rollback seluruh import.

Untuk verifikasi isi yang konsisten, ekspor snapshot dari transaksi read-only
`REPEATABLE READ`, jalankan `pg_dump --snapshot=...` sebelum transaksi tersebut
ditutup, dan simpan jumlah row/checksum per tabel dari snapshot yang sama. Bandingkan
hasil restore terhadap manifest tersebut. Verifikasi backup awal bukan pengganti
backup akhir saat seluruh penulisan source telah dihentikan.

Setiap migration berikutnya yang menambah tabel atau fungsi juga harus menyertakan
grant/RLS yang sesuai sebelum backend dibuka kembali. Jangan mengasumsikan default
privilege project Supabase sama dengan PostgreSQL biasa.

Jika pengguna memilih database kosong, lewati dump/restore dan jalankan migration
Up pada target yang telah diperiksa. Seeder demo remote tidak dijalankan otomatis.

## PowerShell tanpa Make

Salin contoh hanya bila file tujuan belum ada:

```powershell
if (-not (Test-Path -LiteralPath .env.supabase-migration)) {
    Copy-Item docs/examples/supabase-migration.env.example .env.supabase-migration
}
```

Isi file secara lokal. `DATABASE_DSN` adalah koneksi user aplikasi;
`DATABASE_ADMIN_DSN` adalah koneksi owner. Isi ENV yang sama dengan target web.
Migration Up juga memerlukan `REDIS_CACHE_DSN` dan `SERVER_WRITE_TIMEOUT` yang sama
dengan web agar maintenance guard memakai konfigurasi yang konsisten.

File env tidak otomatis dibaca command Go. Gunakan helper yang memuat file hanya
untuk durasi command dan memulihkan environment shell sesudahnya:

```powershell
.\scripts\Invoke-DatabaseMigration.ps1 -EnvFile .env.supabase-migration -Action status
# Jalankan Up setelah restore selesai, atau setelah memilih target kosong.
.\scripts\Invoke-DatabaseMigration.ps1 -EnvFile .env.supabase-migration -Action up
.\scripts\Invoke-DatabaseMigration.ps1 -EnvFile .env.supabase-migration -Action status
```

Jika PowerShell lokal menolak script karena execution policy, jalankan pada proses
tersendiri tanpa mengubah policy permanen komputer:

```powershell
powershell.exe -NoProfile -ExecutionPolicy RemoteSigned -File .\scripts\Invoke-DatabaseMigration.ps1 -EnvFile .env.supabase-migration -Action status
```

Helper menerima KEY=VALUE literal, komentar satu baris diawali #, dan quotes
opsional. Tidak ada evaluasi expression, perluasan variabel, atau komentar inline.
Kredensial tidak dicetak. File `.env.*` dan folder `.tmp/` sudah diabaikan Git.

## Cutover Render

1. Backup akhir dan restore dilakukan ketika semua penulisan Neon dihentikan.
   Maintenance pada target Supabase saja tidak menghentikan server yang masih
   memakai namespace/database Neon.
2. Pastikan semua migration applied, grant runtime lengkap, dan Storage terjangkau.
3. Deploy versi kode yang mendukung Supabase, lalu set `DATABASE_DSN` runtime ke
   target. Jangan simpan DATABASE_ADMIN_DSN dalam environment web.
4. Verifikasi readiness, login, list/detail hospital dan dokter, data appointment,
   dan akses signed URL. Semua pengguna login ulang karena namespace auth berubah
   saat pindah dari Neon ke Supabase.
5. Pertahankan backup dan source sampai hasil diverifikasi. Setelah penulisan baru
   masuk Supabase, kembali ke Neon memerlukan rekonsiliasi data; jangan sekadar
   mengganti DSN kembali.

Referensi: [koneksi Supabase](https://supabase.com/docs/guides/database/connecting-to-postgres),
[migrasi Neon](https://supabase.com/docs/guides/platform/migrating-to-supabase/neon),
[pengaturan Data API](https://supabase.com/docs/guides/api/securing-your-api).
