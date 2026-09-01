package http

import (
	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"

	"github.com/Cendana-Project/medikaone-api/internal/config"
	"github.com/Cendana-Project/medikaone-api/internal/constant"
	authCtrl "github.com/Cendana-Project/medikaone-api/internal/transport/http/auth"
	hospCtrl "github.com/Cendana-Project/medikaone-api/internal/transport/http/hospital"
	userCtrl "github.com/Cendana-Project/medikaone-api/internal/transport/http/user"
	warmupCtrl "github.com/Cendana-Project/medikaone-api/internal/transport/http/warmup"
	transportmw "github.com/Cendana-Project/medikaone-api/internal/transport/middleware"
	"github.com/Cendana-Project/medikaone-api/internal/util"

	hospRepo "github.com/Cendana-Project/medikaone-api/internal/repository/hospital"
	roleRepo "github.com/Cendana-Project/medikaone-api/internal/repository/role"
	userRepo "github.com/Cendana-Project/medikaone-api/internal/repository/user"
)

type Transport struct {
	router             *gin.Engine
	authController     *authCtrl.Controller
	userController     *userCtrl.Controller
	hospitalController *hospCtrl.Controller
	warmupController   *warmupCtrl.Controller

	roleRepo *roleRepo.Repository
	hospRepo *hospRepo.Repository
	userRepo *userRepo.Repository
	rdb      *redis.Client
}

func NewTransport() *Transport                              { return new(Transport) }
func (t *Transport) WithGinEngine(r *gin.Engine) *Transport { t.router = r; return t }
func (t *Transport) WithAuthController(c *authCtrl.Controller) *Transport {
	t.authController = c
	return t
}
func (t *Transport) WithUserController(c *userCtrl.Controller) *Transport {
	t.userController = c
	return t
}
func (t *Transport) WithHospitalController(c *hospCtrl.Controller) *Transport {
	t.hospitalController = c
	return t
}
func (t *Transport) WithRoleRepository(repo *roleRepo.Repository) *Transport {
	t.roleRepo = repo
	return t
}
func (t *Transport) WithHospitalRepository(repo *hospRepo.Repository) *Transport {
	t.hospRepo = repo
	return t
}
func (t *Transport) WithUserRepository(repo *userRepo.Repository) *Transport {
	t.userRepo = repo
	return t
}
func (t *Transport) WithRedisClient(rdb *redis.Client) *Transport {
	t.rdb = rdb
	return t
}
func (t *Transport) WithWarmupController(c *warmupCtrl.Controller) *Transport {
	t.warmupController = c
	return t
}

func (t *Transport) InitRoute() {
	if t.router == nil {
		panic("gin engine is nil")
	}
	t.router.Use(transportmw.RejectDuringMaintenance(t.rdb))
	// ========== WARMUP — PUBLIC ==========
	t.router.GET("/ping", func(c *gin.Context) { t.warmupController.Ping(c) })

	v1 := t.router.Group("/v1")

	// ========== AUTH — PUBLIC ==========
	auth := v1.Group("/auth")
	auth.Use(transportmw.RateLimitPublicAuthByIP(
		t.rdb,
		config.Env.Auth.PublicIPRateLimit,
		config.Env.Auth.PublicIPRateWindow,
		config.Env.Server.ClientIPHeader,
	))
	{
		auth.POST("/register", t.authController.Register)
		auth.POST("/resend-pin", t.authController.ResendPIN)
		auth.POST("/verify-pin", t.authController.VerifyPIN)

		auth.POST("/login", t.authController.LoginPublic)
		auth.POST("/login/hospital", t.authController.LoginHospital)

		auth.POST("/refresh", t.authController.Refresh)
		auth.POST("/password/forgot", t.authController.PasswordForgot)
		auth.POST("/password/reset", t.authController.PasswordReset)
	}

	// === PROTECTED — USER-LEVEL ===
	protected := v1.Group("/")
	protected.Use(transportmw.AuthRequired(t.rdb, t.userRepo))
	{
		protected.GET("/me", t.userController.Me)

		protected.POST("/auth/choose-role", t.authController.ChooseRole)

		protected.PUT("/profile/patient",
			transportmw.RequirePermissions(t.roleRepo, constant.PermissionPatientEdit),
			t.userController.UpdatePatientProfile,
		)

		protected.PUT("/auth/password", t.authController.PasswordChange)

		protected.PUT("/profile/doctor",
			transportmw.RequireDoctor(t.roleRepo),
			t.userController.UpdateDoctorProfile,
		)

		protected.POST("/hospitals",
			transportmw.RequireSuperAdmin(t.roleRepo),
			t.hospitalController.CreateHospital,
		)

		protected.POST("/auth/set-profile", t.authController.SetProfile) // endpoint gabungan

		// === NEW: Logout endpoints ===
		protected.POST("/auth/logout", t.authController.Logout)
		protected.POST("/auth/logout-all", t.authController.LogoutAll)
	}

	// === PROTECTED — HOSPITAL SCOPED (JWT + Tenant) ===
	tenant := v1.Group("/")
	tenant.Use(transportmw.AuthRequired(t.rdb, t.userRepo), transportmw.TenantContext())
	{
		tenant.POST("/hospitals/:hospital_id/admins",
			transportmw.RequireSuperAdmin(t.roleRepo),
			t.hospitalController.CreateHospitalAdmin,
		)

		tenant.POST("/hospitals/:hospital_id/staff",
			transportmw.RequireHospitalAdminOrSuper(t.hospRepo, t.roleRepo),
			t.hospitalController.CreateHospitalStaff,
		)

		tenant.GET("/tenant/me", t.userController.TenantMe)
	}

	// 404
	t.router.NoRoute(func(c *gin.Context) {
		resp := constant.ErrEndpointNotFound.ToResponse()
		util.HandleResponse(c, &resp, nil)
	})
}
