# Migration

Folder `db/` berisi migration SQL Goose yang dijalankan berurutan berdasarkan timestamp nama file. Jalankan upgrade melalui command aplikasi:

```bash
go run . migrate --action up
```

Migration `20260901010000_harden_user_hospitals.sql` sengaja irreversible. Command aplikasi menolak `down`, `down-to`, dan `reset`; pemulihan staging dilakukan melalui command reset yang dijaga, bukan downgrade schema. Setiap migration `Up` baru wajib non-destruktif terhadap tabel/schema dan harus memperbarui grant runtime minimum bila menambah object yang dipakai proses web.
