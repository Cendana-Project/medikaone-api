# Direktori dan profil rumah sakit

Migration `20260919090000_hospital_directory.sql` menambahkan profil publik,
galeri, dan review; migration `20260926120000_department_master.sql` menambahkan
katalog pilihan poli. Jalankan seluruh migration sebelum server versi ini. Buat bucket
Supabase **private** `hospital-images`, allowlist `image/jpeg` dan `image/png`,
batas file 10 MB. Konfigurasi `SUPABASE_HOSPITAL_STORAGE_BUCKET` atau
`storage.hospital_bucket` menunjuk bucket ini. Bucket harus terpisah dari kontrak,
rekam medis, dan foto profil. Bucket tidak dibuat otomatis oleh migration PostgreSQL.

## Endpoint

| Method / path | Akses dan hasil |
| --- | --- |
| `GET /v1/hospitals` | Publik; `data` tetap array, dengan cover image, poli aktif, fasilitas, rating, jam operasional, dan jarak opsional. |
| `GET /v1/departments` | Publik; seluruh master department/poli aktif Indonesia, termasuk yang belum dipakai rumah sakit, beserta kategori dan jumlah rumah sakit/dokter aktif. |
| `GET /v1/hospitals/:hospital_id` | Publik; profil lengkap beserta `gallery` jika ada foto. ID berupa UUID. |
| `GET /v1/hospitals/:hospital_id/images` | Publik; array foto dengan signed URL. Array kosong jika belum ada foto. |
| `POST /v1/hospitals/:hospital_id/images` | ADMIN tenant/SUPER_ADMIN; multipart `image`, `caption` opsional, `sort_order` opsional 0-1000, `is_cover` opsional boolean. HTTP 201. |
| `PATCH /v1/hospitals/:hospital_id/images/:image_id` | ADMIN tenant/SUPER_ADMIN; ubah `caption`, `sort_order`, atau `is_cover: true`. |
| `DELETE /v1/hospitals/:hospital_id/images/:image_id` | ADMIN tenant/SUPER_ADMIN; arsipkan metadata dan hapus object storage. Foto berikutnya menjadi cover jika cover dihapus. |
| `GET /v1/hospitals/:hospital_id/reviews` | Publik; `data: {items,total,limit,offset,rating_average,rating_distribution}`. |
| `GET /v1/hospitals/:hospital_id/reviews/me` | Login; ambil review sendiri, 404 jika belum ada. |
| `PUT /v1/hospitals/:hospital_id/reviews/me` | Login; buat atau perbarui satu review sendiri per rumah sakit. HTTP 200. |
| `DELETE /v1/hospitals/:hospital_id/reviews/me` | Login; arsipkan review sendiri dan keluarkan dari perhitungan rating. |
| `GET /v1/recommendations/hospitals` | Akun pasien; hanya rumah sakit yang mempunyai dokter dan jadwal aktif. Default rating tertinggi. |

POST/PATCH hospital yang sudah ada menerima field profil tambahan. Route admin
menerima UUID/kode hospital sesuai konteks tenant. Route publik dan review sendiri
memakai UUID. Hospital nonaktif/terhapus tidak tersedia di direktori publik.

## Pencarian dan jarak

`GET /v1/hospitals` menerima `search`, `city`, `department` (pencarian lama),
`department_code` (pilihan dari `/v1/departments`), `department_id` (UUID poli), `min_rating` (0-5), `latitude`, `longitude`,
`radius_km` (>0 sampai 5000), `sort` (`name`, `rating`, `distance`),
`limit` (1-100, default 20), dan `offset` (0-100000, default 0).

Latitude dan longitude harus berpasangan. `radius_km` dan `sort=distance`
membutuhkan keduanya. Jarak dihitung sebagai jarak garis lurus di permukaan bumi,
dalam kilometer, bukan jarak rute kendaraan. Koordinat pengguna hanya dipakai pada
request ini. Tanpa koordinat pengguna atau koordinat hospital, `distance_km: null`.
Filter dan pengurutan memakai presisi penuh, respons jarak dibulatkan dua desimal.
Detail hospital juga menerima query latitude/longitude opsional.

Rating tanpa review adalah `rating_average: null`, `rating_count: 0`. Hospital
tanpa rating tidak masuk filter `min_rating`. Sort rating menempatkan rating
tertinggi lalu jumlah review terbesar; hospital tanpa rating berada di akhir.
Filter poli hanya mencakup department rumah sakit yang aktif. Pilihan global
berasal dari `master_departments`; admin mengaktifkan pilihan tersebut pada rumah
sakit melalui `master_department_id`. Parameter katalog: `q`, `category`,
`hospital_id`, `limit`, dan `offset`. Tanpa `hospital_id`, hasil juga memuat
master yang belum dipakai dengan hitungan nol. Kontrak lengkap tersedia di
[`department-master.md`](department-master.md).

