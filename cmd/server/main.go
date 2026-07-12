package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"syscall"
	"time"

	"github.com/empowertours/empowertours-app/internal/config"
	"github.com/empowertours/empowertours-app/internal/database"
	"github.com/empowertours/empowertours-app/internal/handlers"
	appMw "github.com/empowertours/empowertours-app/internal/middleware"
	"github.com/empowertours/empowertours-app/internal/music"
	"github.com/empowertours/empowertours-app/internal/realtime"
	"github.com/empowertours/empowertours-app/internal/services"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/stripe/stripe-go/v82"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	db, err := database.Connect(cfg.DatabasePath)
	if err != nil {
		log.Fatalf("Failed to connect to database: %v", err)
	}
	defer db.Close()

	// Resolve migrations directory relative to this source file's location,
	// which works both during development and after `go build`.
	migrationsDir := resolveMigrationsDir()
	if err := database.RunMigrations(db, migrationsDir); err != nil {
		log.Fatalf("Failed to run migrations: %v", err)
	}

	// Demo seed data is opt-in only. Production should show real published
	// listings from users/admin, not generated sample tours.
	if os.Getenv("ENABLE_DEMO_SEED") == "1" {
		if err := services.SeedExperiences(db); err != nil {
			log.Printf("Warning: failed to seed experiences: %v", err)
		}
	}

	// Initialize Stripe
	if cfg.StripeSecretKey != "" {
		stripe.Key = cfg.StripeSecretKey
		log.Println("Stripe payments enabled")
	} else {
		log.Println("Warning: STRIPE_SECRET_KEY not set, payments disabled")
	}

	// Start WebSocket hub for real-time GPS tracking
	hub := realtime.NewHub()
	go hub.Run()
	log.Println("WebSocket hub started for real-time GPS tracking")

	r := chi.NewRouter()

	// Middleware
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)
	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(middleware.Timeout(30 * time.Second))

	r.Use(appMw.CORS())

	// --- Handlers ---
	authHandler := handlers.NewAuthHandler(db, cfg.JWTSecret)
	authHandler.FacebookAppSecret = cfg.FacebookAppSecret
	appHandler := &handlers.ApplicationHandler{DB: db}
	adminHandler := &handlers.AdminHandler{DB: db}
	expHandler := &handlers.ExperienceHandler{DB: db}
	bookingHandler := &handlers.BookingHandler{DB: db}
	rideHandler := &handlers.RideHandler{DB: db}
	sessionHandler := &handlers.SessionHandler{DB: db, Hub: hub}
	trackingHandler := &handlers.TrackingHandler{DB: db, Hub: hub}
	waypointHandler := &handlers.WaypointHandler{DB: db}
	photoHandler := &handlers.PhotoHandler{DB: db, Hub: hub}
	paymentHandler := &handlers.PaymentHandler{DB: db, StripeWebhookSecret: cfg.StripeWebhookSecret}
	presenceHandler := &handlers.PresenceHandler{DB: db}
	appleValidator := services.NewAppleReceiptValidator(cfg)
	googleValidator := services.NewGoogleReceiptValidator(cfg)
	subHandler := &handlers.SubscriptionHandler{DB: db, AppleValidator: appleValidator, GoogleValidator: googleValidator}
	chillHandler := &handlers.ChillHandler{DB: db}

	// Upload handler (photos stored locally in dev, swap to S3 for prod)
	uploadDir := filepath.Join(filepath.Dir(cfg.DatabasePath), "uploads")
	os.MkdirAll(uploadDir, 0755)
	baseURL := fmt.Sprintf("http://localhost:%s", cfg.Port)
	if os.Getenv("BASE_URL") != "" {
		baseURL = os.Getenv("BASE_URL")
	}
	uploadHandler := &handlers.UploadHandler{UploadDir: uploadDir, BaseURL: baseURL}

	// --- Public routes ---
	r.Get("/health", handlers.HealthCheck)

	// Static pages (privacy policy, terms)
	staticDir := resolveStaticDir()
	appDir := filepath.Join(staticDir, "app")

	// Music streaming (public, no auth): friendly web player + catalog API
	// sourced from the Farcaster miniapp's Envio Music NFT indexer.
	musicHandler := &handlers.MusicHandler{
		Svc:       music.NewService(cfg.EnvioEndpoint),
		StaticDir: staticDir,
	}
	log.Printf("Music streaming enabled (indexer: %s)", cfg.EnvioEndpoint)
	r.Get("/", musicHandler.Player)
	r.Get("/listen", musicHandler.Player)
	r.Get("/api/v1/music", musicHandler.ListCatalog)
	r.Get("/api/v1/music/{tokenId}", musicHandler.GetSong)
	r.Get("/stream/{tokenId}", musicHandler.Stream)

	r.Handle("/static/*", http.StripPrefix("/static/", http.FileServer(http.Dir(staticDir))))
	r.Handle("/_expo/*", http.StripPrefix("/_expo/", http.FileServer(http.Dir(filepath.Join(appDir, "_expo")))))
	r.Get("/app", func(w http.ResponseWriter, r *http.Request) {
		http.ServeFile(w, r, filepath.Join(appDir, "index.html"))
	})
	r.Get("/app/*", func(w http.ResponseWriter, r *http.Request) {
		http.ServeFile(w, r, filepath.Join(appDir, "index.html"))
	})
	r.Get("/favicon.ico", func(w http.ResponseWriter, r *http.Request) {
		http.ServeFile(w, r, filepath.Join(appDir, "favicon.ico"))
	})
	r.Get("/privacy", func(w http.ResponseWriter, r *http.Request) {
		http.ServeFile(w, r, filepath.Join(staticDir, "privacy.html"))
	})
	r.Get("/data-deletion", func(w http.ResponseWriter, r *http.Request) {
		http.ServeFile(w, r, filepath.Join(staticDir, "data-deletion.html"))
	})
	r.Get("/terms", func(w http.ResponseWriter, r *http.Request) {
		http.ServeFile(w, r, filepath.Join(staticDir, "terms.html"))
	})
	r.Get("/admin", func(w http.ResponseWriter, r *http.Request) {
		http.ServeFile(w, r, filepath.Join(staticDir, "admin.html"))
	})

	// Facebook data deletion callback (POST signed_request from Meta).
	// Outside the rate-limited /api/v1/auth group because Facebook posts here directly.
	r.Post("/auth/facebook/data-deletion", authHandler.FacebookDataDeletion)

	// Serve uploaded photos
	r.Handle("/uploads/*", http.StripPrefix("/uploads/", http.FileServer(http.Dir(uploadDir))))

	// Stripe webhook (no auth, signature verified internally)
	r.Post("/webhooks/stripe", paymentHandler.HandleWebhook)

	// Auth routes - rate limited: 20 requests per minute per IP
	authLimiter := appMw.NewRateLimiter(20, time.Minute)
	r.Route("/api/v1/auth", func(r chi.Router) {
		r.Use(authLimiter.Middleware)
		r.Post("/nonce", authHandler.Nonce)
		r.Post("/login", authHandler.Login)
		r.Post("/register", authHandler.Register)
		r.Post("/email-login", authHandler.EmailLogin)
		r.Post("/facebook", authHandler.FacebookLogin)
	})

	// --- Protected routes (auth required, no membership needed) ---
	r.Route("/api/v1", func(r chi.Router) {
		r.Use(appMw.JWTAuth(cfg.JWTSecret))

		// Applications — auth only, no subscription needed
		r.Post("/apply", appHandler.SubmitApplication)
		r.Get("/apply/status", appHandler.GetApplicationStatus)
		r.Post("/apply/documents", appHandler.SubmitDocuments)

		// Subscription management — auth only, no subscription needed
		r.Get("/subscription/status", subHandler.GetSubscriptionStatus)
		r.Post("/subscription/trial", subHandler.StartTrial)
		r.Post("/subscription/verify", subHandler.VerifyReceipt)
		r.Post("/subscription/restore", subHandler.RestorePurchase)

		// File uploads — auth only, no subscription needed
		r.Post("/upload/photo", uploadHandler.UploadPhoto)

		// --- Authenticated product routes ---
		// Marketplace and community access are free at launch. Subscription
		// status remains available for future premium features, but it must not
		// prevent a new user from completing a profile or interacting.
		r.Group(func(r chi.Router) {
			// Experiences (browse)
			r.Get("/experiences", expHandler.ListExperiences)
			r.Get("/experiences/{slug}", expHandler.GetExperience)
			r.Get("/experiences/{slug}/waypoints", waypointHandler.ListWaypoints)

			// User-created experiences
			r.Post("/experiences/create", expHandler.CreateUserExperience)
			r.Get("/experiences/mine", expHandler.ListMyExperiences)
			r.Put("/experiences/mine/{id}", expHandler.UpdateUserExperience)
			r.Delete("/experiences/mine/{id}", expHandler.DeleteUserExperience)
			r.Patch("/experiences/mine/{id}/publish", expHandler.PublishExperience)
			r.Patch("/experiences/mine/{id}/unpublish", expHandler.UnpublishExperience)
			r.Get("/experiences/mine/{id}/bookings", expHandler.GetMyExperienceBookings)

			// Bookings
			r.Post("/bookings", bookingHandler.CreateBooking)
			r.Get("/bookings", bookingHandler.ListBookings)
			r.Get("/bookings/{id}", bookingHandler.GetBooking)
			r.Patch("/bookings/{id}/cancel", bookingHandler.CancelBooking)

			// Marketplace payments and host/worker payouts for real-world services.
			// These are separate from app-store subscription billing.
			r.Post("/payments/create", paymentHandler.CreatePaymentIntent)
			r.Get("/payments", paymentHandler.ListMyPayments)
			r.Get("/payments/payout-status", paymentHandler.GetPayoutStatus)
			r.Post("/payments/connect/onboarding", paymentHandler.CreateConnectOnboardingLink)
			r.Post("/payments/bookings/{id}/release", paymentHandler.ReleaseBookingPayout)
			r.Get("/payments/{id}", paymentHandler.GetPayment)

			// Premium transaction and live-tour features remain subscription-gated.
			r.Group(func(r chi.Router) {
				r.Use(appMw.RequireSubscription(db))

				// Lifetime membership ($100 one-time)
				r.Post("/payments/lifetime", paymentHandler.CreateLifetimePaymentIntent)
				r.Get("/membership/status", paymentHandler.GetMembershipStatus)
				r.Post("/explore/consume-free", paymentHandler.ConsumeFreeExplore)
				r.Post("/payments/lifetime/confirm-dev", paymentHandler.ConfirmLifetimeDev) // dev helper - remove in prod

				// Experience Sessions (live GPS tracking)
				r.Post("/sessions/start", sessionHandler.StartSession)
				r.Get("/sessions/my", sessionHandler.ListMySessions)
				r.Get("/sessions/{id}", sessionHandler.GetSession)
				r.Patch("/sessions/{id}/end", sessionHandler.EndSession)
				r.Get("/sessions/{id}/live", sessionHandler.LiveSession) // WebSocket

				// GPS Location tracking
				r.Post("/sessions/{id}/location", trackingHandler.RecordLocation)
				r.Post("/sessions/{id}/location/batch", trackingHandler.RecordLocationBatch)
				r.Get("/sessions/{id}/location", trackingHandler.GetLocationHistory)

				// Checkpoint photos
				r.Post("/sessions/{id}/photos", photoHandler.UploadCheckpointPhoto)
				r.Get("/sessions/{id}/photos", photoHandler.ListSessionPhotos)
			})

			// Chill - social matching & meetups
			r.Get("/chill/profile", chillHandler.GetProfile)
			r.Put("/chill/profile", chillHandler.UpdateProfile)
			r.Get("/chill/discover", chillHandler.Discover)
			r.Post("/chill/request", chillHandler.SendChillRequest)
			r.Get("/chill/requests", chillHandler.GetPendingRequests)
			r.Patch("/chill/requests/{id}", chillHandler.RespondToRequest)
			r.Get("/chill/matches", chillHandler.GetMatches)
			r.Patch("/chill/matches/{id}/schedule", chillHandler.ScheduleMeet)
			r.Post("/chill/travel-plans", chillHandler.CreateTravelPlan)
			r.Get("/chill/travel-plans", chillHandler.GetMyTravelPlans)
			r.Delete("/chill/travel-plans/{id}", chillHandler.DeleteTravelPlan)
			r.Get("/chill/discover/nearby", chillHandler.DiscoverByTravel)

			// Presence powers nearby counts and world-map activity.
			r.Put("/presence", presenceHandler.UpdatePresence)
			r.Get("/presence/nearby", presenceHandler.Nearby)
			r.Get("/presence/hubs", presenceHandler.Hubs)

			// Ride dispatch remains a premium operational feature.
			r.Group(func(r chi.Router) {
				r.Use(appMw.RequireSubscription(db))
				r.Post("/rides/request", rideHandler.RequestRide)
				r.Get("/rides/available", rideHandler.ListAvailableRides)
				r.Post("/rides/{id}/accept", rideHandler.AcceptRide)
				r.Patch("/rides/{id}/status", rideHandler.UpdateRideStatus)
				r.Get("/rides/my", rideHandler.ListMyRides)
			})
		})
	})

	// --- Admin routes (auth + admin required) ---
	r.Route("/api/v1/admin", func(r chi.Router) {
		r.Use(appMw.JWTAuth(cfg.JWTSecret))
		r.Use(appMw.RequireAdmin)

		// Application management
		r.Get("/applications", adminHandler.ListApplications)
		r.Post("/applications/{id}/request-docs", adminHandler.RequestDocs)
		r.Post("/applications/{id}/schedule-interview", adminHandler.ScheduleInterview)
		r.Post("/applications/{id}/complete-interview", adminHandler.CompleteInterview)
		r.Post("/applications/{id}/approve", adminHandler.ApproveApplication)
		r.Post("/applications/{id}/reject", adminHandler.RejectApplication)

		// Experience management (admin create/update/delete)
		r.Post("/experiences", expHandler.CreateExperience)
		r.Put("/experiences/{id}", expHandler.UpdateExperience)
		r.Delete("/experiences/{id}", expHandler.AdminDeleteExperience)

		// Waypoint management
		r.Post("/waypoints", waypointHandler.CreateWaypoint)
		r.Put("/waypoints/{id}", waypointHandler.UpdateWaypoint)
		r.Delete("/waypoints/{id}", waypointHandler.DeleteWaypoint)

		// Booking confirmation
		r.Patch("/bookings/{id}/confirm", bookingHandler.ConfirmBooking)
	})

	srv := &http.Server{
		Addr:         ":" + cfg.Port,
		Handler:      r,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	// Start server in goroutine
	go func() {
		fmt.Printf("EmpowerTours API server starting on :%s\n", cfg.Port)
		fmt.Println("  GPS tracking:  WebSocket at /api/v1/sessions/{id}/live")
		fmt.Println("  Payments:      Stripe webhook at /webhooks/stripe")
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("Server failed: %v", err)
		}
	}()

	// Graceful shutdown
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	fmt.Println("\nShutting down server...")
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := srv.Shutdown(ctx); err != nil {
		log.Fatalf("Server forced to shutdown: %v", err)
	}

	fmt.Println("Server stopped.")
}

// resolveMigrationsDir finds the migrations directory. It first checks for a
// path relative to the source file (works with `go run`), then falls back to
// the working directory.
func resolveMigrationsDir() string {
	// Try relative to source file
	_, filename, _, ok := runtime.Caller(0)
	if ok {
		srcDir := filepath.Dir(filename)
		candidate := filepath.Join(srcDir, "..", "..", "internal", "database", "migrations")
		if info, err := os.Stat(candidate); err == nil && info.IsDir() {
			return candidate
		}
	}

	// Fallback: relative to working directory
	return filepath.Join("internal", "database", "migrations")
}

func resolveStaticDir() string {
	_, filename, _, ok := runtime.Caller(0)
	if ok {
		srcDir := filepath.Dir(filename)
		candidate := filepath.Join(srcDir, "..", "..", "static")
		if info, err := os.Stat(candidate); err == nil && info.IsDir() {
			return candidate
		}
	}
	return "static"
}
