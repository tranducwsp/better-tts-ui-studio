-- 000011_add_updated_at_to_tts_chunks.up.sql

-- Mốc "lần cuối tiến triển" của một chunk. Trước đây bảng không có cột thời gian nào, nên
-- không thể hỏi "chunk này còn sống hay đã bị kẹt" — một chunk mồ côi do mất hàng đợi (Redis
-- restart) cứ đứng mãi ở 'processing'. updated_at là cái neo để cron reconcile khẳng định
-- chunk đã quá hạn: status chỉ đổi tại các mốc tiến triển, và mọi đường ghi phải dời nó về now().
ALTER TABLE tts_chunks ADD COLUMN IF NOT EXISTS updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP;