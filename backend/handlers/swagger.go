package handlers

import (
	"net/http"
)

// OpenAPI3JSON chứa đặc tả OpenAPI 3.0 dạng JSON cho Better TTS UI Studio API.
const OpenAPI3JSON = `{
  "openapi": "3.0.3",
  "info": {
    "title": "Better TTS UI Studio API",
    "description": "Control Plane Web Backend API cho hệ thống tổng hợp giọng nói AI (TTS).",
    "version": "1.0.0"
  },
  "paths": {
    "/health": {
      "get": {
        "summary": "Health Check",
        "responses": { "200": { "description": "OK" } }
      }
    },
    "/ready": {
      "get": {
        "summary": "Readiness Check",
        "responses": { "200": { "description": "OK" } }
      }
    },
    "/api/info": {
      "get": {
        "summary": "Lấy thông tin Core Engine",
        "responses": { "200": { "description": "Thông tin engine & manifest" } }
      }
    },
    "/api/login": {
      "post": {
        "summary": "Đăng nhập người dùng",
        "requestBody": {
          "required": true,
          "content": {
            "application/json": {
              "schema": {
                "type": "object",
                "properties": {
                  "username": { "type": "string" },
                  "password": { "type": "string" }
                }
              }
            }
          }
        },
        "responses": { "200": { "description": "Đăng nhập thành công" } }
      }
    },
    "/api/synthesize/{mode}": {
      "post": {
        "summary": "Tạo task tổng hợp giọng nói (TTS)",
        "parameters": [
          { "name": "mode", "in": "path", "required": true, "schema": { "type": "string" } }
        ],
        "responses": { "202": { "description": "Task đã được tiếp nhận" } }
      }
    },
    "/api/tasks": {
      "get": {
        "summary": "Danh sách tasks của người dùng",
        "responses": { "200": { "description": "Danh sách tasks" } }
      }
    },
    "/api/tasks/{id}": {
      "get": {
        "summary": "Chi tiết / Trạng thái task",
        "parameters": [
          { "name": "id", "in": "path", "required": true, "schema": { "type": "string" } }
        ],
        "responses": { "200": { "description": "Thông tin task" } }
      }
    }
  }
}`

const SwaggerUIHTML = `<!DOCTYPE html>
<html lang="en">
<head>
  <meta charset="UTF-8">
  <title>Better TTS UI Studio - Swagger UI</title>
  <link rel="stylesheet" type="text/css" href="https://cdnjs.cloudflare.com/ajax/libs/swagger-ui/5.11.0/swagger-ui.css" />
  <style>
    html { box-sizing: border-box; overflow: -moz-scrollbars-vertical; overflow-y: scroll; }
    *, *:before, *:after { box-sizing: inherit; }
    body { margin: 0; background: #fafafa; }
  </style>
</head>
<body>
  <div id="swagger-ui"></div>
  <script src="https://cdnjs.cloudflare.com/ajax/libs/swagger-ui/5.11.0/swagger-ui-bundle.js"></script>
  <script src="https://cdnjs.cloudflare.com/ajax/libs/swagger-ui/5.11.0/swagger-ui-standalone-preset.js"></script>
  <script>
    window.onload = function() {
      const ui = SwaggerUIBundle({
        url: "/swagger/doc.json",
        dom_id: '#swagger-ui',
        deepLinking: true,
        presets: [
          SwaggerUIBundle.presets.apis,
          SwaggerUIStandalonePreset
        ],
        plugins: [
          SwaggerUIBundle.plugins.DownloadUrl
        ],
        layout: "StandaloneLayout"
      });
      window.ui = ui;
    };
  </script>
</body>
</html>`

// ServeSwaggerDoc trả về file OpenAPI JSON.
func ServeSwaggerDoc(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(OpenAPI3JSON))
}

// ServeSwaggerUI trả về giao diện Swagger UI HTML.
func ServeSwaggerUI(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(SwaggerUIHTML))
}
