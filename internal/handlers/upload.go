package handlers

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/empowertours/empowertours-app/internal/middleware"
	"github.com/google/uuid"
)

type UploadHandler struct {
	UploadDir string // e.g. "./data/uploads"
	BaseURL   string // e.g. "http://192.168.1.98:8090" or "https://api.empowertours.xyz"
}

// UploadPhoto handles POST /upload/photo — multipart file upload
// Returns the public URL of the uploaded file.
func (h *UploadHandler) UploadPhoto(w http.ResponseWriter, r *http.Request) {
	_ = middleware.GetUserID(r.Context())

	// Max 10MB
	r.ParseMultipartForm(10 << 20)

	file, header, err := r.FormFile("photo")
	if err != nil {
		writeError(w, http.StatusBadRequest, "No photo file provided. Use form field 'photo'")
		return
	}
	defer file.Close()

	// Validate file type
	ext := strings.ToLower(filepath.Ext(header.Filename))
	allowed := map[string]bool{".jpg": true, ".jpeg": true, ".png": true, ".webp": true, ".heic": true}
	if !allowed[ext] {
		writeError(w, http.StatusBadRequest, "Invalid file type. Allowed: jpg, png, webp, heic")
		return
	}

	// Create date-based directory
	dateDir := time.Now().Format("2006/01/02")
	fullDir := filepath.Join(h.UploadDir, dateDir)
	if err := os.MkdirAll(fullDir, 0755); err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to create upload directory")
		return
	}

	// Generate unique filename
	filename := uuid.New().String() + ext
	filePath := filepath.Join(fullDir, filename)

	dst, err := os.Create(filePath)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to save file")
		return
	}
	defer dst.Close()

	written, err := io.Copy(dst, file)
	if err != nil {
		os.Remove(filePath)
		writeError(w, http.StatusInternalServerError, "Failed to write file")
		return
	}

	// Build public URL
	publicURL := fmt.Sprintf("%s/uploads/%s/%s", h.BaseURL, dateDir, filename)

	writeJSON(w, http.StatusCreated, map[string]interface{}{
		"url":      publicURL,
		"filename": filename,
		"size":     written,
	})
}
