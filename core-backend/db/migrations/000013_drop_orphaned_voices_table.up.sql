-- Bảng voices (migration 000003) không bao giờ được sử dụng — code chỉ dùng user_voices.
-- Xoá để tránh nhầm lẫn và dọn rác trong cơ sở dữ liệu.
DROP TABLE IF EXISTS voices;
