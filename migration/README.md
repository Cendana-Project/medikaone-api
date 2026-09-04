# Migration

Folder `db/` berisi migration SQL Goose yang dijalankan berurutan berdasarkan timestamp nama file. Jalankan upgrade melalui command aplikasi:

```bash
go run . migrate --action up
```

Migration `20260901010000_harden_user_hospitals.sql` dan
`20260904100000_check_in_walk_in.sql` sengaja irreversible. Migration kedua
memisahkan patient record dari akun autentikasi agar pasien walk-in tanpa akun
dapat dilayani tanpa membuat credential palsu. Command aplikasi menolak
`down`, `down-to`, dan `reset`; pemulihan staging dilakukan melalui command
reset yang dijaga, bukan downgrade schema. Setiap migration `Up` baru wajib
non-destruktif terhadap tabel/schema dan harus memperbarui grant runtime minimum
bila menambah object yang dipakai proses web.
