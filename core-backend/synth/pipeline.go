package synth

import (
	"context"
	"log"

	"core-backend/client"
	"core-backend/db"
	"core-backend/queue"
	"core-backend/state"
	"core-backend/storage"
)

// Run thực hiện một lượt tổng hợp và ghi lại kết quả.
//
// Tách khỏi handler vì bây giờ có hai người gọi: worker đọc từ hàng đợi, và chính tiến trình
// web khi không có Redis để xếp hàng. Trước đây đoạn này là một goroutine ẩn danh giữa
// Synthesize, nên không có cách nào chạy nó từ nơi khác.
//
// Không trả về lỗi: không có ai để trả. Người dùng theo dõi qua SSE và bảng tts_chunks, nên
// mọi nhánh hỏng đều phải tự ghi lại trạng thái ở đó — trả lỗi lên trên chỉ khiến người gọi
// phải lặp lại đúng việc này.
func Run(ctx context.Context, tts *client.CoreTTSClient, job queue.Job) {
	taskItem := state.GlobalTaskManager.GetOrCreate(job.TaskID)
	taskItem.SetOwner(job.UserID)

	// Lệnh dừng tới từ tiến trình khác: người dùng bấm huỷ trên web, còn lượt này chạy ở
	// worker. WatchCancel nghe kênh Redis của task nên cắt được lượt gọi Engine đang chạy —
	// trước đây cờ cancel không có ai đọc, nên GPU vẫn chạy hết lượt rồi mới báo "done" đè
	// lên trạng thái "cancelled" mà người dùng đã nhìn thấy.
	synthCtx, stopWatch := taskItem.WatchCancel(ctx)
	defer stopWatch()

	audioBytes, err := tts.Synthesize(synthCtx, job.Text, job.Voice, job.Speed, job.Engine, job.Pitch, job.Emotion)
	if err != nil {
		// Bị huỷ không phải hỏng: trạng thái "cancelled" đã do người huỷ ghi, và ghi đè bằng
		// "error" ở đây chỉ biến một hành động cố ý thành một sự cố trong lịch sử.
		if taskItem.IsCancelled() {
			errMsg := "Đã huỷ theo yêu cầu"
			if dbErr := db.UpdateChunkStatus(ctx, job.TaskID, "cancelled", nil, &errMsg); dbErr != nil {
				log.Printf("Chunk %s bị huỷ nhưng không ghi được trạng thái vào DB: %v", job.TaskID, dbErr)
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
			log.Printf("Chunk %s lỗi nhưng không ghi được trạng thái vào DB: %v", job.TaskID, dbErr)
		}
		return
	}

	// Huỷ có thể tới đúng lúc Engine vừa trả kết quả. Không kiểm lại ở đây thì lượt đó vẫn
	// báo "done" đè lên "cancelled" — người dùng thấy task mình đã dừng lại hiện ra như đã
	// chạy xong. Âm thanh đã sinh vẫn cất vào kho: nó đã tốn GPU rồi, và bộ quét dọn thu hồi
	// theo cùng chính sách với mọi tệp tạm khác.
	if taskItem.IsCancelled() {
		errMsg := "Đã huỷ theo yêu cầu"
		if dbErr := db.UpdateChunkStatus(ctx, job.TaskID, "cancelled", nil, &errMsg); dbErr != nil {
			log.Printf("Chunk %s bị huỷ nhưng không ghi được trạng thái vào DB: %v", job.TaskID, dbErr)
		}
		return
	}

	// Định dạng do Mode quyết định: Edge TTS trả MP3, mô hình cục bộ trả WAV. Ghi lại để
	// GetTaskAudio biết mình đang giữ gì thay vì đoán, và chỉ chuyển mã khi client hỏi định
	// dạng khác.
	sourceFormat := state.GlobalManifestState.Get().ResolveAudioSpec(job.Engine).DefaultFormat
	taskItem.SetSourceFormat(sourceFormat)

	// Ghi vào kho thất bại không phải lỗi chí tử — bản trong RAM là phương án dự phòng ngay
	// dưới đây — nhưng nó cần để lại dấu vết: kho đầy biểu hiện thành RSS tăng dần thay vì một
	// lỗi, và không có dòng log này thì nguyên nhân không thể truy ra từ triệu chứng.
	audioKey := storage.TempKey(job.TaskID, sourceFormat)
	wroteToStore := true
	if err := storage.Global.Put(ctx, audioKey, audioBytes); err != nil {
		log.Printf("Không ghi được âm thanh task %s vào kho (%s): %v — giữ trong RAM", job.TaskID, audioKey, err)
		wroteToStore = false
	}

	// Kho là nơi giữ âm thanh; RAM chỉ giữ khi chưa ghi được vào kho.
	//
	// Với worker chạy ở tiến trình riêng, bản trong RAM còn ít giá trị hơn trước: tiến trình
	// web phục vụ lượt tải không nhìn thấy RAM của worker. Đường đọc thật là kho, và Redis là
	// lớp đệm giữa hai tiến trình.
	if wroteToStore {
		taskItem.ReleaseAudio()
	} else {
		taskItem.SetAudio(audioBytes)
	}
	taskItem.CacheAudio(ctx, sourceFormat, audioBytes)

	// Không có client nào để báo ở đây — công việc đã xong và âm thanh đã có. Nhưng lịch sử
	// đọc trạng thái từ DB, nên một lượt ghi thất bại trong im lặng để chunk mãi ở
	// "processing": giao diện hiển thị một job không bao giờ hoàn thành dù tệp đã nằm sẵn
	// trong kho.
	//
	// audio_path chỉ ghi khi kho đã nhận thật. Ghi khoá của một đối tượng không tồn tại nghĩa
	// là lịch sử báo "done" trỏ vào hư không: tiến trình web phục vụ lượt tải không thấy RAM
	// của worker, nên bản dự phòng trong RAM không cứu được gì, và người dùng nhận 404 mãi mãi
	// cho một chunk mà hệ thống khẳng định là đã xong.
	if wroteToStore {
		if err := db.UpdateChunkStatus(ctx, job.TaskID, "done", &audioKey, nil); err != nil {
			log.Printf("Chunk %s đã xong nhưng không ghi được trạng thái vào DB: %v", job.TaskID, err)
		}
	} else {
		errMsg := "Không lưu được âm thanh vào kho"
		if err := db.UpdateChunkStatus(ctx, job.TaskID, "error", nil, &errMsg); err != nil {
			log.Printf("Chunk %s hỏng khi lưu nhưng không ghi được trạng thái vào DB: %v", job.TaskID, err)
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
