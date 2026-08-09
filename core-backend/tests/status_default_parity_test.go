package tests

import (
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
)

// C9 giữ cho tts_chunks.status có cùng DEFAULT giữa schema.sql và chuỗi migration.
//
// schema.sql là bản đồ tổng hợp được duy trì tay; DB thật dựng từ migrations/*.up.sql theo
// thứ tự. Hai nguồn từng lệch: migration 000006 đề DEFAULT 'processing', schema.sql ghi
// 'pending' — không gây lỗi chạy (mọi insert đã chỉ status tường minh) nhưng bất kỳ công cụ
// nào sinh schema rồi so với DB điều hành sẽ báo "khác nhau" hoài.
//
// Kiểm bằng cách mô phỏng hiệu lực cuối của DEFAULT theo chuỗi migration (lần ghi sau thắng)
// rồi so với giá trị schema.sql khai — không cần Postgres thật, đúng phong cách
// TestPythonSchemaParity.
func TestTTSChunksStatusDefaultParity(t *testing.T) {
	effDefault := ""
	for _, sql := range readUpMigrations(t, filepath.Join("..", "..", "core-backend", "db", "migrations")) {
		if v, ok := alterStatusDefault(sql); ok {
			effDefault = v
			continue
		}
		if v, ok := createTableStatusDefault(sql); ok {
			effDefault = v
		}
	}
	if effDefault == "" {
		t.Fatalf("không thấy khai báo DEFAULT của tts_chunks.status trong toàn bộ migration — regex có thể đã lệch")
	}

	schema, err := os.ReadFile(filepath.Clean(filepath.Join("..", "..", "core-backend", "db", "schema.sql")))
	if err != nil {
		t.Fatalf("đọc schema.sql: %v", err)
	}
	schemaDefault, ok := createTableStatusDefault(string(schema))
	if !ok {
		t.Fatalf("schema.sql không khai DEFAULT cho tts_chunks.status — bảng có còn tồn tại?")
	}

	if schemaDefault != effDefault {
		t.Errorf("default lệch: schema.sql '%s' vs toàn chuỗi migration '%s'. Nếu migration đổi "+
			"default status, hãy cập nhật schema.sql cùng lượt (hoặc thêm migration bù) — đừng để "+
			"hai bản ghi 'đúng' cùng lúc.", schemaDefault, effDefault)
	}
}

// readUpMigrations đọc mọi tệp .up.sql theo thứ tự số.
func readUpMigrations(t *testing.T, dir string) []string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("đọc thư mục migration %s: %v", dir, err)
	}

	names := make([]string, 0, len(entries))
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".up.sql") {
			continue
		}
		names = append(names, e.Name())
	}
	sort.Strings(names)

	out := make([]string, 0, len(names))
	for _, n := range names {
		raw, err := os.ReadFile(filepath.Join(dir, n))
		if err != nil {
			t.Fatalf("đọc migration %s: %v", n, err)
		}
		out = append(out, string(raw))
	}
	return out
}

// ttsChunksCreateRe khớp trọn khối CREATE TABLE tts_chunks. Các cột dùng VARCHAR(..) tức có
// `)` ở giữa, nhưng không `;` nào nằm trong khối, nên `\);` chỉ khớp đúng điểm kết khối.
// statusDefaultRe lấy DEFAULT trong khối đó.
var (
	ttsChunksCreateRe = regexp.MustCompile(`(?s)CREATE TABLE IF NOT EXISTS tts_chunks\s*\((.*?)\)\s*;`)
	statusDefaultRe   = regexp.MustCompile(`(?i)\bstatus\s+VARCHAR\(50\)\s+NOT NULL\s+DEFAULT\s+'([^']+)'`)
	alterStatusRe     = regexp.MustCompile(`(?i)ALTER\s+TABLE\s+tts_chunks\s+ALTER\s+COLUMN\s+status\s+SET\s+DEFAULT\s+'([^']+)'`)
)

// createTableStatusDefault trả về DEFAULT của status khi khối tạo tts_chunks khai nó.
func createTableStatusDefault(sql string) (string, bool) {
	m := ttsChunksCreateRe.FindStringSubmatch(sql)
	if m == nil {
		return "", false
	}
	d := statusDefaultRe.FindStringSubmatch(m[1])
	if d == nil {
		return "", false
	}
	return d[1], true
}

// alterStatusDefault trả về DEFAULT được một lệnh ALTER tts_chunks... đặt (lần ghi sau thắng).
func alterStatusDefault(sql string) (string, bool) {
	m := alterStatusRe.FindStringSubmatch(sql)
	if m == nil {
		return "", false
	}
	return m[1], true
}