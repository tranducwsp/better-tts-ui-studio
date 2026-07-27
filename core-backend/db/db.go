package db

import (
	"log"
	"time"

	"core-backend/config"
	"core-backend/models"
	"core-backend/security"

	"github.com/google/uuid"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

var DB *gorm.DB

func InitDB(cfg *config.Config) *gorm.DB {
	var gormDB *gorm.DB
	var err error

	maxRetries := 10
	for i := 1; i <= maxRetries; i++ {
		log.Printf("Connecting to PostgreSQL database (Attempt %d/%d)...", i, maxRetries)
		gormDB, err = gorm.Open(postgres.Open(cfg.DatabaseURL), &gorm.Config{
			Logger: logger.Default.LogMode(logger.Silent),
		})
		if err == nil {
			sqlDB, pingErr := gormDB.DB()
			if pingErr == nil && sqlDB.Ping() == nil {
				log.Println("Successfully connected to PostgreSQL!")
				break
			}
		}

		if i == maxRetries {
			log.Fatalf("Failed to connect to PostgreSQL database after %d attempts: %v", maxRetries, err)
		}
		time.Sleep(2 * time.Second)
	}

	log.Println("Auto-migrating PostgreSQL database schema...")
	err = gormDB.AutoMigrate(
		&models.User{},
		&models.UserVoice{},
		&models.TTSJob{},
		&models.TTSChunk{},
	)
	if err != nil {
		log.Fatalf("Failed to migrate database schema: %v", err)
	}

	DB = gormDB
	seedDefaultAccounts(cfg)

	log.Println("Database initialization completed successfully.")
	return DB
}

func seedDefaultAccounts(cfg *config.Config) {
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
		var existing models.User
		err := DB.Where("username = ?", acc.username).First(&existing).Error
		if err == gorm.ErrRecordNotFound {
			hashedPw, err := security.HashPassword(acc.password)
			if err != nil {
				log.Printf("Error hashing password for %s: %v", acc.username, err)
				continue
			}

			userID := uuid.NewString()
			newUser := models.User{
				ID:           userID,
				Username:     acc.username,
				PasswordHash: hashedPw,
				Role:         acc.role,
				IsApproved:   true,
			}

			if err := DB.Create(&newUser).Error; err != nil {
				log.Printf("Error creating default account %s: %v", acc.username, err)
			} else {
				log.Printf("Created default %s account: %s", acc.role, acc.username)
			}
		} else {
			log.Printf("Account '%s' already exists.", acc.username)
		}
	}
}

func RegisterJobAndChunk(userID, jobID, engine, voice string, speed float64, totalChunks int, taskID string, chunkIndex int, text string) error {
	var job models.TTSJob
	err := DB.Where("id = ?", jobID).First(&job).Error
	if err == gorm.ErrRecordNotFound {
		newJob := models.TTSJob{
			ID:          jobID,
			UserID:      userID,
			Engine:      engine,
			Voice:       voice,
			Speed:       speed,
			TotalChunks: totalChunks,
		}
		_ = DB.Create(&newJob).Error
	}

	chunk := models.TTSChunk{
		ID:         taskID,
		JobID:      jobID,
		ChunkIndex: chunkIndex,
		Text:       text,
		Status:     "processing",
	}
	return DB.Create(&chunk).Error
}

func UpdateChunkStatus(taskID, status string, audioPath *string, errorMsg *string) error {
	updates := map[string]interface{}{
		"status": status,
	}
	if audioPath != nil {
		updates["audio_path"] = *audioPath
	}
	if errorMsg != nil {
		updates["error_msg"] = *errorMsg
	}
	return DB.Model(&models.TTSChunk{}).Where("id = ?", taskID).Updates(updates).Error
}
