# HTTP routes

`http.transport.go` adalah sumber route aktif. Public auth berada di `/v1/auth`; route user-level memakai `AuthRequired`; route hospital memakai `AuthRequired`, `TenantContext`, lalu middleware role/permission yang sesuai.

Jangan menambahkan route admin tanpa middleware eksplisit dan jangan mengambil `hospital_id` otoritatif dari JSON.

Check-in menggunakan lookup dua tahap. Token hasil lookup terikat pada actor dan
tenant, sehingga tidak boleh disimpan lintas sesi petugas. Endpoint identitas
dan walk-in harus tetap memakai body `POST`; jangan memindahkan NIK, email,
telepon, atau tanggal lahir ke query string.
