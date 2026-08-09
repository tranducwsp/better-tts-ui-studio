-- Khôi phục lại bảng voices nếu cần quay lại. Không có dữ liệu nào mất vì
-- bảng không bao giờ được ghi.
CREATE TABLE IF NOT EXISTS voices (
    id VARCHAR(64) PRIMARY KEY,
    name VARCHAR(255) NOT NULL,
    gender VARCHAR(50),
    descriptions TEXT[],
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);
