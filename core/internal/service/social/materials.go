package social

import (
	"archive/zip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/xml"
	"errors"
	"image"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"mime"
	"os"
	"path"
	"path/filepath"
	"strings"

	"billionmail-core/internal/service/governance"
	"billionmail-core/internal/service/growth"
	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/util/gconv"
	"github.com/google/uuid"
	"github.com/xuri/excelize/v2"
	_ "golang.org/x/image/webp"
)

const MaxUploadBytes = int64(128 * 1024 * 1024)

func storageRoot() string { return filepath.Join(growth.StorageRoot(), "social") }
func allowedFile(name string) bool {
	switch strings.ToLower(filepath.Ext(name)) {
	case ".xlsx", ".zip", ".pdf", ".docx", ".pptx", ".txt", ".md", ".jpg", ".jpeg", ".png", ".webp", ".mp4":
		return true
	}
	return false
}
func assetPath(row gdb.Record) (string, error) {
	key := row["storage_key"].String()
	if _, e := uuid.Parse(key); e != nil {
		return "", errors.New("invalid asset key")
	}
	p := filepath.Join(storageRoot(), key)
	info, e := os.Lstat(p)
	if e != nil || !info.Mode().IsRegular() {
		return "", errors.New("asset unavailable")
	}
	return p, nil
}
func saveAsset(ctx context.Context, p *governance.Principal, planID int64, parent, name string, input io.Reader, ai bool) (gdb.Record, error) {
	if !allowedFile(name) {
		return nil, errors.New("unsupported material format")
	}
	if e := os.MkdirAll(storageRoot(), 0700); e != nil {
		return nil, e
	}
	id := uuid.NewString()
	file, e := os.OpenFile(filepath.Join(storageRoot(), id), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if e != nil {
		return nil, e
	}
	digest := sha256.New()
	n, e := io.Copy(io.MultiWriter(file, digest), io.LimitReader(input, MaxUploadBytes+1))
	closeErr := file.Close()
	if e != nil || closeErr != nil || n == 0 || n > MaxUploadBytes {
		os.Remove(filepath.Join(storageRoot(), id))
		return nil, errors.New("empty or oversized material")
	}
	sum := hex.EncodeToString(digest.Sum(nil))
	old, e := g.DB().Model("bm_social_assets").Ctx(ctx).Where("plan_id", planID).Where("sha256", sum).Where("filename", name).One()
	if e != nil || len(old) > 0 {
		os.Remove(filepath.Join(storageRoot(), id))
		return old, e
	}
	values := g.Map{"id": id, "plan_id": planID, "dept_id": p.DeptID, "owner_id": p.ID, "filename": str(name, 500), "storage_key": id, "mime": mime.TypeByExtension(strings.ToLower(filepath.Ext(name))), "sha256": sum, "size": n, "parent_id": nil, "ai_allowed": ai}
	if parent != "" {
		values["parent_id"] = parent
	}
	_, e = g.DB().Model("bm_social_assets").Data(values).Insert()
	if e != nil {
		os.Remove(filepath.Join(storageRoot(), id))
		return nil, e
	}
	return g.DB().Model("bm_social_assets").Where("id", id).One()
}
func Upload(ctx context.Context, p *governance.Principal, planID int64, name string, input io.Reader, ai bool) (g.Map, error) {
	if _, e := scoped(ctx, p, "bm_social_plans", planID, "social:content:edit"); e != nil {
		return nil, e
	}
	row, e := saveAsset(ctx, p, planID, "", filepath.Base(strings.ReplaceAll(name, "\\", "/")), input, ai)
	if e != nil {
		return nil, e
	}
	if _, e = queue(ctx, p, planID, 0, "ingest", row["id"].String(), g.Map{}); e != nil {
		return nil, e
	}
	return g.Map{"id": row["id"].String(), "filename": row["filename"].String(), "status": row["status"].String()}, nil
}
func Asset(ctx context.Context, p *governance.Principal, id, permission string) (gdb.Record, error) {
	row, e := g.DB().Model("bm_social_assets").Ctx(ctx).Where("id", id).One()
	if e != nil {
		return nil, e
	}
	if len(row) == 0 {
		return nil, errors.New("asset unavailable")
	}
	if _, e = scoped(ctx, p, "bm_social_plans", row["plan_id"].Int64(), permission); e != nil {
		return nil, e
	}
	return row, nil
}
func MaterialDetail(ctx context.Context, p *governance.Principal, id string, page int, keyword string) (g.Map, error) {
	row, e := Asset(ctx, p, id, "social:plan:view")
	if e != nil {
		return nil, e
	}
	delete(row, "storage_key")
	if page < 1 {
		page = 1
	}
	q := g.DB().Model("bm_social_asset_rows").Ctx(ctx).Where("asset_id", id)
	if keyword != "" {
		q = q.Where("sku ILIKE ? OR name ILIKE ?", "%"+str(keyword, 100)+"%", "%"+str(keyword, 100)+"%")
	}
	count, e := q.Clone().Count()
	if e != nil {
		return nil, e
	}
	rows, e := q.Order("id").Page(page, 20).All()
	return g.Map{"asset": row, "rows": rows, "total": count}, e
}
func MaterialControl(ctx context.Context, p *governance.Principal, id, operation string, data g.Map) error {
	row, e := Asset(ctx, p, id, "social:content:edit")
	if e != nil {
		return e
	}
	switch operation {
	case "recognize":
		_, e = queue(ctx, p, row["plan_id"].Int64(), 0, "ingest", id, g.Map{})
		if e == nil {
			_, e = g.DB().Model("bm_social_assets").Where("id", id).Data(g.Map{"status": "queued", "approved": false, "error": ""}).Update()
		}
	case "approve":
		if row["status"].String() != "ready" {
			return errors.New("material recognition has not completed")
		}
		_, e = g.DB().Model("bm_social_assets").Where("id", id).Data("approved", true).Update()
	case "reject":
		_, e = g.DB().Model("bm_social_assets").Where("id", id).Data("approved", false).Update()
	case "archive_to_library":
		if row["status"].String() != "ready" || !row["approved"].Bool() {
			return errors.New("approve the material before archiving")
		}
		plan, e2 := scoped(ctx, p, "bm_social_plans", row["plan_id"].Int64(), "social:content:edit")
		if e2 != nil {
			return e2
		}
		file, e2 := assetPath(row)
		if e2 != nil {
			return e2
		}
		reader, e2 := os.Open(file)
		if e2 != nil {
			return e2
		}
		defer reader.Close()
		_, e = growth.Upload(ctx, p, plan["company_id"].Int64(), filepath.Base(row["filename"].String()), gconv.String(data["purpose"]), reader, row["ai_allowed"].Bool())
	default:
		return errors.New("unsupported material operation")
	}
	return e
}
func jsonSchema(props g.Map) g.Map {
	keys := []string{}
	for k := range props {
		keys = append(keys, k)
	}
	return g.Map{"type": "object", "properties": props, "required": keys, "additionalProperties": false}
}
func stringSchema() g.Map { return g.Map{"type": "string"} }
func materialRows(ctx context.Context, row gdb.Record, file string) error {
	book, e := excelize.OpenFile(file)
	if e != nil {
		return errors.New("invalid XLSX workbook")
	}
	defer book.Close()
	total := 0
	return g.DB().Transaction(ctx, func(ctx context.Context, tx gdb.TX) error {
		if _, e := tx.Model("bm_social_asset_rows").Where("asset_id", row["id"]).Delete(); e != nil {
			return e
		}
		for _, sheet := range book.GetSheetList() {
			rows, e := book.Rows(sheet)
			if e != nil {
				return e
			}
			headers := []string{}
			line := 0
			batch := []g.Map{}
			for rows.Next() {
				line++
				cells, e := rows.Columns()
				if e != nil {
					rows.Close()
					return e
				}
				if len(cells) == 0 {
					continue
				}
				if len(headers) == 0 {
					headers = cells
					continue
				}
				total++
				if total > 50000 {
					rows.Close()
					return errors.New("split workbook above 50000 rows")
				}
				fields := g.Map{}
				sku, name := "", ""
				for i, value := range cells {
					if i >= len(headers) {
						continue
					}
					header := headers[i]
					fields[header] = value
					key := strings.ToLower(strings.ReplaceAll(strings.ReplaceAll(strings.TrimSpace(header), "_", ""), " ", ""))
					switch key {
					case "sku", "itemno", "partnumber", "产品编号", "货号":
						sku = value
					case "name", "productname", "产品名称", "品名":
						name = value
					}
				}
				state := "valid"
				if sku == "" || name == "" {
					state = "review"
				}
				batch = append(batch, g.Map{"asset_id": row["id"], "sheet": sheet, "row_number": line, "sku": str(sku, 200), "name": str(name, 500), "fields": encode(fields), "state": state})
				if len(batch) >= 200 {
					if _, e = tx.Model("bm_social_asset_rows").Data(batch).Insert(); e != nil {
						rows.Close()
						return e
					}
					batch = nil
				}
			}
			rows.Close()
			if len(batch) > 0 {
				if _, e = tx.Model("bm_social_asset_rows").Data(batch).Insert(); e != nil {
					return e
				}
			}
		}
		_, e = tx.Model("bm_social_assets").Where("id", row["id"]).Data(g.Map{"category": "product_sheet", "extracted": encode(g.Map{"row_count": total, "preserve_text_codes": true})}).Update()
		return e
	})
}
func officeText(file string) (string, error) {
	r, e := zip.OpenReader(file)
	if e != nil {
		return "", errors.New("invalid Office document")
	}
	defer r.Close()
	var text strings.Builder
	var size uint64
	for _, entry := range r.File {
		if !strings.HasSuffix(entry.Name, ".xml") || (!strings.HasPrefix(entry.Name, "word/") && !strings.HasPrefix(entry.Name, "ppt/slides/")) {
			continue
		}
		size += entry.UncompressedSize64
		if size > 20*1024*1024 {
			return "", errors.New("Office document too large")
		}
		reader, e := entry.Open()
		if e != nil {
			return "", e
		}
		decoder := xml.NewDecoder(io.LimitReader(reader, 20*1024*1024))
		for {
			token, e := decoder.Token()
			if e == io.EOF {
				break
			}
			if e != nil {
				reader.Close()
				return "", errors.New("invalid Office XML")
			}
			if chars, ok := token.(xml.CharData); ok {
				text.Write(chars)
				text.WriteString(" ")
			}
			if text.Len() > 1000000 {
				reader.Close()
				return "", errors.New("split large Office document")
			}
		}
		reader.Close()
	}
	return text.String(), nil
}
func unpack(ctx context.Context, p *governance.Principal, row gdb.Record, file string) error {
	r, e := zip.OpenReader(file)
	if e != nil {
		return errors.New("invalid ZIP archive")
	}
	defer r.Close()
	if len(r.File) > 1000 {
		return errors.New("split archive above 1000 files")
	}
	var total uint64
	count := 0
	for _, entry := range r.File {
		if entry.FileInfo().IsDir() {
			continue
		}
		name := strings.ReplaceAll(entry.Name, "\\", "/")
		if strings.HasPrefix(name, "/") || strings.Contains(name, ":") || path.Clean(name) != name || strings.HasPrefix(name, "../") || name == ".." || entry.Mode()&os.ModeSymlink != 0 {
			return errors.New("unsafe archive path")
		}
		total += entry.UncompressedSize64
		if total > 512*1024*1024 || entry.UncompressedSize64 > 32*1024*1024 {
			return errors.New("archive expansion limit")
		}
		if !allowedFile(name) || strings.EqualFold(filepath.Ext(name), ".zip") {
			continue
		}
		reader, e := entry.Open()
		if e != nil {
			return e
		}
		child, e := saveAsset(ctx, p, row["plan_id"].Int64(), row["id"].String(), name, reader, row["ai_allowed"].Bool())
		reader.Close()
		if e != nil {
			return e
		}
		if _, e = queue(ctx, p, row["plan_id"].Int64(), 0, "ingest", child["id"].String(), g.Map{}); e != nil {
			return e
		}
		count++
	}
	_, e = g.DB().Model("bm_social_assets").Where("id", row["id"]).Data(g.Map{"category": "material_pack", "extracted": encode(g.Map{"child_count": count})}).Update()
	return e
}
func ingestMaterial(ctx context.Context, p *governance.Principal, id string) error {
	row, e := Asset(ctx, p, id, "social:content:edit")
	if e != nil {
		return e
	}
	file, e := assetPath(row)
	if e != nil {
		return e
	}
	_, e = g.DB().Model("bm_social_assets").Where("id", id).Data(g.Map{"status": "processing", "approved": false, "error": ""}).Update()
	if e != nil {
		return e
	}
	ext := strings.ToLower(filepath.Ext(row["filename"].String()))
	text := ""
	var pdf, imageData []byte
	category := "document"
	switch {
	case ext == ".xlsx":
		e = materialRows(ctx, row, file)
	case ext == ".zip":
		e = unpack(ctx, p, row, file)
	case strings.HasPrefix(row["mime"].String(), "image/"):
		var f *os.File
		f, e = os.Open(file)
		if e == nil {
			var info image.Config
			info, _, e = image.DecodeConfig(f)
			f.Close()
			if e == nil && (int64(info.Width)*int64(info.Height) > 36000000 || info.Width < 1 || info.Height < 1) {
				e = errors.New("invalid image dimensions")
			}
			if e == nil {
				category = "product_image"
				_, e = g.DB().Model("bm_social_assets").Where("id", id).Data(g.Map{"category": category, "extracted": encode(g.Map{"width": info.Width, "height": info.Height, "sku_candidates": []string{strings.TrimSuffix(filepath.Base(row["filename"].String()), ext)}})}).Update()
				if e == nil && row["ai_allowed"].Bool() {
					if row["size"].Int64() > 8*1024*1024 {
						e = errors.New("use images below 8 MB for recognition")
					} else {
						imageData, e = os.ReadFile(file)
					}
				}
			}
		}
	case ext == ".docx" || ext == ".pptx":
		text, e = officeText(file)
	case ext == ".pdf":
		if row["size"].Int64() > 20*1024*1024 {
			e = errors.New("split PDF above 20 MB")
		} else {
			pdf, e = os.ReadFile(file)
		}
	default:
		var b []byte
		b, e = os.ReadFile(file)
		if e == nil && len(b) > 1000000 {
			e = errors.New("split large text material")
		}
		text = string(b)
	}
	if e == nil && (text != "" || len(pdf) > 0 || len(imageData) > 0) {
		if row["ai_allowed"].Bool() {
			var result struct {
				Category string `json:"category"`
				Summary  string `json:"summary"`
				Facts    []struct {
					Key      string `json:"key"`
					Value    string `json:"value"`
					Evidence string `json:"evidence"`
				} `json:"facts"`
			}
			input := growth.AIInput{MaxOutput: 4000, Instruction: "Classify this company's supplied material as company_profile, catalog, certificate, sales_policy, technical, faq, quotation, internal, product_image. Extract only explicit evidence-backed facts. Mark cost, bank details and contracts internal. Do not infer certifications, warranty, fitment or prices. Never change the company or products. Return plain text summary and facts.", Data: g.Map{"filename": row["filename"].String(), "text": str(text, 60000)}, File: pdf, Filename: row["filename"].String(), Schema: jsonSchema(g.Map{"category": stringSchema(), "summary": stringSchema(), "facts": g.Map{"type": "array", "items": jsonSchema(g.Map{"key": stringSchema(), "value": stringSchema(), "evidence": stringSchema()})}})}
			input.Image = imageData
			input.ImageMIME = row["mime"].String()
			e = growth.SocialAI(ctx, p, "social_material", input, &result)
			if e == nil {
				_, e = g.DB().Model("bm_social_assets").Where("id", id).Data(g.Map{"category": str(result.Category, 60), "content": str(text, 60000), "extracted": encode(result)}).Update()
			}
		} else {
			_, e = g.DB().Model("bm_social_assets").Where("id", id).Data(g.Map{"category": category, "content": str(text, 60000)}).Update()
		}
	}
	state, errorText := "ready", ""
	if e != nil {
		state = "failed"
		errorText = e.Error()
	}
	_, err := g.DB().Model("bm_social_assets").Where("id", id).Data(g.Map{"status": state, "error": errorText, "version": gdb.Raw("version+1")}).Update()
	if e != nil {
		return e
	}
	return err
}
func Media(ctx context.Context, p *governance.Principal, plan gdb.Record, refs []string) ([]MediaFile, error) {
	return readMedia(ctx, p, plan, refs, true)
}
func readMedia(ctx context.Context, p *governance.Principal, plan gdb.Record, refs []string, approved bool) ([]MediaFile, error) {
	if len(refs) > 10 {
		return nil, errors.New("at most 10 images per post")
	}
	media := []MediaFile{}
	seen := map[string]bool{}
	for _, ref := range refs {
		if seen[ref] {
			return nil, errors.New("duplicate publishing image")
		}
		seen[ref] = true
		parts := strings.SplitN(ref, ":", 2)
		if len(parts) != 2 {
			return nil, errors.New("invalid image reference")
		}
		var row gdb.Record
		var file string
		var e error
		if parts[0] == "task" {
			row, e = Asset(ctx, p, parts[1], sourcePermission(p))
			if e != nil {
				return nil, e
			}
			needsApproval := approved || object(row["extracted"])["purpose"] != "post_image"
			if row["plan_id"].Int64() != plan["id"].Int64() || (needsApproval && !row["approved"].Bool()) || row["status"].String() != "ready" {
				return nil, errors.New("approve this task image first")
			}
			file, e = assetPath(row)
		} else if parts[0] == "library" {
			row, e = growth.Asset(ctx, p, parts[1], "material:list")
			if e != nil {
				return nil, e
			}
			if row["company_id"].Int64() != plan["company_id"].Int64() || !row["published"].Bool() || row["visibility"].String() != "public" {
				return nil, errors.New("select a published company image")
			}
			file, e = growth.BlobPath(row)
		} else {
			return nil, errors.New("invalid image source")
		}
		if e != nil {
			return nil, e
		}
		if row["category"].String() == "internal" {
			return nil, errors.New("internal material cannot be published")
		}
		if row["mime"].String() != "image/jpeg" && row["mime"].String() != "image/png" {
			return nil, errors.New("publishing images must be JPG or PNG")
		}
		if row["size"].Int64() > 8*1024*1024 {
			return nil, errors.New("publish image exceeds 8 MB")
		}
		data, e := os.ReadFile(file)
		if e != nil {
			return nil, e
		}
		if hashString(string(data)) != row["sha256"].String() {
			return nil, errors.New("stored publishing image checksum changed; review a new upload")
		}
		media = append(media, MediaFile{ID: ref, Filename: filepath.Base(row["filename"].String()), MIME: row["mime"].String(), Data: data, AIGenerated: object(row["extracted"])["kind"] == "ai_result"})
	}
	return media, nil
}

func MaterialFile(ctx context.Context, p *governance.Principal, id string) (string, string, string, error) {
	row, e := Asset(ctx, p, id, "social:plan:view")
	if e != nil {
		return "", "", "", e
	}
	file, e := assetPath(row)
	return file, row["filename"].String(), row["mime"].String(), e
}
