# Service

- `auth/` menangani registrasi challenge, PIN, password, JWT, refresh rotation, session revocation, dan profile self-service.
- `hospital/` menangani pembuatan hospital serta user/membership/role tenant secara transaksional.
- `doctor_hospital/` menangani undangan dan affiliation dokter dengan rumah sakit.
- `appointment/` menangani jadwal, booking, pencarian dan konfirmasi check-in
  oleh petugas, walk-in tanpa akun, antrean, konsultasi, dan klaim patient record.

Keputusan role privileged tidak boleh berasal dari input self-service. Redis merupakan dependency keamanan, bukan cache opsional.
