package bootstrap

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"github.com/Cendana-Project/medikaone-api/internal/config"
	"github.com/Cendana-Project/medikaone-api/internal/email"
	"github.com/Cendana-Project/medikaone-api/internal/infrastructure"
	appointmentRepo "github.com/Cendana-Project/medikaone-api/internal/repository/appointment"
	doctorHospitalRepo "github.com/Cendana-Project/medikaone-api/internal/repository/doctor_hospital"
	examinationRepo "github.com/Cendana-Project/medikaone-api/internal/repository/examination"
	hospRepo "github.com/Cendana-Project/medikaone-api/internal/repository/hospital"
	roleRepo "github.com/Cendana-Project/medikaone-api/internal/repository/role"
	userRepo "github.com/Cendana-Project/medikaone-api/internal/repository/user"
	appointmentSvc "github.com/Cendana-Project/medikaone-api/internal/service/appointment"
	authSvc "github.com/Cendana-Project/medikaone-api/internal/service/auth"
	doctorHospitalSvc "github.com/Cendana-Project/medikaone-api/internal/service/doctor_hospital"
	examinationSvc "github.com/Cendana-Project/medikaone-api/internal/service/examination"
	hospSvc "github.com/Cendana-Project/medikaone-api/internal/service/hospital"
	storageclient "github.com/Cendana-Project/medikaone-api/internal/storage"
	httpTransport "github.com/Cendana-Project/medikaone-api/internal/transport/http"
	appointmentHTTP "github.com/Cendana-Project/medikaone-api/internal/transport/http/appointment"
	authHTTP "github.com/Cendana-Project/medikaone-api/internal/transport/http/auth"
	doctorHospitalHTTP "github.com/Cendana-Project/medikaone-api/internal/transport/http/doctor_hospital"
	examinationHTTP "github.com/Cendana-Project/medikaone-api/internal/transport/http/examination"
	hospHTTP "github.com/Cendana-Project/medikaone-api/internal/transport/http/hospital"
	userHTTP "github.com/Cendana-Project/medikaone-api/internal/transport/http/user"
	warmupHTTP "github.com/Cendana-Project/medikaone-api/internal/transport/http/warmup"
	"github.com/sirupsen/logrus"
)

func StartServer() {
	if err := runServer(context.Background()); err != nil {
		logrus.WithError(err).Fatal("server stopped unexpectedly")
	}
}

