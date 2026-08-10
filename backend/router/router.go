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

// Hạn mức riêng của /extract-text — endpoint nặng nhất của backend.
//
// Mỗi request bóc chữ giữ tới ~32 MiB RAM (xem phần trần trong handlers/utils.go), nên một
// loạt request cùng lúc nhân lượng bộ nhớ đó lên: đây là chốt chặn global cuối cùng, không thể
// điều khiển bằng input hay tài khoản. Rate-limit theo user thì thoáng (20 lượt/10 phút) để
// hàng nghìn người dùng hợp lệ đều đi qua được — nó chỉ có nhiệm vụ chặn một người giữ vòng
// lặp gọi liên tục, cúp cầu hay không.
const (
	extractMaxConcurrent     = 8
	extractRateLimitRequests = 20
	extractRateLimitWindow   = 10 * time.Minute
)

// NewRouter tạo và cấu hình toàn bộ HTTP Router (go-chi) kèm Middlewares và API Endpoints.
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
		// Cap body của mọi payload JSON/text (C3). Tác dụng phụ có lợi: với Content-Length
		// vượt trần, chi trả lỗi 413 ngay khi ghi chứ không phải sau khi handler xử lý xong.
		r.Use(middleware.BodyLimit)

		// Public Auth & Engine Info routes
		r.Get("/info", handlers.GetEngineInfo)

		// Chỉ các route cần bảo vệ dò mật khẩu / tạo rác mới nằm trong nhóm này. Mỗi nhóm
		// có khoá Redis riêng (scope khác nhau), nên refresh không ăn budget của login.
		r.Group(func(r chi.Router) {
			r.Use(middleware.RateLimit("auth", cfg.AuthRateLimitRequests, time.Duration(cfg.AuthRateLimitWindowSeconds)*time.Second))

			r.Post("/register", authHandler.Register)
			r.Post("/login", authHandler.Login)
		})

		// Refresh có budget riêng: frontend gọi nó trên mỗi lần tải trang (kể cả khi chưa
		// đăng nhập), gộp chung với login sẽ làm cạn budget của chính người dùng.
		r.Group(func(r chi.Router) {
			r.Use(middleware.RateLimit("refresh", cfg.AuthRateLimitRequests, time.Duration(cfg.AuthRateLimitWindowSeconds)*time.Second))

			// Refresh chỉ đọc refresh_token từ HttpOnly cookie. Không nhận refresh token trong
			// Authorization header hay body.
			r.Post("/auth/refresh", authHandler.Refresh)
		})

		// Logout phải ở cùng scope Path=/api/auth với refresh cookie để cookie được gửi tới
		// logout — server cần nó để thu hồi phiên phía DB.
		r.Post("/auth/logout", authHandler.Logout)

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

			// Utils — /extract-text có hai lớp bảo vệ riêng, không áp dụng cho route khác:
			//   * ConcurrencyLimit dừng lượng bộ nhớ tổng (mỗi request ~32 MiB),
			//   * RateLimitUser khoá theo user đã xác thực (fallback IP) để một tài khoản
			//     giữ chu kỳ gọi không ăn hết chỗ của những người dùng khác.
			r.Group(func(r chi.Router) {
				r.Use(middleware.ConcurrencyLimit(extractMaxConcurrent))
				r.Use(middleware.RateLimitUser("extract", extractRateLimitRequests, extractRateLimitWindow))
				r.Post("/extract-text", utilsHandler.ExtractText)
			})
		})

		// Admin routes
		r.Group(func(r chi.Router) {
			r.Use(middleware.RequireAdmin)

			// Reload Manifest nằm ở đây, không ở nhóm public.
			//
			// Tiền tố "internal" chỉ là quy ước đặt tên, không phải một lớp bảo vệ: route này
			// từng mở cho mọi người, và mỗi lần gọi vừa thay Manifest đang dùng vừa bắn một
			// webhook rebuild sang FE Builder — nên gọi nó trong vòng lặp là hạ cả engine lẫn
			// builder mà không cần đăng nhập.
			r.Post("/internal/engine/reload", engineSyncHandler.ReloadManifest)

			r.Get("/admin/users", authHandler.GetUsers)
			r.Post("/admin/users/{user_id}/approve", authHandler.ApproveUser)
			r.Get("/admin/users/{user_id}/history", historyHandler.GetUserHistoryAdmin)
		})
	})

	return r
}
