package upload

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"image"
	"image/jpeg"
	_ "image/jpeg"
	_ "image/png"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/google/uuid"
)

const avatarSize = 256
const maxAvatarRequestBytes = 8 << 20

type UploadHandler struct {
	uploadDir string
}

func NewHandler() *UploadHandler {
	uploadDir := "./web/uploads"
	os.MkdirAll(uploadDir, 0755)
	return &UploadHandler{uploadDir: uploadDir}
}

func (h *UploadHandler) UploadAvatar(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Image string `json:"image"`
	}

	r.Body = http.MaxBytesReader(w, r.Body, maxAvatarRequestBytes)
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"invalid json"}`, http.StatusBadRequest)
		return
	}

	if req.Image == "" {
		http.Error(w, `{"error":"no image provided"}`, http.StatusBadRequest)
		return
	}

	raw, err := decodeBase64Image(req.Image)
	if err != nil {
		http.Error(w, `{"error":"invalid image"}`, http.StatusBadRequest)
		return
	}

	img, _, err := image.Decode(bytes.NewReader(raw))
	if err != nil {
		http.Error(w, `{"error":"invalid image format"}`, http.StatusBadRequest)
		return
	}

	resized := cropToSquare(resizeImage(img, avatarSize))

	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, resized, &jpeg.Options{Quality: 85}); err != nil {
		http.Error(w, `{"error":"failed to process image"}`, http.StatusInternalServerError)
		return
	}

	filename := uuid.New().String() + ".jpg"
	filePath := filepath.Join(h.uploadDir, filename)

	if err := os.WriteFile(filePath, buf.Bytes(), 0644); err != nil {
		http.Error(w, `{"error":"failed to save"}`, http.StatusInternalServerError)
		return
	}

	url := "/uploads/" + filename
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]string{"url": url})
}

func decodeBase64Image(data string) ([]byte, error) {
	if idx := strings.Index(data, ","); idx != -1 {
		data = data[idx+1:]
	}
	return base64.StdEncoding.DecodeString(data)
}

func cropToSquare(img image.Image) image.Image {
	bounds := img.Bounds()
	size := bounds.Dx()
	if bounds.Dy() < size {
		size = bounds.Dy()
	}

	x0 := bounds.Min.X + (bounds.Dx()-size)/2
	y0 := bounds.Min.Y + (bounds.Dy()-size)/2

	dst := image.NewRGBA(image.Rect(0, 0, size, size))
	for y := 0; y < size; y++ {
		for x := 0; x < size; x++ {
			dst.Set(x, y, img.At(x0+x, y0+y))
		}
	}
	return dst
}

func resizeImage(src image.Image, size int) image.Image {
	bounds := src.Bounds()
	dst := image.NewRGBA(image.Rect(0, 0, size, size))

	for y := 0; y < size; y++ {
		for x := 0; x < size; x++ {
			srcX := bounds.Min.X + x*bounds.Dx()/size
			srcY := bounds.Min.Y + y*bounds.Dy()/size
			dst.Set(x, y, src.At(srcX, srcY))
		}
	}
	return dst
}
