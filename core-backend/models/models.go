package models

import (
	"time"
)

type User struct {
	ID           string      `gorm:"primaryKey;column:id" json:"id"`
	Username     string      `gorm:"column:username;unique;not null" json:"username"`
	PasswordHash string      `gorm:"column:password_hash;not null" json:"-"`
	Role         string      `gorm:"column:role;default:'user'" json:"role"`
	IsApproved   bool        `gorm:"column:is_approved;default:false" json:"is_approved"`
	CreatedAt    time.Time   `gorm:"column:created_at" json:"created_at"`
	Voices       []UserVoice `gorm:"foreignKey:UserID;constraint:OnDelete:CASCADE" json:"voices,omitempty"`
}

func (User) TableName() string {
	return "users"
}

type UserVoice struct {
	ID        string    `gorm:"primaryKey;column:id" json:"id"`
	UserID    string    `gorm:"column:user_id" json:"user_id"`
	Name      string    `gorm:"column:name;not null" json:"name"`
	Gender    *string   `gorm:"column:gender" json:"gender"`
	Region    *string   `gorm:"column:region" json:"region"`
	Style     *string   `gorm:"column:style" json:"style"`
	FilePath  string    `gorm:"column:file_path;not null" json:"-"`
	CreatedAt time.Time `gorm:"column:created_at" json:"created_at"`
}

func (UserVoice) TableName() string {
	return "user_voices"
}

type TTSJob struct {
	ID          string     `gorm:"primaryKey;column:id" json:"job_id"`
	UserID      string     `gorm:"column:user_id" json:"user_id"`
	Engine      string     `gorm:"column:engine" json:"engine"`
	Voice       string     `gorm:"column:voice" json:"voice"`
	Speed       float64    `gorm:"column:speed" json:"speed"`
	TotalChunks int        `gorm:"column:total_chunks" json:"total_chunks"`
	Text        string     `gorm:"column:text" json:"text"`
	CreatedAt   time.Time  `gorm:"column:created_at" json:"created_at"`
	User        *User      `gorm:"foreignKey:UserID" json:"-"`
	Chunks      []TTSChunk `gorm:"foreignKey:JobID;constraint:OnDelete:CASCADE" json:"chunks,omitempty"`
}

func (TTSJob) TableName() string {
	return "tts_jobs"
}

type TTSChunk struct {
	ID         string  `gorm:"primaryKey;column:id" json:"task_id"`
	JobID      string  `gorm:"column:job_id" json:"job_id"`
	ChunkIndex int     `gorm:"column:chunk_index" json:"chunk_index"`
	Text       string  `gorm:"column:text" json:"text"`
	AudioPath  *string `gorm:"column:audio_path" json:"audio_path,omitempty"`
	Status     string  `gorm:"column:status;default:'pending'" json:"status"`
	ErrorMsg   *string `gorm:"column:error_msg" json:"error_msg,omitempty"`
}

func (TTSChunk) TableName() string {
	return "tts_chunks"
}
