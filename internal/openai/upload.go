package openai

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"path/filepath"
	"strings"
	"time"
)

type uploadItem struct {
	Filename    string
	ContentType string
	Content     []byte
}

func (h *Handler) HandleUploads(w http.ResponseWriter, r *http.Request) {
	items, err := parseUploadItems(r)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{
			"error": map[string]any{"message": err.Error()},
		})
		return
	}
	if len(items) == 0 {
		writeJSON(w, http.StatusBadRequest, map[string]any{
			"error": map[string]any{"message": "至少上传一个文件"},
		})
		return
	}

	session, err := h.accounts.GetAccountSession()
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]any{
			"error": map[string]any{"message": err.Error()},
		})
		return
	}

	results := make([]map[string]any, 0, len(items))
	for _, item := range items {
		filename := strings.TrimSpace(item.Filename)
		if filename == "" {
			filename = fmt.Sprintf("upload_%d%s", time.Now().UnixNano(), extensionForContentType(item.ContentType))
		}

		fileURL, fileID, err := h.qwen.UploadFile(r.Context(), session.Token, filename, item.Content, item.ContentType)
		if err != nil {
			h.accounts.RecordFailure(session.Email)
			writeJSON(w, http.StatusBadGateway, map[string]any{
				"error": map[string]any{"message": err.Error()},
			})
			return
		}

		results = append(results, map[string]any{
			"filename":     filename,
			"content_type": item.ContentType,
			"size":         len(item.Content),
			"url":          fileURL,
			"file_id":      fileID,
		})
	}

	h.accounts.ResetFailure(session.Email)
	writeJSON(w, http.StatusOK, map[string]any{
		"object": "list",
		"data":   results,
	})
}

func parseUploadItems(r *http.Request) ([]uploadItem, error) {
	contentType := strings.ToLower(strings.TrimSpace(r.Header.Get("Content-Type")))
	switch {
	case strings.HasPrefix(contentType, "multipart/form-data"):
		if err := r.ParseMultipartForm(256 << 20); err != nil {
			return nil, errors.New("无法解析 multipart 请求")
		}
		return collectUploadMultipartItems(r.MultipartForm)
	case strings.HasPrefix(contentType, "application/json"):
		return parseJSONUploadItems(r)
	default:
		return parseRawUploadItem(r)
	}
}

func parseJSONUploadItems(r *http.Request) ([]uploadItem, error) {
	var payload struct {
		Filename    string `json:"filename"`
		ContentType string `json:"content_type"`
		Data        string `json:"data"`
		Base64      string `json:"base64"`
	}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		return nil, errors.New("JSON 请求体格式错误")
	}

	rawData := strings.TrimSpace(payload.Data)
	if rawData == "" {
		rawData = strings.TrimSpace(payload.Base64)
	}
	if rawData == "" {
		return nil, errors.New("JSON 上传缺少 data/base64 字段")
	}

	contentType := strings.TrimSpace(payload.ContentType)
	if matches := dataURIExpr.FindStringSubmatch(rawData); len(matches) == 3 {
		contentType = fallbackUploadContentType(contentType, matches[1], payload.Filename)
		decoded, err := base64.StdEncoding.DecodeString(matches[2])
		if err != nil {
			return nil, errors.New("data URI base64 解码失败")
		}
		return []uploadItem{{
			Filename:    payload.Filename,
			ContentType: contentType,
			Content:     decoded,
		}}, nil
	}

	decoded, err := base64.StdEncoding.DecodeString(rawData)
	if err != nil {
		return nil, errors.New("base64 解码失败")
	}
	return []uploadItem{{
		Filename:    payload.Filename,
		ContentType: fallbackUploadContentType(contentType, "", payload.Filename),
		Content:     decoded,
	}}, nil
}

func parseRawUploadItem(r *http.Request) ([]uploadItem, error) {
	content, err := io.ReadAll(r.Body)
	if err != nil {
		return nil, errors.New("读取上传内容失败")
	}
	if len(content) == 0 {
		return nil, nil
	}

	filename := strings.TrimSpace(r.URL.Query().Get("filename"))
	if filename == "" {
		filename = strings.TrimSpace(r.Header.Get("X-Filename"))
	}
	if filename == "" {
		filename = fmt.Sprintf("upload_%d%s", time.Now().UnixNano(), extensionForContentType(r.Header.Get("Content-Type")))
	}

	return []uploadItem{{
		Filename:    filename,
		ContentType: fallbackUploadContentType(r.Header.Get("Content-Type"), "", filename),
		Content:     content,
	}}, nil
}

func collectUploadMultipartItems(form *multipart.Form) ([]uploadItem, error) {
	if form == nil {
		return nil, nil
	}
	keys := []string{"file", "files", "image", "images", "video", "videos"}
	result := make([]uploadItem, 0)
	for _, key := range keys {
		headers := form.File[key]
		for _, header := range headers {
			file, err := header.Open()
			if err != nil {
				return nil, err
			}
			content, err := io.ReadAll(file)
			_ = file.Close()
			if err != nil {
				return nil, err
			}
			result = append(result, uploadItem{
				Filename:    header.Filename,
				ContentType: fallbackUploadContentType(header.Header.Get("Content-Type"), "", header.Filename),
				Content:     content,
			})
		}
	}
	return result, nil
}

func fallbackUploadContentType(values ...string) string {
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			return value
		}
	}
	return "application/octet-stream"
}

func extensionForContentType(contentType string) string {
	contentType = strings.TrimSpace(contentType)
	if contentType == "" {
		return ".bin"
	}
	exts, _ := mime.ExtensionsByType(contentType)
	if len(exts) > 0 && strings.TrimSpace(exts[0]) != "" {
		return exts[0]
	}
	if ext := filepath.Ext(contentType); ext != "" {
		return ext
	}
	return ".bin"
}
