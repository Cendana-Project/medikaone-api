# Migration

Folder `db/` berisi migration SQL Goose yang dijalankan berurutan berdasarkan timestamp nama file. Jalankan upgrade melalui command aplikasi:

```bash
go run . migrate --action up
```

Migration `20260901010000_harden_user_hospitals.sql`,
`20260904100000_check_in_walk_in.sql`,
`20260904150000_examination.sql`, dan
`20260905090000_prescription.sql` sengaja irreversible. Migration tersebut
memisahkan patient record dari akun autentikasi, mempertahankan revisi rekam
medis final, dan melindungi resep yang sudah diterbitkan. Command aplikasi menolak
`down`, `down-to`, dan `reset`; pemulihan staging dilakukan melalui command
reset yang dijaga, bukan downgrade schema. Setiap migration `Up` baru wajib
non-destruktif terhadap tabel/schema dan harus memperbarui grant runtime minimum
bila menambah object yang dipakai proses web.
Rollback data klinis dilakukan dengan restore backup terverifikasi, bukan
menghapus tabel atau menurunkan schema.