## Profil, fasilitas, dan jam operasional

Field tambahan: `email` (maksimum 190), `website` (HTTP/HTTPS, maksimum 2048),
`established_year` (1800 sampai tahun berjalan), `timezone` (IANA, default
`Asia/Jakarta`), `opening_hours`. `description` kini maksimal 10000 karakter.
PATCH string kosong menghapus email/website/deskripsi; `established_year: 0`
menghapus tahun berdiri. Field yang dihilangkan tetap. Nilai JSON null untuk
field pointer diperlakukan seperti field yang dihilangkan.

Fasilitas memakai array terstruktur, maksimum 50 dan kode unik:

```json
[
  {"code":"parking","name":"Parkir Mobil & Motor","icon":"car"},
  {"code":"laboratory","name":"Laboratorium","icon":"flask"}
]
```

Kode dan icon berupa key huruf kecil/angka/underscore/hyphen, maksimum 64 karakter;
nama maksimum 120. `icon` opsional dan merupakan key ikon untuk dipetakan frontend.
Array label dan object boolean/string/angka versi lama masih diterima lalu
dinormalisasi. `facilities: []` atau `null` mengosongkan fasilitas; respons kosong
adalah `[]`.

JSON bebas lama yang tidak dapat dinormalisasi ditampilkan sebagai `[]` dan
dicatat sebagai warning beserta ID hospital. Data asli tetap tersimpan agar admin
dapat memperbaikinya melalui PATCH; satu data lama tidak menggagalkan seluruh list.

`opening_hours: []` berarti belum diketahui, sehingga `is_open: null`. Saat jam
diisi, kirim seluruh 7 hari unik (`day_of_week` 0 Minggu sampai 6 Sabtu). Hari tutup
memakai `is_closed: true, periods: []`; buka 24 jam memakai
`is_24_hours: true, periods: []`. Hari reguler memakai 1-4 interval `open`/`close`
berformat HH:mm. Contoh satu entri:

```json
{"day_of_week":1,"is_closed":false,"is_24_hours":false,"periods":[{"open":"22:00","close":"02:00"}]}
```

Close lebih awal dari open berarti berakhir esok hari. Interval tidak boleh
overlap, termasuk antarhari dan batas Sabtu/Minggu. Open=close ditolak; gunakan
`is_24_hours` untuk sehari penuh. `is_open` dihitung pada timezone hospital dan
berlaku sampai tepat sebelum waktu close. Jadwal ini adalah informasi operasional
rumah sakit; availability dokter tetap memakai jadwal praktik masing-masing.

## Foto dan review

Maksimal 20 foto per rumah sakit. API memeriksa format file aktual dan batas
4096 x 4096 piksel sebelum menyimpan. Foto pertama otomatis menjadi cover;
memilih cover lain hanya mengubah pilihan cover, bukan isi file. Thumbnail list
menggunakan URL foto cover yang sama. Signed URL memiliki `expires_at` dan perlu
diperbarui dengan mengambil data API kembali. Object path dan bucket tidak
ditampilkan pada respons. Penghapusan metadata tetap dilakukan meskipun cleanup
storage gagal; kegagalan cleanup tercatat di log untuk tindak lanjut operasional.

Body PUT review: `{"rating":5,"comment":"Pelayanan baik"}`. Rating integer 1-5,
komentar opsional maksimum 2000 karakter. Backend memakai identitas login, lalu
memverifikasi akun aktif/terverifikasi dan appointment `COMPLETED` pada hospital
tersebut. Kunjungan walk-in yang sudah diklaim ke akun pasien juga memenuhi syarat.
Satu record unik per pasangan pasien/hospital; update bersamaan tidak membuat
duplikat. PUT setelah DELETE mengaktifkan kembali record yang sama.

Daftar review menerima `limit`, `offset`, `sort=latest|highest|lowest`. Ringkasan
rating dihitung dari seluruh review aktif, bukan hanya halaman yang dibaca.
Reviewer publik hanya menampilkan nama depan, atau `Pasien` bila akun dihapus;
tidak memuat ID akun, appointment, diagnosis, atau kontak. Tidak ada endpoint admin
untuk mengubah rating pasien. Review yang dihapus pemilik tidak masuk agregasi.
