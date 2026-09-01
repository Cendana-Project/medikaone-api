# HTTP routes

`http.transport.go` adalah sumber route aktif. Public auth berada di `/v1/auth`; route user-level memakai `AuthRequired`; route hospital memakai `AuthRequired`, `TenantContext`, lalu middleware role/permission yang sesuai.

Jangan menambahkan route admin tanpa middleware eksplisit dan jangan mengambil `hospital_id` otoritatif dari JSON.
