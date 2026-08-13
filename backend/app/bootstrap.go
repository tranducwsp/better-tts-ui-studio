package app

import (
	"errors"
	"fmt"
	"log"
	"time"

	"backend/audio"
	"backend/client"
	"backend/config"
	"backend/db"
	"backend/handlers"
	"backend/middleware"
	"backend/state"
	"backend/storage"
)

// BootstrapWeb initializes everything the HTTP backend needs and returns the Engine client.
//
// Web is the only process that runs migrations, seeds accounts, and configures auth middleware.
// Worker uses BootstrapWorker; cron uses BootstrapCron — no mode flag or hidden branch to guess
// which role a process is playing.
func BootstrapWeb(cfg *config.Config) *client.CoreTTSClient {
	initStorage(cfg)

	handlers.SetMaxUploadMB(cfg.MaxUploadMB)
	audio.ConfigureTranscoding(time.Duration(cfg.TranscodeTimeoutSeconds)*time.Second, cfg.TranscodeMaxConcurrency)
	middleware.ConfigureUserCache(time.Duration(cfg.AuthUserCacheSeconds) * time.Second)
	middleware.SetTrustedProxies(cfg.TrustedProxies)

	return bootstrapEngine(cfg, db.InitOptions{Migrate: true, Seed: true})
}

// BootstrapWorker initializes the dependencies a worker needs to pick up jobs and write results.
//
// Worker does not run migrations/seed, does not configure auth, and does not accept multipart
// requests. DB is still needed for UpdateChunkStatus; Redis is needed for Stream and
// cross-process state.
func BootstrapWorker(cfg *config.Config) *client.CoreTTSClient {
	initStorage(cfg)
	return bootstrapEngine(cfg, db.InitOptions{})
}

// BootstrapCron initializes storage and DB and returns the store for cron.Run to run its own
// loop.
//
// Cron does not need Redis or Engine: it LIST/DELETEs temp objects and Reconciles orphaned
// chunks directly on the DB. DB is a new dependency — needed for moving stuck chunks to error —
// but there is still no Redis, and migrations/seed still do not run here (cron only reads/writes
// status, does not own the schema).
func BootstrapCron(cfg *config.Config) storage.Store {
	initStorage(cfg)
	db.InitDB(cfg, db.InitOptions{})
	return storage.Global
}

func initStorage(cfg *config.Config) {
	if err := storage.Init(cfg.StorageBackend, cfg.StorageDir, storage.S3Config{
		Bucket:         cfg.S3Bucket,
		Region:         cfg.S3Region,
		Endpoint:       cfg.S3Endpoint,
		AccessKey:      cfg.S3AccessKey,
		SecretKey:      cfg.S3SecretKey,
		ForcePathStyle: cfg.S3ForcePathStyle,
		Prefix:         cfg.S3Prefix,
	}); err != nil {
		log.Fatalf("Failed to initialize storage backend: %v", err)
	}
}

func bootstrapEngine(cfg *config.Config, dbOptions db.InitOptions) *client.CoreTTSClient {
	db.InitDB(cfg, dbOptions)
	state.InitRedis(cfg)

	ttsClient := client.NewCoreTTSClient(cfg.CoreTTSURL, cfg.TTSClientTimeout)

	// Wire gRPC client for Synthesize when configured.
	// Manifest/Voices/Clone stay HTTP: they are lightweight and infrequent.
	if cfg.CoreTTSGrpcURL != "" {
		grpcClient, err := client.NewGrpcTTSClient(cfg.CoreTTSGrpcURL)
		if err != nil {
			log.Printf("gRPC connection to %s failed, falling back to HTTP for Synthesize: %v", cfg.CoreTTSGrpcURL, err)
		} else {
			ttsClient.SetGrpcClient(grpcClient)
			log.Printf("Synthesize calls will use gRPC (%s)", cfg.CoreTTSGrpcURL)
		}
	}

	if err := discoverManifest(ttsClient, cfg.CoreTTSURL, 90*time.Second, 3*time.Second); err != nil {
		log.Fatalf("Failed to fetch Manifest from AI Engine: %v.\n"+
			"Backend refuses to start without a Manifest, because all input limits (text length,\n"+
			"speed/pitch range, mode and emotion list) are declared by the Manifest. Check\n"+
			"CORE_ENGINE_URL and verify the Engine is ready.", err)
	}
	return ttsClient
}

// discoverManifest queries the Engine until a valid Manifest is returned, or the timeout expires.
func discoverManifest(ttsClient *client.CoreTTSClient, engineURL string, timeout, retryInterval time.Duration) error {
	deadline := time.Now().Add(timeout)
	var lastErr error

	for attempt := 1; ; attempt++ {
		manifest, err := ttsClient.GetInfo()
		switch {
		case err != nil:
			lastErr = err
		case manifest == nil:
			lastErr = errors.New("engine returned empty manifest")
		default:
			if setErr := state.GlobalManifestState.Set(manifest); setErr != nil {
				lastErr = fmt.Errorf("invalid manifest: %w", setErr)
				break
			}
			log.Printf("🚀 AI Engine Manifest discovered & cached: %s (v%s) [Max Length: %d chars]",
				manifest.EngineName, manifest.Version, manifest.Constraints.MaxTextLength)
			return nil
		}

		if time.Now().After(deadline) {
			return fmt.Errorf("after %s and %d attempts at %s: %w", timeout, attempt, engineURL, lastErr)
		}

		log.Printf("⏳ Waiting for Manifest from AI Engine at %s (%v)... retrying in 3s", engineURL, lastErr)
		time.Sleep(retryInterval)
	}
}
