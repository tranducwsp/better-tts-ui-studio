package router

import (
	"net/http"
	"time"

	"backend/client"
	"backend/config"
	"backend/handlers"
	"backend/middleware"

	"github.com/go-chi/chi/v5"
	chiMiddleware "github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"
)

// Dedicated limits for /extract-text — the heaviest backend endpoint.
//
// Each text extraction request holds up to ~32 MiB RAM (see the ceiling in handlers/utils.go),
// so a burst of concurrent requests multiplies that memory usage: this is the final global
// throttle, uncontrollable by input or account. The per-user rate limit is generous (20
// requests/10 min) so thousands of legitimate users all get through — its only job is to stop
// one person from looping calls continuously, whether they mean to or not.
const (
	extractMaxConcurrent     = 8
	extractRateLimitRequests = 20
	extractRateLimitWindow   = 10 * time.Minute
)

// NewRouter creates and configures the full HTTP Router (go-chi) with Middlewares and API Endpoints.
func NewRouter(cfg *config.Config, ttsClient *client.CoreTTSClient) http.Handler {
	r := chi.NewRouter()

	// Base middlewares (Global)
	if cfg.EnableRequestLogging {
		r.Use(chiMiddleware.Logger)
	}
	r.Use(chiMiddleware.Recoverer)

	allowedOrigins := cfg.CORSOrigins
	if len(allowedOrigins) == 0 {
		allowedOrigins = []string{"*"}
	}

	r.Use(cors.Handler(cors.Options{
		AllowedOrigins:   allowedOrigins,
		AllowedMethods:   []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
		AllowedHeaders:   []string{"Accept", "Authorization", "Content-Type", "X-CSRF-Token"},
		ExposedHeaders:   []string{"Link"},
		AllowCredentials: true,
		MaxAge:           300,
	}))

	r.Use(middleware.AuthMiddleware(cfg))

	// Health check & optional debug / swagger endpoints
	r.Get("/health", handlers.HealthCheck)
	r.Get("/ready", handlers.ReadinessCheck)

	if cfg.EnablePprof {
		r.Mount("/debug", chiMiddleware.Profiler())
	}

	if cfg.EnableSwagger {
		r.Get("/swagger", handlers.ServeSwaggerUI)
		r.Get("/swagger/", handlers.ServeSwaggerUI)
		r.Get("/swagger/doc.json", handlers.ServeSwaggerDoc)
	}

	// Handlers
	authHandler := handlers.NewAuthHandler(cfg)
	historyHandler := handlers.NewHistoryHandler()
	cloneHandler := handlers.NewTTSCloneHandler(ttsClient, cfg)
	tasksHandler := handlers.NewTasksHandler()
	utilsHandler := handlers.NewUtilsHandler()
	unifiedHandler := handlers.NewUnifiedHandler(ttsClient)
	engineSyncHandler := handlers.NewEngineSyncHandler(ttsClient, cfg.FEBuilderURL)

	r.Route("/api", func(r chi.Router) {
		// Cap the body of every JSON/text payload (C3). Beneficial side effect: with
		// Content-Length exceeding the limit, chi returns 413 immediately upon writing
		// rather than after the handler finishes processing.
		r.Use(middleware.BodyLimit)

		// Public Auth & Engine Info routes
		r.Get("/info", handlers.GetEngineInfo)

		// Only routes that need password-guessing / spam protection go in this group. Each
		// group has its own Redis key (different scope), so refresh doesn't eat login's
		// budget.
		r.Group(func(r chi.Router) {
			r.Use(middleware.RateLimit("auth", cfg.AuthRateLimitRequests, time.Duration(cfg.AuthRateLimitWindowSeconds)*time.Second))

			r.Post("/register", authHandler.Register)
			r.Post("/login", authHandler.Login)
		})

		// Refresh has its own budget: the frontend calls it on every page load (even when
		// not logged in); sharing a bucket with login would drain the user's own budget.
		r.Group(func(r chi.Router) {
			r.Use(middleware.RateLimit("refresh", cfg.AuthRateLimitRequests, time.Duration(cfg.AuthRateLimitWindowSeconds)*time.Second))

			// Refresh only reads the refresh_token from an HttpOnly cookie. It does not
			// accept a refresh token in the Authorization header or body.
			r.Post("/auth/refresh", authHandler.Refresh)
		})

		// Logout must share the same Path=/api/auth scope as the refresh cookie so the
		// cookie is sent to logout — the server needs it to revoke the session on the DB
		// side.
		r.Post("/auth/logout", authHandler.Logout)

		// Protected routes (Require active/approved user)
		r.Group(func(r chi.Router) {
			r.Use(middleware.RequireActiveUser)

			r.Get("/me", authHandler.Me)

			// Universal Gateway Dynamic Endpoints (require model_id extraction)
			r.Get("/voices/{model_id}", unifiedHandler.GetVoices)
			r.Post("/synthesize/{model_id}", unifiedHandler.Synthesize)

			// History
			r.Get("/history", historyHandler.GetUserHistory)
			r.Get("/history/{job_id}", historyHandler.GetJobDetail)
			r.Post("/jobs/init", historyHandler.InitJob)

			// Voice Management & Custom Voice Cloning
			r.Post("/clone/upload", cloneHandler.UploadVoice)
			r.Post("/clone/upload-temp", cloneHandler.UploadTempVoice)
			r.Get("/clone/voices", cloneHandler.GetUserVoices)
			r.Delete("/clone/voices/{clone_id}", cloneHandler.DeleteUserVoice)

			// Tasks
			r.Get("/tasks/{task_id}", tasksHandler.GetTaskStatus)
			r.Post("/tasks/{task_id}/cancel", tasksHandler.CancelTask)
			r.Get("/tasks/{task_id}/audio", tasksHandler.GetTaskAudio)
			r.Get("/stream/tasks/{task_id}", tasksHandler.StreamTaskProgress)

			// Utils — /extract-text has two dedicated protection layers, not applied to
			// other routes:
			//   * ConcurrencyLimit caps total memory usage (each request ~32 MiB),
			//   * RateLimitUser locks per authenticated user (IP fallback) so one account
			//     looping calls doesn't consume all slots from other users.
			r.Group(func(r chi.Router) {
				r.Use(middleware.ConcurrencyLimit(extractMaxConcurrent))
				r.Use(middleware.RateLimitUser("extract", extractRateLimitRequests, extractRateLimitWindow))
				r.Post("/extract-text", utilsHandler.ExtractText)
			})
		})

		// Admin routes
		r.Group(func(r chi.Router) {
			r.Use(middleware.RequireAdmin)

			// Reload Manifest lives here, not in the public group.
			//
			// The "internal" prefix is just a naming convention, not a protection layer:
			// this route was once open to everyone, and each call both replaces the active
			// Manifest and fires a rebuild webhook to FE Builder — so calling it in a loop
			// would take down both the engine and the builder without logging in.
			r.Post("/internal/engine/reload", engineSyncHandler.ReloadManifest)

			r.Get("/admin/users", authHandler.GetUsers)
			r.Post("/admin/users/{user_id}/approve", authHandler.ApproveUser)
			r.Get("/admin/users/{user_id}/history", historyHandler.GetUserHistoryAdmin)
		})
	})

	return r
}
