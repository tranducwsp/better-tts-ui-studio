package handlers

import (
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"slices"
	"strings"

	"backend/state"
)

// Default matches MAX_UPLOAD_SIZE_MB=50 in config/settings.go so handlers built independently
// in tests don't have a hidden 32MB ceiling. Production bootstrap always overrides with the
// validated Config value.
var maxUploadBytes int64 = 50 << 20

// SetMaxUploadMB sets the upload size cap for every handler that accepts multipart.
func SetMaxUploadMB(mb int) {
	if mb > 0 {
		maxUploadBytes = int64(mb) << 20
	}
}

// audioSignatures are format identification signatures, to catch files with renamed extensions.
//
// Only lists formats with stable signatures at the beginning of the file. Formats not in the
// table are still accepted if the Engine declares them — the platform just cannot verify the
// content, and guessing wrong is worse than letting the Engine reject it.
var audioSignatures = map[string]func([]byte) bool{
	"wav":  func(b []byte) bool { return len(b) >= 12 && string(b[0:4]) == "RIFF" && string(b[8:12]) == "WAVE" },
	"flac": func(b []byte) bool { return len(b) >= 4 && string(b[0:4]) == "fLaC" },
	"ogg":  func(b []byte) bool { return len(b) >= 4 && string(b[0:4]) == "OggS" },
	// MP3 may start with an ID3 tag or directly with a frame sync (0xFF 0xEx/0xFx).
	"mp3": func(b []byte) bool {
		if len(b) >= 3 && string(b[0:3]) == "ID3" {
			return true
		}
		return len(b) >= 2 && b[0] == 0xFF && (b[1]&0xE0) == 0xE0
	},
}

// checkReferenceAudio validates the reference file format against what the Engine declares it
// can read.
//
// Only the file head is needed, not the whole file: format signatures are in the first few
// bytes, and the size cap compares against the part size declared by multipart, not the
// length of an in-RAM copy. The browser's accept attribute is only a hint for the file picker;
// any client can send something else. The signature check is also needed because renaming
// .mp3 to .wav is the easiest thing to do.
func checkReferenceAudio(filename string, head []byte, size int64, modeID string) error {
	// Without a Manifest, the format allowlist would be the platform's default, not what the
	// Engine actually reads — a limit that looks like it applies but describes a different engine.
	m := state.GlobalManifestState.Get()
	if m == nil {
		return errors.New("no manifest from AI Engine yet, temporarily not accepting reference files")
	}
	spec := m.ResolveAudioSpec(modeID)

	ext := ""
	if i := strings.LastIndex(filename, "."); i != -1 {
		ext = strings.ToLower(filename[i+1:])
	}

	accepted := spec.ReferenceAudioFormats
	if !slices.Contains(accepted, ext) {
		return fmt.Errorf("only accepts %s for reference audio", strings.ToUpper(strings.Join(accepted, ", ")))
	}

	if check, known := audioSignatures[ext]; known && !check(head) {
		return fmt.Errorf("file is not valid %s despite having .%s extension", strings.ToUpper(ext), ext)
	}

	if max := spec.MaxReferenceBytes; max > 0 && size > max {
		return fmt.Errorf("reference audio exceeds the %d MB limit the engine accepts", max>>20)
	}
	return nil
}

// multipartMemoryLimit is the threshold for keeping multipart file data in RAM during form parsing.
//
// ParseMultipartForm keeps file parts in memory up to this limit; larger parts are written to
// temporary files on disk. Passing the limit (upload cap) as this parameter is wrong: it tells
// Go to hold a file up to 256MB in RAM just to parse the form, then the caller reads it again.
// 32MB is enough for a typical reference file (a few seconds of recording) while still
// allowing large files to go to disk.
const multipartMemoryLimit = 32 << 20 // 32 MiB

// parseUpload reads the multipart form within the configured limit.
//
// MaxBytesReader blocks at the connection layer, so an oversized file is cut off during
// transfer rather than being fully loaded into RAM and then rejected. Returns a message
// stating the limit, because "request too large" without saying how large leaves the user
// not knowing how much to trim.
func parseUpload(w http.ResponseWriter, r *http.Request) error {
	limit := uploadLimit()
	r.Body = http.MaxBytesReader(w, r.Body, limit)
	if err := r.ParseMultipartForm(multipartMemoryLimit); err != nil {
		return fmt.Errorf("file exceeds the %d MB limit", limit>>20)
	}
	return nil
}