func runServer(parent context.Context) (returnErr error) {
	gormDB, err := infrastructure.OpenDBConn()
	if err != nil {
		return fmt.Errorf("connect database: %w", err)
	}
	rdb, err := infrastructure.OpenRedisClient()
	if err != nil {
		_ = infrastructure.CloseDB()
		return fmt.Errorf("connect Redis: %w", err)
	}
	defer func() {
		returnErr = errors.Join(returnErr, rdb.Close(), infrastructure.CloseDB())
	}()
	r := infrastructure.NewGinEngine()

	uRepo := userRepo.NewRepository(gormDB)
	rRepo := roleRepo.NewRepository(gormDB)
	hRepo := hospRepo.NewRepository(gormDB)
	dhRepo := doctorHospitalRepo.NewRepository(gormDB)
	aRepo := appointmentRepo.NewRepository(gormDB)
	eRepo := examinationRepo.NewRepository(gormDB)
	privateStorage, err := storageclient.NewSupabaseClient(config.Env.Storage)
	if err != nil {
		return fmt.Errorf("configure private storage: %w", err)
	}
	medicalStorage, err := storageclient.NewSupabaseClientForBucket(config.Env.Storage, config.Env.Storage.MedicalBucket)
	if err != nil {
		return fmt.Errorf("configure medical storage: %w", err)
	}
	sender := email.NewSMTPSender(&email.Config{
		Enabled:     config.Env.SMTP.Enabled,
		Host:        config.Env.SMTP.Host,
		Port:        config.Env.SMTP.Port,
		Username:    config.Env.SMTP.Username,
		Password:    config.Env.SMTP.Password,
		FromEmail:   config.Env.SMTP.From,
		FromName:    config.Env.SMTP.FromName,
		UseSTARTTLS: config.Env.SMTP.UseSTARTTLS,
		Timeout:     config.Env.SMTP.Timeout,
	})

	authService := authSvc.NewService(uRepo, rRepo, rdb, sender, hRepo)
	hospitalService := hospSvc.NewService(uRepo, rRepo, hRepo)
	doctorHospitalService := doctorHospitalSvc.NewService(
		dhRepo,
		privateStorage,
		sender,
		storageclient.MaxFileSize(config.Env.Storage),
		storageclient.SignedURLTTL(config.Env.Storage),
	)
	appointmentService := appointmentSvc.NewService(aRepo, sender, config.Env.JWT.Secret)
	examinationService := examinationSvc.NewService(
		eRepo,
		medicalStorage,
		storageclient.MaxFileSize(config.Env.Storage),
		storageclient.SignedURLTTL(config.Env.Storage),
	)
	authController := authHTTP.NewController(authService, uRepo)
	userController := userHTTP.NewController(authService, uRepo)
	hospitalController := hospHTTP.NewController(hospitalService)
	doctorHospitalController := doctorHospitalHTTP.NewController(doctorHospitalService)
	appointmentController := appointmentHTTP.NewController(appointmentService)
	examinationController := examinationHTTP.NewController(examinationService)
	warmupController := warmupHTTP.NewController()

	httpTransport.NewTransport().
		WithGinEngine(r).
		WithAuthController(authController).
		WithUserController(userController).
		WithHospitalController(hospitalController).
		WithDoctorHospitalController(doctorHospitalController).
		WithAppointmentController(appointmentController).
		WithExaminationController(examinationController).
		WithWarmupController(warmupController).
		WithRoleRepository(rRepo).
		WithHospitalRepository(hRepo).
		WithUserRepository(uRepo).
		WithRedisClient(rdb).
		InitRoute()

	serverCtx, stopSignals := signal.NotifyContext(parent, os.Interrupt, syscall.SIGTERM)
	defer stopSignals()
	go appointmentService.RunBackgroundWorker(serverCtx)
	requestBaseCtx, cancelRequests := context.WithCancel(context.WithoutCancel(parent))
	defer cancelRequests()
	server := newHTTPServer(requestBaseCtx, r)
	serveErr := make(chan error, 1)
	go func() {
		logrus.WithField("port", config.Env.Server.Port).Info("HTTP server started")
		serveErr <- server.ListenAndServe()
	}()

	var runErr error
	select {
	case err := <-serveErr:
		if !errors.Is(err, http.ErrServerClosed) {
			runErr = fmt.Errorf("serve HTTP: %w", err)
		}
	case <-serverCtx.Done():
		logrus.Info("shutdown signal received")
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), config.Env.GracefulShutdownTimeout)
	defer cancel()
	shutdownErr := server.Shutdown(shutdownCtx)
	if shutdownErr != nil {
		_ = server.Close()
	}
	// In-flight handlers keep a live BaseContext while Shutdown drains them.
	// Cancel it only after the drain completes (or the timeout forces Close).
	cancelRequests()
	emailCloseErr := authService.CloseEmailDispatcher(shutdownCtx)
	return errors.Join(runErr, shutdownErr, emailCloseErr)
}

func newHTTPServer(baseContext context.Context, handler http.Handler) *http.Server {
	return &http.Server{
		Addr:              net.JoinHostPort("", config.Env.Server.Port),
		Handler:           handler,
		ReadHeaderTimeout: config.Env.Server.ReadHeaderTimeout,
		ReadTimeout:       config.Env.Server.ReadTimeout,
		WriteTimeout:      config.Env.Server.WriteTimeout,
		IdleTimeout:       config.Env.Server.IdleTimeout,
		MaxHeaderBytes:    1 << 20,
		BaseContext: func(net.Listener) context.Context {
			return baseContext
		},
	}
}
