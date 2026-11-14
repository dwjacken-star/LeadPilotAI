package askai

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"github.com/gogf/gf/v2/frame/g"
	"image"
	"image/png"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestBrandImageStorageAndPublicRead(t *testing.T) {
	var buffer bytes.Buffer
	if e := png.Encode(&buffer, image.NewRGBA(image.Rect(0, 0, 2, 2))); e != nil {
		t.Fatal(e)
	}
	encoded := "data:image/png;base64," + base64.StdEncoding.EncodeToString(buffer.Bytes())
	for _, invalid := range []string{base64.StdEncoding.EncodeToString([]byte("<html>not an image</html>")), "data:image/svg+xml;base64," + base64.StdEncoding.EncodeToString([]byte(`<svg xmlns="http://www.w3.org/2000/svg" onload="alert(1)"></svg>`)), "data:image/svg+xml;base64," + base64.StdEncoding.EncodeToString([]byte(`<svg xmlns="http://www.w3.org/2000/svg"><script>alert(1)</script></svg>`))} {
		if _, _, e := decodeBrandImage(invalid); e == nil {
			t.Fatal("unsafe or invalid image was accepted")
		}
	}
	oldDirectory, oldBase := brandAssetsDirectory, brandAssetBaseURL
	brandAssetsDirectory = t.TempDir()
	brandAssetBaseURL = func() string { return "https://mail.example.test" }
	defer func() { brandAssetsDirectory = oldDirectory; brandAssetBaseURL = oldBase }()
	domain := "qa-brand-" + GetUUID() + ".test"
	projectPath := filepath.Join(PRODUCT_CONFIG_PATH, domain)
	t.Cleanup(func() { _ = os.RemoveAll(projectPath) })
	if e := SaveProjectConfig(domain, ProjectConfig{Domain: domain, ProjectName: "QA existing project", Description: "Keep this description"}); e != nil {
		t.Fatal(e)
	}
	urls := []string{}
	for _, tag := range []string{"primary logo", "secondary logo", "favicon"} {
		url, e := UploadImage(domain, encoded, "logo.png", "QA logo", tag)
		if e != nil {
			t.Fatal(e)
		}
		if !strings.HasPrefix(url, "https://mail.example.test/brand-assets/") {
			t.Fatal("image does not use this server", url)
		}
		urls = append(urls, url)
	}
	project, e := ReadProjectConfig(domain)
	if e != nil || project.PrimaryLogo != urls[0] || project.SecondaryLogo != urls[1] || project.Favicon != urls[2] || project.Description != "Keep this description" {
		t.Fatal("brand fields did not persist or other details changed", e)
	}
	images, e := ReadImagesConfig(domain)
	if e != nil || len(images) != 3 {
		t.Fatal("image library was not saved", e)
	}
	server := g.Server("brand-image-" + GetUUID())
	server.SetAddr("127.0.0.1:0")
	server.SetDumpRouterMap(false)
	server.BindHandler("/brand-assets/*any", ServeBrandAsset)
	if e = server.Start(); e != nil {
		t.Fatal(e)
	}
	defer server.Shutdown()
	response, e := http.Get(fmt.Sprintf("http://127.0.0.1:%d/brand-assets/%s", server.GetListenedPort(), filepath.Base(urls[0])))
	if e != nil {
		t.Fatal(e)
	}
	body, _ := io.ReadAll(response.Body)
	response.Body.Close()
	if response.StatusCode != 200 || response.Header.Get("Content-Type") != "image/png" || !bytes.Equal(body, buffer.Bytes()) || response.Header.Get("X-Content-Type-Options") != "nosniff" {
		t.Fatal("public image did not load correctly")
	}
	if e = RemoveImage(domain, images[0].ImageId); e != nil {
		t.Fatal(e)
	}
	if _, e = os.Stat(filepath.Join(brandAssetsDirectory, filepath.Base(urls[0]))); e != nil {
		t.Fatal("removing metadata broke a published image link")
	}
}
