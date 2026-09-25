# Master Department/Poli Indonesia

MedikaOne menyediakan katalog global department/poli agar admin rumah sakit
memilih layanan yang konsisten dan tidak mengetik kode atau nama bebas. Katalog
disimpan pada `master_departments` dan tersedia melalui endpoint publik:

```http
GET /v1/departments?q=Mata&category=MEDICAL_SPECIALIST&limit=100&offset=0
```

Query `hospital_id` opsional membatasi hasil ke master yang sudah diaktifkan pada
rumah sakit tersebut. Tanpa `hospital_id`, seluruh master aktif tetap ditampilkan,
termasuk yang belum dipakai rumah sakit mana pun. `hospital_count` dan
`doctor_count` dapat bernilai `0`.

Contoh item:

```json
{
  "id": "11111111-1111-4111-8111-111111111111",
  "code": "POLI-MATA",
  "name": "Poli Mata",
  "category": "MEDICAL_SPECIALIST",
  "hospital_count": 2,
  "doctor_count": 3
}
```

`id` adalah `master_department_id` yang dipakai admin saat membuat atau mengganti
department rumah sakit. `code` adalah kode stabil lokal MedikaOne yang dipakai
sebagai `department_code` pada filter direktori dokter/rumah sakit. Kode tersebut
bukan kode SATUSEHAT, KKI, atau SNOMED CT.

## Kontrak pengelolaan poli rumah sakit

Admin mengambil master, lalu membuat department rumah sakit:

```http
POST /v1/hospitals/{hospital_id}/departments
Content-Type: application/json

{
  "master_department_id": "11111111-1111-4111-8111-111111111111"
}
```

Backend menyalin `code` dan `name` dari master. Request tidak menerima lagi kode
atau nama bebas. Trigger database juga menormalisasi snapshot tersebut agar write
langsung tidak dapat membuat pasangan master/kode/nama yang berbeda.

Untuk memperbaiki pilihan pada department yang belum mempunyai riwayat:

```http
PATCH /v1/hospitals/{hospital_id}/departments/{department_id}
Content-Type: application/json

{
  "master_department_id": "22222222-2222-4222-8222-222222222222"
}
```

Perubahan master ditolak dengan `RESOURCE_IN_USE` jika department sudah dirujuk
undangan, afiliasi, atau appointment. Hapus/nonaktifkan department lama sesuai
lifecycle lalu buat department baru agar riwayat medis tidak berubah makna.
Master yang tidak ada atau nonaktif menghasilkan `MASTER_DEPARTMENT_NOT_FOUND`.
Master yang sudah aktif pada rumah sakit yang sama menghasilkan
`DEPARTMENT_ALREADY_EXISTS`.

`GET /v1/hospitals/{hospital_id}/departments` tetap mengembalikan department
milik satu rumah sakit. Gunakan `data[].id` dari endpoint tenant tersebut sebagai
`department_id` untuk room, invitation, affiliation, schedule, dan appointment.
Jangan mengirim `master_department_id` pada field yang meminta placement rumah
sakit.

## Kategori

| Nilai | Cakupan |
| --- | --- |
| `GENERAL` | Layanan umum dan kedokteran keluarga/layanan primer. |
| `MEDICAL_SPECIALIST` | Poli spesialis medik non-bedah. |
| `SURGICAL_SPECIALIST` | Poli spesialis bedah. |
| `DENTAL` | Layanan dokter gigi umum dan spesialis gigi. |
| `SUPPORT_SPECIALIST` | Pelayanan spesialis penunjang medik dan diagnostik. |

## Katalog awal

