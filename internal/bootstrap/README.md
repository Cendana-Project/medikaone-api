# Bootstrap

- `server.bootstrap.go` membuat dependency sekali, melakukan wiring route, menjalankan `http.Server` dengan timeout, lalu menutup HTTP, Redis, dan PostgreSQL saat SIGINT/SIGTERM.
- `migrate.bootstrap.go` menjalankan Goose tanpa `AllowMissing` dan selalu mengembalikan error kepada command pemanggil.

Reset destruktif hanya diekspos melalui command staging yang memiliki environment guard dan confirmation token; lihat README root.
