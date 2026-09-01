# Transport

Transport HTTP memakai Gin. Middleware menangani trace ID, batas body, rate limit auth publik, JWT/session validation, dan otorisasi global/tenant. Handler hanya melakukan bind, mengambil identity yang sudah diverifikasi, memanggil service, lalu membentuk respons.
