// Command gen-env sinh .env.example từ bảng đặc tả trong config/settings.go.
//
// Chạy bằng `go generate ./config` sau khi thêm hoặc sửa một biến. Tệp sinh ra được commit
// để người vận hành đọc được mà không cần công cụ Go, và config.TestEnvExampleMatchesSpec
// đối chiếu tệp đã commit với bảng, nên quên chạy generate sẽ làm test đỏ.
package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"core-backend/config"
)

const header = `# Sinh tự động từ core-backend/config/settings.go — đừng sửa tay.
# Cập nhật bằng: cd core-backend && go generate ./config
#
# Sao chép thành .env rồi điền trước khi triển khai. Giá trị hiển thị là mặc định backend
# dùng khi biến không được đặt; biến nào đánh dấu BẮT BUỘC thì không có mặc định an toàn.
`

func main() {
	// go generate chạy với cwd là thư mục chứa chỉ thị (config/), nên lùi hai cấp để tới
	// gốc kho mã. Cho phép ghi đè để chạy tay từ chỗ khác.
	out := os.Getenv("GEN_ENV_OUT")
	if out == "" {
		out = filepath.Join("..", "..", ".env.example")
	}
	out, err := filepath.Abs(out)
	if err != nil {
		fmt.Fprintln(os.Stderr, "gen-env:", err)
		os.Exit(1)
	}

	var b strings.Builder
	b.WriteString(header)

	lastGroup := ""
	for _, s := range config.Settings {
		if s.Group != lastGroup {
			b.WriteString("\n" + sectionRule(s.Group) + "\n")
			lastGroup = s.Group
		}

		if s.Required {
			b.WriteString("# BẮT BUỘC cho môi trường thật.\n")
		}
		for _, line := range docLines(s.Doc) {
			b.WriteString("# " + line + "\n")
		}
		if s.Kind == config.KindInt {
			fmt.Fprintf(&b, "# Số nguyên trong khoảng %d đến %d.\n", s.Min, s.Max)
		}
		if len(s.Aliases) > 0 {
			fmt.Fprintf(&b, "# Tên cũ vẫn dùng được: %s\n", strings.Join(s.Aliases, ", "))
		}

		// Bí mật không bao giờ được ghi giá trị vào tệp mẫu, kể cả khi mặc định là rỗng —
		// để không ai vô tình commit một giá trị thật vào đúng chỗ này.
		value := s.Default
		if s.Kind == config.KindSecret {
			value = ""
		}
		fmt.Fprintf(&b, "%s=%s\n", s.Key, value)
	}

	if err := os.WriteFile(out, []byte(b.String()), 0644); err != nil {
		fmt.Fprintln(os.Stderr, "gen-env:", err)
		os.Exit(1)
	}
	fmt.Printf("gen-env: đã ghi %s (%d biến)\n", out, len(config.Settings))
}

// sectionRule tạo dải phân cách canh đều 90 cột cho dễ đọc.
func sectionRule(group string) string {
	const width = 90
	prefix := "# ── " + group + " "
	if n := width - len([]rune(prefix)); n > 0 {
		return prefix + strings.Repeat("─", n)
	}
	return prefix
}

func docLines(doc string) []string {
	if strings.TrimSpace(doc) == "" {
		return nil
	}
	return strings.Split(doc, "\n")
}
