package synth

import (
	"bytes"
	"context"
	"log"

	"backend/client"
	"backend/db"
	"backend/queue"
	"backend/state"
	"backend/storage"
)

// Run performs a synthesis job and records the result.
//
// Separated from the handler because there are now two callers: the worker reading from the
// queue, and the web process itself when there is no Redis to queue work. Previously this
// was an anonymous goroutine inside Synthesize, so there was no way to run it from elsewhere.
//
// Does not return an error: there is no one to return it to. The user tracks progress via SSE
// and the tts_chunks table, so every failure branch must record its own status there — bubbling
// the error up would only force the caller to repeat the same work.
func Run(ctx context.Context, tts *client.CoreTTSClient, job queue.Job) {
	taskItem := state.GlobalTaskManager.GetOrCreate(job.TaskID)
	taskItem.SetOwner(job.UserID)

	// A stop command arrives from another process: the user clicks cancel on the web UI, while
	// this job is running in a worker. WatchCancel listens on the task's Redis channel so it can
	// cut off an in-flight Engine call — previously the cancel flag had no reader, so the GPU
	// would run the full job and then report "done", overwriting the "cancelled" status the user
	// had already seen.
	synthCtx, stopWatch := taskItem.WatchCancel(ctx)
	defer stopWatch()

	audioBytes, err := tts.Synthesize(synthCtx, job.Text, job.Voice, job.Speed, job.Engine, job.Pitch, job.Emotion)
	if err != nil {
		// Cancelled is not a failure: the "cancelled" status was already written by whoever
		// cancelled, and overwriting it with "error" here would turn a deliberate action into
		// an incident in the history.
		if taskItem.IsCancelled() {
			errMsg := "Cancelled as requested"
			if dbErr := db.UpdateChunkStatus(ctx, job.TaskID, "cancelled", nil, &errMsg); dbErr != nil {
				log.Printf("Chunk %s was cancelled but failed to write status to DB: %v", job.TaskID, dbErr)
			}
			return
		}

		_, _, progress, _ := taskItem.Snapshot()
		taskItem.Notify(state.TaskUpdate{
			Status:   "error",
			Progress: progress,
			Error:    err.Error(),
		})
		errMsg := err.Error()
		if dbErr := db.UpdateChunkStatus(ctx, job.TaskID, "error", nil, &errMsg); dbErr != nil {
			log.Printf("Chunk %s errored but failed to write status to DB: %v", job.TaskID, dbErr)
		}
		return
	}

	// Cancellation can arrive right as the Engine returns a result. If we don't re-check here
	// the job would still report "done" over "cancelled" — the user sees a task they stopped
	// appear as if it completed. The generated audio is still stored: GPU time was already
	// spent, and the cleanup scanner reclaims it under the same policy as all other temp files.
	if taskItem.IsCancelled() {
		errMsg := "Cancelled as requested"
		if dbErr := db.UpdateChunkStatus(ctx, job.TaskID, "cancelled", nil, &errMsg); dbErr != nil {
			log.Printf("Chunk %s was cancelled but failed to write status to DB: %v", job.TaskID, dbErr)
		}
		return
	}

	// Format is determined by Mode: Edge TTS returns MP3, local models return WAV. Record it
	// so GetTaskAudio knows what it holds instead of guessing, and only transcodes when the
	// client requests a different format.
	sourceFormat := state.GlobalManifestState.Get().ResolveAudioSpec(job.Engine).DefaultFormat
	taskItem.SetSourceFormat(sourceFormat)

	// A failed store write is not a fatal error — the in-RAM copy is the fallback right below
	// — but it needs to leave a trace: a full store manifests as gradually increasing RSS rather
	// than an error, and without this log line the cause cannot be traced from the symptom.
	audioKey := storage.AudioKey(job.TaskID, sourceFormat)
	wroteToStore := true
	if err := storage.Global.Put(ctx, audioKey, bytes.NewReader(audioBytes)); err != nil {
		log.Printf("Failed to write audio for task %s to store (%s): %v — keeping in RAM", job.TaskID, audioKey, err)
		wroteToStore = false
	}

	// The store is the canonical audio holder; RAM only holds when the store write failed.
	//
	// With workers running in a separate process, the in-RAM copy is even less valuable than
	// before: the web process serving downloads cannot see the worker's RAM. The real read path
	// is the store, with Redis as the buffer between the two processes.
	if wroteToStore {
		taskItem.ReleaseAudio()
	} else {
		taskItem.SetAudio(audioBytes)
	}
	taskItem.CacheAudio(ctx, sourceFormat, audioBytes)

	// There is no client to notify here — the work is done and the audio is ready. But history
	// reads status from the DB, so a silent write failure leaves the chunk forever at
	// "processing": the UI shows a job that never completes even though the file is already in
	// the store.
	//
	// audio_path is only written when the store has actually received the file. Writing the key
	// of a non-existent object means history reports "done" pointing into the void: the web
	// process serving downloads cannot see the worker's RAM, so the in-RAM fallback cannot save
	// anything, and the user gets 404 forever for a chunk the system claims is done.
	if wroteToStore {
		if err := db.UpdateChunkStatus(ctx, job.TaskID, "done", &audioKey, nil); err != nil {
			log.Printf("Chunk %s completed but failed to write status to DB: %v", job.TaskID, err)
		}
	} else {
		errMsg := "Failed to save audio to store"
		if err := db.UpdateChunkStatus(ctx, job.TaskID, "error", nil, &errMsg); err != nil {
			log.Printf("Chunk %s failed on save but failed to write status to DB: %v", job.TaskID, err)
		}
		taskItem.Notify(state.TaskUpdate{
			Status:   "error",
			Progress: 100,
			Error:    errMsg,
		})
		return
	}

	taskItem.Notify(state.TaskUpdate{
		Status:   "done",
		Progress: 100,
	})
}
