# Model

- `entity/` memetakan tabel PostgreSQL yang benar-benar digunakan.
- `request/` berisi DTO input dan aturan validasi.
- `response/` berisi payload API publik.

Identitas tenant tidak pernah dipercaya dari JSON; controller mengisinya dari context tenant yang telah diotorisasi.
