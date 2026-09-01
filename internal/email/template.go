package email

import (
	"fmt"
	"html"
	"time"
)

// RenderVerifyPIN menghasilkan HTML untuk email verifikasi PIN.
func RenderVerifyPIN(firstName, pin string, ttlMinutes int) string {
	if firstName == "" {
		firstName = "Pengguna"
	}
	return fmt.Sprintf(`
<!doctype html><html><body style="font-family:Arial,Helvetica,sans-serif;background:#f6f9fc;padding:24px">
  <div style="max-width:560px;margin:0 auto;background:#fff;border:1px solid #e6ecf1;border-radius:12px;padding:24px">
    <h2 style="margin:0 0 8px 0;color:#111">MedikaOne</h2>
    <p style="color:#555">Halo %s, gunakan PIN berikut untuk verifikasi email (berlaku %d menit):</p>
    <div style="text-align:center;margin:16px 0">
      <span style="display:inline-block;font-size:28px;letter-spacing:6px;font-weight:700;background:#0ea5e9;color:#fff;padding:12px 16px;border-radius:10px">%s</span>
    </div>
    <p style="color:#667">Jika kamu tidak meminta verifikasi ini, abaikan email ini.</p>
  </div>
  <div style="text-align:center;color:#99a; font-size:12px;margin-top:10px">&copy; %d MedikaOne</div>
</body></html>`, html.EscapeString(firstName), ttlMinutes, html.EscapeString(pin), YearNow())
}

// RenderResetPIN menghasilkan HTML untuk email reset password (PIN).
func RenderResetPIN(firstName, pin string, ttlMinutes int) string {
	if firstName == "" {
		firstName = "Pengguna"
	}
	return fmt.Sprintf(`
<!doctype html><html><body style="font-family:Arial,Helvetica,sans-serif;background:#f6f9fc;padding:24px">
  <div style="max-width:560px;margin:0 auto;background:#fff;border:1px solid #e6ecf1;border-radius:12px;padding:24px">
    <h2 style="margin:0 0 8px 0;color:#111">MedikaOne</h2>
    <p style="color:#555">Halo %s, berikut PIN reset password (berlaku %d menit):</p>
    <div style="text-align:center;margin:16px 0">
      <span style="display:inline-block;font-size:28px;letter-spacing:6px;font-weight:700;background:#0ea5e9;color:#fff;padding:12px 16px;border-radius:10px">%s</span>
    </div>
    <p style="color:#667">Jika kamu tidak meminta reset ini, abaikan email ini.</p>
  </div>
  <div style="text-align:center;color:#99a; font-size:12px;margin-top:10px">&copy; %d MedikaOne</div>
</body></html>`, html.EscapeString(firstName), ttlMinutes, html.EscapeString(pin), YearNow())
}

func YearNow() int { return time.Now().Year() }
