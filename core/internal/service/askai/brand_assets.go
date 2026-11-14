package askai

import (
	"billionmail-core/internal/service/domains"
	"billionmail-core/internal/service/public"
	"bytes"
	"encoding/base64"
	"encoding/binary"
	"encoding/xml"
	"errors"
	"fmt"
	"github.com/gogf/gf/v2/net/ghttp"
	_ "golang.org/x/image/webp"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
)

var brandAssetsDirectory = "../conf/brand-assets"
var brandAssetBaseURL = domains.GetBaseURL

var brandAssetName = regexp.MustCompile(`^[a-f0-9-]{36}\.(png|jpg|gif|webp|ico|svg)$`)
var projectImageLock sync.Mutex

func writeProjectFile(path string, data []byte) error {
	file, e := os.CreateTemp(filepath.Dir(path), ".maplemail-*.tmp")
	if e != nil {
		return e
	}
	temporary := file.Name()
	defer os.Remove(temporary)
	if e = file.Chmod(0644); e != nil {
		file.Close()
		return e
	}
	if _, e = file.Write(data); e != nil {
		file.Close()
		return e
	}
	if e = file.Close(); e != nil {
		return e
	}
	return os.Rename(temporary, path)
}

func decodeBrandImage(encoded string) ([]byte, string, error) {
	if len(encoded) > 7*1024*1024 {
		return nil, "", errors.New("upload_image_size")
	}
	if strings.HasPrefix(encoded, "data:") {
		parts := strings.SplitN(encoded, ",", 2)
		if len(parts) != 2 || !strings.Contains(parts[0], ";base64") {
			return nil, "", errors.New("upload_image_format")
		}
		encoded = parts[1]
	}
	raw, e := base64.StdEncoding.DecodeString(encoded)
	if e != nil || len(raw) == 0 {
		return nil, "", errors.New("upload_image_format")
	}
	if len(raw) > 5*1024*1024 {
		return nil, "", errors.New("upload_image_size")
	}
	mime := http.DetectContentType(raw)
	ext := map[string]string{"image/png": "png", "image/jpeg": "jpg", "image/gif": "gif", "image/webp": "webp", "image/x-icon": "ico", "image/vnd.microsoft.icon": "ico"}[mime]
	if ext == "ico" {
		if len(raw) < 6 || binary.LittleEndian.Uint16(raw[4:6]) == 0 {
			return nil, "", errors.New("upload_image_format")
		}
		return raw, ext, nil
	}
	if ext != "" {
		config, _, e := image.DecodeConfig(bytes.NewReader(raw))
		if e != nil || config.Width <= 0 || config.Height <= 0 || int64(config.Width)*int64(config.Height) > 25000000 {
			return nil, "", errors.New("upload_image_format")
		}
		return raw, ext, nil
	}
	if !bytes.Contains(bytes.ToLower(raw), []byte("<svg")) {
		return nil, "", errors.New("upload_image_format")
	}
	decoder := xml.NewDecoder(bytes.NewReader(raw))
	root := false
	for {
		token, e := decoder.Token()
		if e == io.EOF {
			break
		}
		if e != nil {
			return nil, "", errors.New("upload_image_format")
		}
		switch v := token.(type) {
		case xml.Directive:
			return nil, "", errors.New("upload_image_format")
		case xml.StartElement:
			name := strings.ToLower(v.Name.Local)
			if !root {
				if name != "svg" {
					return nil, "", errors.New("upload_image_format")
				}
				root = true
			}
			if name == "script" || name == "foreignobject" || name == "iframe" || name == "object" || name == "embed" {
				return nil, "", errors.New("upload_image_format")
			}
			for _, attr := range v.Attr {
				key := strings.ToLower(attr.Name.Local)
				value := strings.TrimSpace(attr.Value)
				if strings.HasPrefix(key, "on") || strings.Contains(strings.ToLower(value), "javascript:") || (key == "href" && !strings.HasPrefix(value, "#")) {
					return nil, "", errors.New("upload_image_format")
				}
			}
		}
	}
	if !root {
		return nil, "", errors.New("upload_image_format")
	}
	return raw, "svg", nil
}
func storeBrandImage(domain, encoded, filename, alt, tag string) (string, error) {
	if domain == "" || domain == "." || domain == ".." || strings.ContainsAny(domain, "/\\\x00") {
		return "", errors.New("upload_project_missing")
	}
	raw, ext, e := decodeBrandImage(encoded)
	if e != nil {
		return "", e
	}
	projectImageLock.Lock()
	defer projectImageLock.Unlock()
	project, e := ReadProjectConfig(domain)
	if e != nil {
		return "", errors.New("upload_project_missing")
	}
	if e = os.MkdirAll(brandAssetsDirectory, 0755); e != nil {
		return "", fmt.Errorf("Unable to create image storage: %w", e)
	}
	id := GetUUID()
	name := id + "." + ext
	path := filepath.Join(brandAssetsDirectory, name)
	if e = os.WriteFile(path, raw, 0644); e != nil {
		return "", e
	}
	assetURL := strings.TrimRight(brandAssetBaseURL(), "/") + "/brand-assets/" + name
	images, e := ReadImagesConfig(domain)
	if e != nil {
		_ = os.Remove(path)
		return "", e
	}
	images = append(images, ImageInfo{ImageId: id, ImageUrl: assetURL, Filename: filepath.Base(filename), AltText: alt, ImageTag: tag, UpdateTime: public.GetNowTime()})
	if e = SaveImagesConfig(domain, images); e != nil {
		_ = os.Remove(path)
		return "", e
	}
	switch strings.ToLower(strings.TrimSpace(tag)) {
	case "primary logo", "primary_logo", "logo":
		project.PrimaryLogo = assetURL
	case "secondary logo", "secondary_logo":
		project.SecondaryLogo = assetURL
	case "favicon":
		project.Favicon = assetURL
	default:
		return assetURL, nil
	}
	if e = SaveProjectConfig(domain, project); e != nil {
		return "", e
	}
	return assetURL, nil
}
func ServeBrandAsset(r *ghttp.Request) {
	if r.Method != "GET" && r.Method != "HEAD" {
		r.Response.WriteHeader(405)
		return
	}
	name := strings.TrimPrefix(r.URL.Path, "/brand-assets/")
	if !brandAssetName.MatchString(name) {
		r.Response.WriteHeader(404)
		return
	}
	path := filepath.Join(brandAssetsDirectory, name)
	info, e := os.Stat(path)
	if e != nil || info.IsDir() {
		r.Response.WriteHeader(404)
		return
	}
	mime := map[string]string{".png": "image/png", ".jpg": "image/jpeg", ".gif": "image/gif", ".webp": "image/webp", ".ico": "image/x-icon", ".svg": "image/svg+xml"}[filepath.Ext(name)]
	r.Response.Header().Set("Content-Type", mime)
	r.Response.Header().Set("X-Content-Type-Options", "nosniff")
	r.Response.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
	r.Response.Header().Set("Content-Security-Policy", "default-src 'none'; style-src 'unsafe-inline'; sandbox")
	r.Response.ServeFile(path)
}
