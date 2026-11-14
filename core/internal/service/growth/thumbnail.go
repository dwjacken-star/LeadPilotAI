package growth

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"image"
	"image/jpeg"
	"os"
	"strings"

	"billionmail-core/internal/service/governance"
	"golang.org/x/image/draw"
)

func ImagePreview(ctx context.Context, p *governance.Principal, id string) (string, error) {
	r, e := Asset(ctx, p, id, "material:list")
	if e != nil {
		return "", e
	}
	if !strings.HasPrefix(r["mime"].String(), "image/") {
		return "", errors.New("image required")
	}
	path, e := BlobPath(r)
	if e != nil {
		return "", errors.New("image unavailable")
	}
	file, e := os.Open(path)
	if e != nil {
		return "", errors.New("image unavailable")
	}
	defer file.Close()
	cfg, _, e := image.DecodeConfig(file)
	if e != nil || int64(cfg.Width)*int64(cfg.Height) > 40000000 {
		return "", errors.New("invalid image")
	}
	file.Seek(0, 0)
	src, _, e := image.Decode(file)
	if e != nil {
		return "", errors.New("invalid image")
	}
	w, h := cfg.Width, cfg.Height
	if w > 600 {
		h = h * 600 / w
		w = 600
	}
	if h > 600 {
		w = w * 600 / h
		h = 600
	}
	if w < 1 {
		w = 1
	}
	if h < 1 {
		h = 1
	}
	thumb := image.NewRGBA(image.Rect(0, 0, w, h))
	draw.CatmullRom.Scale(thumb, thumb.Bounds(), src, src.Bounds(), draw.Over, nil)
	var result bytes.Buffer
	if e = jpeg.Encode(&result, thumb, &jpeg.Options{Quality: 80}); e != nil {
		return "", e
	}
	return "data:image/jpeg;base64," + base64.StdEncoding.EncodeToString(result.Bytes()), nil
}