| Code | Nama | Kategori |
| --- | --- | --- |
| `POLI-UMUM` | Poli Umum | `GENERAL` |
| `POLI-KKLP` | Poli Kedokteran Keluarga dan Layanan Primer | `GENERAL` |
| `POLI-GIGI-UMUM` | Poli Gigi Umum | `DENTAL` |
| `POLI-PENYAKIT-DALAM` | Poli Penyakit Dalam | `MEDICAL_SPECIALIST` |
| `POLI-ANAK` | Poli Anak | `MEDICAL_SPECIALIST` |
| `POLI-OBGYN` | Poli Obstetri dan Ginekologi | `MEDICAL_SPECIALIST` |
| `POLI-MATA` | Poli Mata | `MEDICAL_SPECIALIST` |
| `POLI-THT-KL` | Poli THT, Kepala, dan Leher | `MEDICAL_SPECIALIST` |
| `POLI-SARAF` | Poli Saraf | `MEDICAL_SPECIALIST` |
| `POLI-JANTUNG` | Poli Jantung dan Pembuluh Darah | `MEDICAL_SPECIALIST` |
| `POLI-KULIT-KELAMIN` | Poli Dermatologi, Venereologi, dan Estetika | `MEDICAL_SPECIALIST` |
| `POLI-JIWA` | Poli Kedokteran Jiwa | `MEDICAL_SPECIALIST` |
| `POLI-PARU` | Poli Pulmonologi dan Kedokteran Respirasi | `MEDICAL_SPECIALIST` |
| `POLI-GIZI-KLINIK` | Poli Gizi Klinik | `MEDICAL_SPECIALIST` |
| `POLI-ANDROLOGI` | Poli Andrologi | `MEDICAL_SPECIALIST` |
| `POLI-KEDOKTERAN-OLAHRAGA` | Poli Kedokteran Olahraga | `MEDICAL_SPECIALIST` |
| `POLI-KEDOKTERAN-OKUPASI` | Poli Kedokteran Okupasi | `MEDICAL_SPECIALIST` |
| `POLI-FARMAKOLOGI-KLINIK` | Poli Farmakologi Klinik | `MEDICAL_SPECIALIST` |
| `POLI-AKUPUNKTUR-MEDIK` | Poli Akupunktur Medik | `MEDICAL_SPECIALIST` |
| `POLI-KEDARURATAN-MEDIK` | Poli Kedaruratan Medik | `MEDICAL_SPECIALIST` |
| `POLI-BEDAH-UMUM` | Poli Bedah Umum | `SURGICAL_SPECIALIST` |
| `POLI-ORTHOPAEDI-TRAUMATOLOGI` | Poli Orthopaedi dan Traumatologi | `SURGICAL_SPECIALIST` |
| `POLI-UROLOGI` | Poli Urologi | `SURGICAL_SPECIALIST` |
| `POLI-BEDAH-SARAF` | Poli Bedah Saraf | `SURGICAL_SPECIALIST` |
| `POLI-BEDAH-PLASTIK` | Poli Bedah Plastik Rekonstruksi dan Estetika | `SURGICAL_SPECIALIST` |
| `POLI-BEDAH-ANAK` | Poli Bedah Anak | `SURGICAL_SPECIALIST` |
| `POLI-BTKV` | Poli Bedah Toraks, Kardiak, dan Vaskular | `SURGICAL_SPECIALIST` |
| `POLI-BEDAH-MULUT` | Poli Bedah Mulut dan Maksilofasial | `DENTAL` |
| `POLI-KONSERVASI-GIGI` | Poli Konservasi Gigi | `DENTAL` |
| `POLI-ORTODONSIA` | Poli Ortodonsia | `DENTAL` |
| `POLI-PERIODONSIA` | Poli Periodonsia | `DENTAL` |
| `POLI-PROSTODONSIA` | Poli Prostodonsia | `DENTAL` |
| `POLI-GIGI-ANAK` | Poli Kedokteran Gigi Anak | `DENTAL` |
| `POLI-PENYAKIT-MULUT` | Poli Penyakit Mulut | `DENTAL` |
| `POLI-RADIOLOGI-GIGI` | Poli Radiologi Kedokteran Gigi | `DENTAL` |
| `POLI-ANESTESIOLOGI` | Poli Anestesiologi dan Terapi Intensif | `SUPPORT_SPECIALIST` |
| `POLI-REHABILITASI-MEDIK` | Poli Kedokteran Fisik dan Rehabilitasi | `SUPPORT_SPECIALIST` |
| `POLI-RADIOLOGI` | Poli Radiologi | `SUPPORT_SPECIALIST` |
| `POLI-PATOLOGI-KLINIK` | Poli Patologi Klinik | `SUPPORT_SPECIALIST` |
| `POLI-PATOLOGI-ANATOMIK` | Poli Patologi Anatomik | `SUPPORT_SPECIALIST` |
| `POLI-MIKROBIOLOGI-KLINIK` | Poli Mikrobiologi Klinik | `SUPPORT_SPECIALIST` |
| `POLI-PARASITOLOGI-KLINIK` | Poli Parasitologi Klinik | `SUPPORT_SPECIALIST` |
| `POLI-ONKOLOGI-RADIASI` | Poli Onkologi Radiasi | `SUPPORT_SPECIALIST` |
| `POLI-KEDOKTERAN-NUKLIR` | Poli Kedokteran Nuklir dan Teranostik Molekuler | `SUPPORT_SPECIALIST` |
| `POLI-FORENSIK-MEDIKOLEGAL` | Poli Kedokteran Forensik dan Medikolegal | `SUPPORT_SPECIALIST` |

Katalog awal mengikuti nama pelayanan pada [Permenkes Nomor 33 Tahun
2023](https://jdih.kemkes.go.id/common/dokumen/2023permenkes033.pdf), kelompok
spesialis gigi pada [instrumen rumah sakit pendidikan Kementerian
Kesehatan](https://jdih.kemkes.go.id/common/dokumen/KMK%20No.%20HK.01.07-MENKES-16-2023%20ttg%20Instrumen%20Penilaian%20RS%20Pendidikan%20dan%20Rasio%20Jumlah%20Dosen%20Dengan%20Mahasiswa%20di%20RS%20Pendidikan-signed.pdf),
serta nomenklatur layanan primer/kedaruratan yang digunakan di
[RS Online Kementerian Kesehatan](https://sirs.kemkes.go.id/fo/). SATUSEHAT
memakai terminologi pada `HealthcareService.specialty`; integrasi terminologi
tersebut harus menambahkan mapping eksplisit dan tidak boleh menganggap code
lokal di atas sebagai code SATUSEHAT.

## Data lama dan deployment

Migration `20260926120000_department_master.sql` memasukkan master dan mengisi
`master_department_id` untuk department lama yang mempunyai code sama. Row lama
dengan code khusus rumah sakit tetap disimpan dengan `master_department_id: null`
agar migration tidak merusak data produksi. Row legacy tersebut masih tampil
pada detail rumah sakit, tetapi tidak dapat dibuat lagi melalui API dan tidak
masuk katalog master global.

Jalankan migration sebelum deploy backend. Seeder menyinkronkan 45 definisi
master secara idempoten dan menghubungkan fixture department ke master yang sama.
