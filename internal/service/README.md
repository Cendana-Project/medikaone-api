# Service

- `auth/` menangani registrasi challenge, PIN, password, JWT, refresh rotation, session revocation, dan profile self-service.
- `hospital/` menangani pembuatan hospital serta user/membership/role tenant secara transaksional.

Keputusan role privileged tidak boleh berasal dari input self-service. Redis merupakan dependency keamanan, bukan cache opsional.
