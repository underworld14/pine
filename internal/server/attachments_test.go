package server

import (
	"bytes"
	"encoding/json"
	"image"
	"image/color"
	"image/png"
	"mime/multipart"
	"net/http"
	"strings"
	"testing"
)

func pngBytes(w, h int) []byte {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.Set(x, y, color.RGBA{uint8(x), uint8(y), 128, 255})
		}
	}
	var buf bytes.Buffer
	_ = png.Encode(&buf, img)
	return buf.Bytes()
}

func multipartImage(t *testing.T, field, filename string, data []byte) (string, []byte) {
	t.Helper()
	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	fw, err := mw.CreateFormFile(field, filename)
	if err != nil {
		t.Fatal(err)
	}
	fw.Write(data)
	mw.Close()
	return mw.FormDataContentType(), body.Bytes()
}

func TestAttachmentUploadServeDelete(t *testing.T) {
	ts := newTestServer(t)
	do(t, "POST", ts.URL+"/api/tickets", `{"type":"bug","title":"x"}`, nil)

	ct, body := multipartImage(t, "files", "shot.png", pngBytes(64, 64))
	req, _ := http.NewRequest("POST", ts.URL+"/api/tickets/BUG-001/attachments", bytes.NewReader(body))
	req.Header.Set("Content-Type", ct)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	respBody := readAll(t, resp)
	if resp.StatusCode != 201 {
		t.Fatalf("upload status %d: %s", resp.StatusCode, respBody)
	}
	var up struct {
		Attachments []attachResult `json:"attachments"`
	}
	json.Unmarshal([]byte(respBody), &up)
	if len(up.Attachments) != 1 {
		t.Fatalf("attachments = %d", len(up.Attachments))
	}
	name := up.Attachments[0].Name
	if !strings.Contains(up.Attachments[0].Markdown, "../attachments/BUG-001/") {
		t.Errorf("markdown = %s", up.Attachments[0].Markdown)
	}

	// Serve it.
	resp2, body2 := do(t, "GET", ts.URL+"/attachments/BUG-001/"+name, "", nil)
	if resp2.StatusCode != 200 {
		t.Fatalf("serve status %d", resp2.StatusCode)
	}
	if resp2.Header.Get("X-Content-Type-Options") != "nosniff" {
		t.Errorf("missing nosniff header")
	}
	if len(body2) == 0 {
		t.Errorf("served empty body")
	}

	// It appears in the ticket's attachments.
	_, snap := do(t, "GET", ts.URL+"/api/tickets/BUG-001", "", nil)
	if !strings.Contains(snap, name) {
		t.Errorf("ticket should list attachment: %s", snap)
	}

	// Delete it.
	resp3, _ := do(t, "DELETE", ts.URL+"/api/tickets/BUG-001/attachments/"+name, "", nil)
	if resp3.StatusCode != 204 {
		t.Fatalf("delete status %d", resp3.StatusCode)
	}
	resp4, _ := do(t, "GET", ts.URL+"/attachments/BUG-001/"+name, "", nil)
	if resp4.StatusCode != 404 {
		t.Errorf("attachment should be gone, got %d", resp4.StatusCode)
	}
}

func readAll(t *testing.T, resp *http.Response) string {
	t.Helper()
	defer resp.Body.Close()
	var b bytes.Buffer
	b.ReadFrom(resp.Body)
	return b.String()
}