// uploadLimit is the cap for the RAW uploaded file, before trimming.
//
// Intentionally higher than max_reference_bytes: the user may drag in a long recording and
// only take a few seconds, so blocking at the file selection step with the Engine's cap would
// reject a perfectly valid file for trimming. The Engine declares this value; MAX_UPLOAD_SIZE_MB
// remains the infrastructure's hard ceiling and wins if tighter, because it speaks about the
// deployment's RAM and bandwidth, not about the model.
func uploadLimit() int64 {
	limit := maxUploadBytes
	if m := state.GlobalManifestState.Get(); m != nil {
		if declared := m.ResolveAudioSpec("").MaxUploadBytes; declared > 0 && declared < limit {
			limit = declared
		}
	}
	return limit
}

// referenceUpload is a reference file that has passed all checks, ready to use.
//
// Does not hold Data []byte as before: a minute-long reference file is tens of MB, and reading
// it all into RAM just for the caller to write it elsewhere is two copies. The caller streams
// directly from File.

type referenceUpload struct {
	ModelID  string
	Filename string
	File     multipart.File
}

// receiveReferenceAudio resolves the mode → checks the file head → returns a stream handle at position 0.
//
// Only reads enough of the first few bytes to check the signature, then Seeks back to 0
// instead of io.ReadAll on the entire file. parseUpload is intentionally outside this function:
// r.FormValue is only readable after the form is parsed, and most of that file is on disk
// (multipartMemoryLimit) — swallowing a 100 MB file into RAM only to discover a missing voice
// name is exactly what MaxBytesReader in parseUpload is placed there to prevent.
//
// The caller is responsible for closing File and calling r.MultipartForm.RemoveAll() after use
// — parts spilled to disk must be cleaned up, while files in RAM only need Close.
func receiveReferenceAudio(w http.ResponseWriter, r *http.Request) (referenceUpload, bool) {
	// Don't guess the mode name: ask the Manifest which Mode actually supports cloning. The
	// previously hardcoded "clone" string caused voices from an Engine whose mode was named
	// zero_shot_clone to be saved under a non-existent model_id, so no mode could list them later.
	modelID := r.FormValue("model_id")
	if modelID == "" {
		modelID = firstCloningMode()
	}

	// model_id goes into the storage key below (storage.VoiceKey), so it must be a Mode the
	// Engine actually declares — not an arbitrary string, because it is used verbatim to
	// construct the path / key. Previously model_id=../../.. would escape os.WriteFile outside
	// the storage directory, with content controlled by the sender. ResolveAudioSpec cannot
	// guard against this because it silently falls back to defaults for unknown modes, so the
	// guard must be here.
	if !state.GlobalManifestState.HasMode(modelID) {
		writeError(w, http.StatusBadRequest, "model_id is not a mode supported by the engine")
		return referenceUpload{}, false
	}

	file, header, err := r.FormFile("file")
	if err != nil {
		writeError(w, http.StatusBadRequest, "Missing audio file")
		return referenceUpload{}, false
	}

	// Read just enough of the head for signature checking (known format signatures are within
	// the first few bytes), then rewind to 0 so the caller can stream from the start. No need
	// to read the entire file.
	head, _ := io.ReadAll(io.LimitReader(file, signaturePeekLen))
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		file.Close()
		writeError(w, http.StatusInternalServerError, "Failed to re-read audio file")
		return referenceUpload{}, false
	}

	if err := checkReferenceAudio(header.Filename, head, header.Size, modelID); err != nil {
		file.Close()
		writeError(w, http.StatusBadRequest, err.Error())
		return referenceUpload{}, false
	}

	return referenceUpload{ModelID: modelID, Filename: header.Filename, File: file}, true
}

// signaturePeekLen is the number of bytes to read from the beginning of a file to recognise
// known format signatures.
//
// The longest signature currently checked is wav (RIFF…WAVE) at 12 bytes; 512 bytes is more
// than generous and still negligible compared to hundreds of MB of audio data.
const signaturePeekLen = 512

// cleanupMultipartForm cleans up file parts left behind by ParseMultipartForm.
//
// With multipartMemoryLimit, every part larger than the threshold is stored on disk as a
// temporary file; without calling RemoveAll they accumulate until the process exits. Called
// at the end of every handler that uses parseUpload, after every stream has finished reading
// the part.
func cleanupMultipartForm(r *http.Request) {
	if r.MultipartForm != nil {
		_ = r.MultipartForm.RemoveAll()
	}
}
