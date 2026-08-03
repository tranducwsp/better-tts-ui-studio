package router

import (
	"net/http"

	"core-backend/client"
	"core-backend/config"
	"core-backend/handlers"
	"core-backend/middleware"

	"github.com/go-chi/chi/v5"
	chiMiddleware "github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"
)

// NewRouter tạo và cấu hình toàn bộ HTTP Router (go-chi) kèm Middlewares và API Endpoints.
func NewRouter(cfg *config.Config, ttsClient *client.CoreTTSClient) http.Handler {
	r := chi.NewRouter()

	// Base middlewares (Global)
	r.Use(chiMiddleware.Logger)
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

	// Health check endpoints
	r.Get("/health", handlers.HealthCheck)
	r.Get("/ready", handlers.ReadinessCheck)

	// Handlers
	authHandler := handlers.NewAuthHandler(cfg)
	historyHandler := handlers.NewHistoryHandler()
	cloneHandler := handlers.NewTTSCloneHandler(ttsClient)
	tasksHandler := handlers.NewTasksHandler()
	utilsHandler := handlers.NewUtilsHandler()
	unifiedHandler := handlers.NewUnifiedHandler(ttsClient)
	engineSyncHandler := handlers.NewEngineSyncHandler(ttsClient, cfg.FEBuilderURL)

	r.Route("/api", func(r chi.Router) {
		// Public Auth & Engine Info routes
		r.Get("/info", handlers.GetEngineInfo)
		r.Post("/internal/engine/reload", engineSyncHandler.ReloadManifest)
		r.Post("/register", authHandler.Register)
		r.Post("/login", authHandler.Login)
		r.Post("/logout", authHandler.Logout)

		// Protected routes (Require active/approved user)
		r.Group(func(r chi.Router) {
			r.Use(middleware.RequireActiveUser)

			r.Get("/me", authHandler.Me)

			// Universal Gateway Dynamic Endpoints (bắt buộc trích xuất theo model_id)
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

			// Utils
			r.Post("/extract-text", utilsHandler.ExtractText)
		})

		// Admin routes
		r.Group(func(r chi.Router) {
			r.Use(middleware.RequireAdmin)

			r.Get("/admin/users", authHandler.GetUsers)
			r.Post("/admin/users/{user_id}/approve", authHandler.ApproveUser)
			r.Get("/admin/users/{user_id}/history", historyHandler.GetUserHistoryAdmin)
		})
	})

	return r
}
