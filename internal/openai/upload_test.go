package openai

import (
	"bytes"
	"encoding/base64"
	"mime/multipart"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestParseUploadItemsMultipart(t *testing.T) {
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	part, err := writer.CreateFormFile("file", "hello.txt")
	if err != nil {
		t.Fatalf("CreateFormFile() error = %v", err)
	}
	if _, err := part.Write([]byte("hello")); err != nil {
		t.Fatalf("Write() error = %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}

	req := httptest.NewRequest("POST", "/v1/uploads", &body)
	req.Header.Set("Content-Type", writer.FormDataContentType())

	items, err := parseUploadItems(req)
	if err != nil {
		t.Fatalf("parseUploadItems() error = %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("len(items) = %d, want 1", len(items))
	}
	if items[0].Filename != "hello.txt" {
		t.Fatalf("Filename = %q, want hello.txt", items[0].Filename)
	}
	if string(items[0].Content) != "hello" {
		t.Fatalf("Content = %q, want hello", string(items[0].Content))
	}
}

func TestParseUploadItemsJSONDataURI(t *testing.T) {
	raw := base64.StdEncoding.EncodeToString([]byte("hello"))
	req := httptest.NewRequest("POST", "/v1/uploads", strings.NewReader(`{"filename":"a.txt","data":"data:text/plain;base64,`+raw+`"}`))
	req.Header.Set("Content-Type", "application/json")

	items, err := parseUploadItems(req)
	if err != nil {
		t.Fatalf("parseUploadItems() error = %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("len(items) = %d, want 1", len(items))
	}
	if items[0].ContentType != "text/plain" {
		t.Fatalf("ContentType = %q, want text/plain", items[0].ContentType)
	}
	if string(items[0].Content) != "hello" {
		t.Fatalf("Content = %q, want hello", string(items[0].Content))
	}
}
