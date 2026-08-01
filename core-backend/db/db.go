package db

import (
	"context"
	"embed"
	"log"
	"time"

	"core-backend/config"
	"core-backend/db/sqlc"
	"core-backend/security"

	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/postgres"
	"github.com/golang-migrate/migrate/v4/source/iofs"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

//go:embed migrations/*.sql
var migrationFS embed.FS

var (
	// Pool quản lý Connection Pool PostgreSQL hiệu năng cao của pgx.
	Pool *pgxpool.Pool

	// Queries chứa các phương thức truy vấn CSDL type-safe được sinh ra tự động bởi sqlc.
	Queries *sqlc.Queries
)

// InitDB khởi tạo kết nối CSDL PostgreSQL với cấu hình Connection Pool linh hoạt và tự động thực thi golang-migrate.
func InitDB(cfg *config.Config) {
	var pool *pgxpool.Pool
	var err error

	ctx := context.Background()

	// 1. Cấu hình pgxpool từ URL và các biến môi trường
	poolConfig, parseErr := pgxpool.ParseConfig(cfg.DatabaseURL)
	if parseErr != nil {
		log.Fatalf("Lỗi cấu hình DATABASE_URL: %v", parseErr)
	}

	poolConfig.MaxConns = cfg.DBMaxConns
	poolConfig.MinConns = cfg.DBMinConns
	poolConfig.MaxConnLifetime = time.Duration(cfg.DBMaxConnLifetimeMinutes) * time.Minute
	poolConfig.MaxConnIdleTime = time.Duration(cfg.DBMaxConnIdleMinutes) * time.Minute

	maxRetries := cfg.DBConnectMaxRetries
	retryInterval := time.Duration(cfg.DBConnectRetryIntervalSec) * time.Second

	// 2. Kết nối PostgreSQL với cơ chế Retry
	for i := 1; i <= maxRetries; i++ {
		log.Printf("Connecting to PostgreSQL pool [MaxConns: %d, MinConns: %d] (Attempt %d/%d)...",
			cfg.DBMaxConns, cfg.DBMinConns, i, maxRetries)

		pool, err = pgxpool.NewWithConfig(ctx, poolConfig)
		if err == nil {
			if pingErr := pool.Ping(ctx); pingErr == nil {
				log.Printf("Successfully connected to PostgreSQL via pgxpool (MaxConns: %d, MinConns: %d)!", cfg.DBMaxConns, cfg.DBMinConns)
				break
			}
			pool.Close()
		}

		if i == maxRetries {
			log.Fatalf("Failed to connect to PostgreSQL after %d attempts: %v", maxRetries, err)
		}
		time.Sleep(retryInterval)
	}

	// 3. Tự động thực thi Migration có đánh phiên bản qua golang-migrate
	runDatabaseMigrations(cfg.DatabaseURL)

	Pool = pool
	Queries = sqlc.New(pool)

	// 4. Khởi tạo các tài khoản mặc định (Admin & User)
	seedDefaultAccounts(ctx, cfg)
	log.Println("Database initialization completed successfully (SQL-First sqlc & golang-migrate).")
}

// runDatabaseMigrations thực thi hệ thống quản lý phiên bản database schema (golang-migrate).
func runDatabaseMigrations(dbURL string) {
	log.Println("Executing database versioned migrations (golang-migrate)...")

	driver, err := iofs.New(migrationFS, "migrations")
	if err != nil {
		log.Printf("Warning: Không thể đọc tập tin migrations nhúng trong binary: %v", err)
		return
	}

	m, err := migrate.NewWithSourceInstance("iofs", driver, dbURL)
	if err != nil {
		log.Printf("Warning: Không thể khởi tạo golang-migrate instance: %v", err)
		return
	}

	if err := m.Up(); err != nil && err != migrate.ErrNoChange {
		log.Printf("Warning khi thực thi golang-migrate up: %v", err)
	} else {
		log.Println("Database migrations applied successfully (Schema up-to-date)!")
	}
}

// seedDefaultAccounts tự động khởi tạo tài khoản Admin và User mặc định nếu chưa tồn tại trong PostgreSQL.
func seedDefaultAccounts(ctx context.Context, cfg *config.Config) {
	type account struct {
		username string
		password string
		role     string
	}

	accounts := []account{}
	if cfg.DefaultAdminUsername != "" && cfg.DefaultAdminPassword != "" {
		accounts = append(accounts, account{
			username: cfg.DefaultAdminUsername,
			password: cfg.DefaultAdminPassword,
			role:     "admin",
		})
	}

	if cfg.DefaultUserUsername != "" && cfg.DefaultUserPassword != "" {
		accounts = append(accounts, account{
			username: cfg.DefaultUserUsername,
			password: cfg.DefaultUserPassword,
			role:     "user",
		})
	}

	for _, acc := range accounts {
		_, err := Queries.GetUserByUsername(ctx, acc.username)
		if err != nil {
			hashedPw, err := security.HashPassword(acc.password)
			if err != nil {
				log.Printf("Error hashing password for %s: %v", acc.username, err)
				continue
			}

			userID := uuid.NewString()
			_, err = Queries.CreateUser(ctx, sqlc.CreateUserParams{
				ID:           userID,
				Username:     acc.username,
				PasswordHash: hashedPw,
				Role:         acc.role,
				IsApproved:   true,
			})
			if err != nil {
				log.Printf("Error creating default account %s: %v", acc.username, err)
			} else {
				log.Printf("Created default %s account: %s", acc.role, acc.username)
			}
		} else {
			log.Printf("Account '%s' already exists.", acc.username)
		}
	}
}

// RegisterJobAndChunk đăng ký hoặc cập nhật một Job TTS lớn cùng với đoạn Chunk con vào PostgreSQL bằng sqlc.
// JobAudioParams gom các tham số điều khiển giọng đọc mà Manifest có thể bật/tắt theo
// từng Engine. Pitch/Emotion dùng con trỏ để phân biệt "engine không hỗ trợ" (nil) với
// giá trị người dùng thực sự chọn (ví dụ pitch = 0 là trung tính hợp lệ).
type JobAudioParams struct {
	Speed   float64
	Pitch   *float64
	Emotion *string
}

// PitchColumn chuyển Pitch sang cột nullable của PostgreSQL.
func (p JobAudioParams) PitchColumn() pgtype.Float8 {
	if p.Pitch == nil {
		return pgtype.Float8{}
	}
	return pgtype.Float8{Float64: *p.Pitch, Valid: true}
}

// EmotionColumn chuyển Emotion sang cột nullable của PostgreSQL.
func (p JobAudioParams) EmotionColumn() pgtype.Text {
	if p.Emotion == nil || *p.Emotion == "" {
		return pgtype.Text{}
	}
	return pgtype.Text{String: *p.Emotion, Valid: true}
}

func RegisterJobAndChunk(ctx context.Context, userID, jobID, engine, voice string, audio JobAudioParams, totalChunks int, taskID string, chunkIndex int, text string) error {
	_, err := Queries.GetTTSJobByID(ctx, jobID)
	if err != nil {
		_, _ = Queries.CreateTTSJob(ctx, sqlc.CreateTTSJobParams{
			ID:          jobID,
			UserID:      userID,
			Engine:      engine,
			Voice:       voice,
			Speed:       audio.Speed,
			Pitch:       audio.PitchColumn(),
			Emotion:     audio.EmotionColumn(),
			TotalChunks: int32(totalChunks),
			Text:        text,
		})
	}

	_, err = Queries.CreateTTSChunk(ctx, sqlc.CreateTTSChunkParams{
		ID:         taskID,
		JobID:      jobID,
		ChunkIndex: int32(chunkIndex),
		Text:       text,
		Status:     "processing",
	})
	return err
}

// UpdateChunkStatus cập nhật trạng thái tiến độ (processing, done, error), đường dẫn file audio hoặc thông báo lỗi của Chunk.
func UpdateChunkStatus(ctx context.Context, taskID, status string, audioPath *string, errorMsg *string) error {
	params := sqlc.UpdateTTSChunkStatusParams{
		ID:     taskID,
		Status: status,
	}
	if audioPath != nil {
		params.AudioPath = pgtype.Text{String: *audioPath, Valid: true}
	}
	if errorMsg != nil {
		params.ErrorMsg = pgtype.Text{String: *errorMsg, Valid: true}
	}
	_, err := Queries.UpdateTTSChunkStatus(ctx, params)
	return err
}
