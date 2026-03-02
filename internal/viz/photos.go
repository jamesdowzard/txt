package viz

import (
	"encoding/base64"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/maxghenis/openmessage/internal/db"
)

// FindMediaMessages returns messages that have media attachments (images/videos).
func FindMediaMessages(messages []*db.Message) []*db.Message {
	var media []*db.Message
	for _, m := range messages {
		if m.MediaID != "" && isVisualMedia(m.MimeType) {
			media = append(media, m)
		}
	}
	return media
}

// isVisualMedia checks if a MIME type is an image or video.
func isVisualMedia(mime string) bool {
	return strings.HasPrefix(mime, "image/") || strings.HasPrefix(mime, "video/")
}

// EncodePhotosFromDir reads image files from a directory and returns them as
// base64 data URIs for inline <img> src. Files are sorted by name. If maxPhotos
// > 0, evenly samples that many across the set.
func EncodePhotosFromDir(dir string, maxPhotos int) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("read dir: %w", err)
	}

	var imagePaths []string
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		ext := strings.ToLower(filepath.Ext(e.Name()))
		switch ext {
		case ".jpg", ".jpeg", ".png", ".webp", ".gif":
			imagePaths = append(imagePaths, filepath.Join(dir, e.Name()))
		}
	}
	sort.Strings(imagePaths)

	if maxPhotos > 0 && len(imagePaths) > maxPhotos {
		sampled := make([]string, maxPhotos)
		step := float64(len(imagePaths)) / float64(maxPhotos)
		for i := range maxPhotos {
			sampled[i] = imagePaths[int(float64(i)*step)]
		}
		imagePaths = sampled
	}

	var dataURIs []string
	for _, p := range imagePaths {
		data, err := os.ReadFile(p)
		if err != nil {
			continue
		}
		mime := mimeFromExt(filepath.Ext(p))
		uri := fmt.Sprintf("data:%s;base64,%s", mime, base64.StdEncoding.EncodeToString(data))
		dataURIs = append(dataURIs, uri)
	}
	return dataURIs, nil
}

func mimeFromExt(ext string) string {
	switch strings.ToLower(ext) {
	case ".jpg", ".jpeg":
		return "image/jpeg"
	case ".png":
		return "image/png"
	case ".webp":
		return "image/webp"
	case ".gif":
		return "image/gif"
	default:
		return "image/jpeg"
	}
}
