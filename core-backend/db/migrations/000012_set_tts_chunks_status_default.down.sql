-- Đưa về giá trị migration 000006 đặt ra, để rollback khớp từng bước.

ALTER TABLE tts_chunks ALTER COLUMN status SET DEFAULT 'processing';