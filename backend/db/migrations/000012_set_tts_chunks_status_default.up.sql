-- Migration 000006 đề ra DEFAULT 'processing' cho tts_chunks.status, nhưng một chunk chỉ nên
-- tự khai "đang xử lý" khi worker thực sự bắt đầu — dòng mới miễn phí được tạo với đúng nghĩa
-- "chờ việc". schema.sql (bản đồ tổng hợp, duy trì tay) đã ghi DEFAULT 'pending' từ đầu, nên
-- schema thật sau chuỗi migration và schema.sql lệch nhau: bất kỳ công cụ nào dựng DB từ
-- schema.sql rồi so với DB migration sẽ báo khác.

ALTER TABLE tts_chunks ALTER COLUMN status SET DEFAULT 'pending';