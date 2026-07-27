package db

import (
	"context"
	"log"
	"os"
	"time"

	"core-backend/config"
	"core-backend/db/sqlc"
	"core-backend/security"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	// Pool quản lý Connection Pool PostgreSQL hiệu năng cao của pgx.
	Pool *pgxpool.Pool

	// Queries chứa các phương thức truy vấn CSDL type-safe được sinh ra tự động bởi sqlc.
	Queries *sqlc.Queries
)

// InitDB khởi tạo kết nối CSDL PostgreSQL với cơ chế Retry ngắt kết nối, tự động nạp Schema DDL và khởi tạo tài khoản mặc định.
func InitDB(cfg *config.Config) {
	var pool *pgxpool.Pool
	var err error

	maxRetries := 10
	ctx := context.Background()

	// 1. Kết nối PostgreSQL với cơ chế retry (thích hợp khi chạy Docker Compose)
	for i := 1; i <= maxRetries; i++ {
		log.Printf("Connecting to PostgreSQL database via pgxpool (Attempt %d/%d)...", i, maxRetries)
		pool, err = pgxpool.New(ctx, cfg.DatabaseURL)
		if err == nil {
			if pingErr := pool.Ping(ctx); pingErr == nil {
				log.Println("Successfully connected to PostgreSQL via pgxpool!")
				break
			}
		}

		if i == maxRetries {
			log.Fatalf("Failed to connect to PostgreSQL after %d attempts: %v", maxRetries, err)
		}
		time.Sleep(2 * time.Second)
	}

	// 2. Tự động thực thi DDL schema nếu chưa tồn tại bảng
	schemaBytes, err := os.ReadFile("db/schema.sql")
	if err == nil {
		log.Println("Executing PostgreSQL database schema...")
		if _, execErr := pool.Exec(ctx, string(schemaBytes)); execErr != nil {
			log.Printf("Warning executing schema.sql: %v", execErr)
		}
	}

	Pool = pool
	Queries = sqlc.New(pool)

	// 3. Đặt các tài khoản mặc định (Admin & User)
	seedDefaultAccounts(ctx, cfg)
	log.Println("Database initialization completed successfully (SQL-First sqlc).")
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
func RegisterJobAndChunk(ctx context.Context, userID, jobID, engine, voice string, speed float64, totalChunks int, taskID string, chunkIndex int, text string) error {
	_, err := Queries.GetTTSJobByID(ctx, jobID)
	if err != nil {
		_, _ = Queries.CreateTTSJob(ctx, sqlc.CreateTTSJobParams{
			ID:          jobID,
			UserID:      userID,
			Engine:      engine,
			Voice:       voice,
			Speed:       speed,
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
