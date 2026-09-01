# Infrastructure

- `gorm.go` membuat satu pool PostgreSQL. `database/sql` menangani reconnect tanpa mengganti pointer yang sudah dipakai repository. Query staging/production diparameterisasi di log.
- `cache.go` membuat satu Redis client yang diinjeksi ke service dan middleware.
- `gin.go` memasang trace ID, recovery, batas body 1 MiB, structured access log dengan client-IP header tepercaya, exact CORS allowlist, serta liveness/readiness endpoint.
- `common.go` menyimpan registry health check yang concurrency-safe dan helper redaksi error koneksi.

Lifecycle dan penutupan HTTP server, PostgreSQL, serta Redis dikelola oleh `internal/bootstrap/server.bootstrap.go`.
