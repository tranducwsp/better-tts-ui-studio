-- Postgres không tự tạo index cho cột tham chiếu khoá ngoại, nên hai cột mà mọi trang
-- lịch sử đều lọc theo vẫn phải quét tuần tự.
--
-- ListUserHistorySummaries join tts_jobs với tts_chunks rồi GROUP BY: không có index trên
-- tts_chunks.job_id, mỗi lần mở lịch sử là một lần quét TOÀN BỘ bảng chunks — của mọi
-- người dùng, không riêng người đang xem. Bảng đó tăng một dòng mỗi chunk và không bao giờ
-- co lại, nên chi phí đi theo dữ liệu toàn hệ thống chứ không theo dữ liệu của một người.
--
-- Index thứ hai gộp cả cột lọc và cột sắp xếp, vì truy vấn luôn kết thúc bằng
-- ORDER BY created_at DESC. ON DELETE CASCADE trên cả hai khoá ngoại cũng dùng chính
-- những index này khi xoá người dùng.
CREATE INDEX IF NOT EXISTS idx_tts_chunks_job_id ON tts_chunks (job_id);
CREATE INDEX IF NOT EXISTS idx_tts_jobs_user_created ON tts_jobs (user_id, created_at DESC);
