# Supabase database CA

`supabase-prod-ca-2021.crt` is the public Supabase Root 2021 CA certificate, not a
private key or application credential. It is included so PostgreSQL clients can
verify Supabase database certificates without changing the machine's trust store.

- Download: <https://supabase-downloads.s3-ap-southeast-1.amazonaws.com/prod/ssl/prod-ca-2021.crt>
- Official dashboard download URL definition: <https://github.com/supabase/supabase/blob/master/apps/studio/hooks/custom-content/custom-content.json>
- Retrieved: 2026-09-20
- SHA-256 of the file: `700723581420dd1ac98fd7e9ac529f0ef210eadcaf87fc868a3ad7d114c2f3b7`
- Certificate expiry: 2031-04-26 10:56:53 UTC

From the repository root, append
`sslmode=verify-full&sslrootcert=certs%2Fsupabase-prod-ca-2021.crt` to the PostgreSQL
URL's query parameters. Keep `verify-full`; the CA authenticates the certificate
chain and the client also checks the configured hostname.

For a deployment with another working directory, use an absolute path to the
deployed certificate. Check Supabase's current certificate before replacing this
file or when the provider rotates its CA. Never add a private key to this folder.
