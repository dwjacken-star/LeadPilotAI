package growth

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"mime"
	"os"
	"path/filepath"
	"strings"

	"billionmail-core/internal/service/governance"
	"billionmail-core/internal/service/public"
	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/google/uuid"
)

const MaxUploadBytes = int64(128 * 1024 * 1024)

func StorageRoot() string {
	if root := os.Getenv("MAPLEMAIL_GROWTH_DATA"); root != "" {
		return root
	}
	return public.AbsPath("../core/data/growth-files")
}
func supportedFile(name string) bool {
	switch strings.ToLower(filepath.Ext(name)) {
	case ".xlsx", ".zip", ".pdf", ".docx", ".pptx", ".txt", ".md", ".jpg", ".jpeg", ".png", ".webp":
		return true
	}
	return false
}
func Asset(ctx context.Context, p *governance.Principal, id, permission string) (gdb.Record, error) {
	row, e := g.DB().Model("bm_growth_assets").Ctx(ctx).Where("id", id).WhereNull("deleted_at").One()
	if e != nil || len(row) == 0 {
		return nil, errors.New("material not found")
	}
	shared := permission == "material:list" || permission == "material:download"
	if _, e = company(ctx, p, row["company_id"].Int64(), permission, shared); e != nil {
		return nil, e
	}
	if shared {
		if _, e = company(ctx, p, row["company_id"].Int64(), permission, false); e != nil && (!row["published"].Bool() || row["visibility"].String() != "public") {
			return nil, errors.New("material is private")
		}
	}
	return row, nil
}
func BlobPath(row gdb.Record) (string, error) {
	key := row["storage_key"].String()
	if _, e := uuid.Parse(key); e != nil {
		return "", errors.New("invalid storage key")
	}
	path := filepath.Join(StorageRoot(), key)
	info, e := os.Lstat(path)
	if e != nil {
		return "", e
	}
	if !info.Mode().IsRegular() {
		return "", errors.New("invalid stored file")
	}
	return path, nil
}
func saveBlob(ctx context.Context, p *governance.Principal, companyID int64, name, parent, purpose string, reader io.Reader, ai bool) (gdb.Record, error) {
	if !supportedFile(name) {
		return nil, errors.New("unsupported file; use XLSX, ZIP, PDF, DOCX, PPTX, TXT or images")
	}
	name = cleanText(strings.ReplaceAll(name, "\\", "/"), 600)
	if name == "" || strings.ContainsRune(name, 0) {
		return nil, errors.New("invalid filename")
	}
	if e := os.MkdirAll(StorageRoot(), 0700); e != nil {
		return nil, e
	}
	id := uuid.NewString()
	path := filepath.Join(StorageRoot(), id)
	file, e := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if e != nil {
		return nil, e
	}
	hash := sha256.New()
	n, e := io.Copy(io.MultiWriter(file, hash), io.LimitReader(reader, MaxUploadBytes+1))
	closeErr := file.Close()
	if e != nil || closeErr != nil || n > MaxUploadBytes || n == 0 {
		_ = os.Remove(path)
		return nil, errors.New("invalid file or file exceeds 128 MB")
	}
	sum := hex.EncodeToString(hash.Sum(nil))
	old, e := g.DB().Model("bm_growth_assets").Ctx(ctx).Where("company_id", companyID).Where("sha256", sum).Where("filename", name).One()
	if e != nil {
		_ = os.Remove(path)
		return nil, e
	}
	if len(old) > 0 {
		_ = os.Remove(path)
		if !old["deleted_at"].IsNil() {
			return nil, errors.New("this file was archived; restore it before importing again")
		}
		return old, nil
	}
	kind := mime.TypeByExtension(strings.ToLower(filepath.Ext(name)))
	if kind == "" {
		kind = "application/octet-stream"
	}
	category := "unclassified"
	if purpose == "products" && strings.EqualFold(filepath.Ext(name), ".xlsx") {
		category = "product_sheet"
	}
	values := g.Map{"id": id, "company_id": companyID, "dept_id": p.DeptID, "owner_id": p.ID, "filename": name, "storage_key": id, "mime": kind, "sha256": sum, "size": n, "category": category, "parent_id": parent, "ai_allowed": ai, "status": "uploaded"}
	if _, e = g.DB().Model("bm_growth_assets").Ctx(ctx).Data(values).Insert(); e != nil {
		_ = os.Remove(path)
		return nil, e
	}
	return g.DB().Model("bm_growth_assets").Ctx(ctx).Where("id", id).One()
}
func Upload(ctx context.Context, p *governance.Principal, companyID int64, name, purpose string, reader io.Reader, ai bool) (g.Map, error) {
	if _, e := company(ctx, p, companyID, "material:upload", false); e != nil {
		return nil, e
	}
	if purpose == "products" {
		if e := require(p, "product:import"); e != nil {
			return nil, e
		}
	}
	row, e := saveBlob(ctx, p, companyID, filepath.Base(name), "", purpose, reader, ai)
	if e != nil {
		return nil, e
	}
	job := int64(0)
	if p.Has("material:recognize") && row["status"].String() == "uploaded" {
		job, e = Queue(ctx, p, "ingest", row["id"].String(), "ingest:"+row["id"].String(), g.Map{"model": selectedModel(ctx)})
		if e == nil {
			_, e = g.DB().Model("bm_growth_assets").Where("id", row["id"]).Data(g.Map{"status": "queued"}).Update()
		}
	}
	return g.Map{"id": row["id"].String(), "job_id": job, "filename": row["filename"].String()}, e
}
func RecognizeAsset(ctx context.Context, p *governance.Principal, id string) (int64, error) {
	row, e := Asset(ctx, p, id, "material:recognize")
	if e != nil {
		return 0, e
	}
	if row["category"].String() == "product_sheet" && !p.Has("product:import") {
		return 0, errors.New("product import permission required")
	}
	// A deliberate retry gets a new job; automatic retries never repeat completed imports.
	return Queue(ctx, p, "ingest", id, "recognize:"+uuid.NewString(), g.Map{"model": selectedModel(ctx)})
}
func UpdateAsset(ctx context.Context, p *governance.Principal, id string, data g.Map) error {
	row, e := Asset(ctx, p, id, "material:manage")
	if e != nil {
		return e
	}
	visibility := fmt.Sprint(data["visibility"])
	if visibility != "public" && visibility != "internal" && visibility != "restricted" {
		return errors.New("invalid visibility")
	}
	values := g.Map{"category": cleanText(fmt.Sprint(data["category"]), 100), "visibility": visibility, "ai_allowed": data["ai_allowed"] == true, "version": gdb.Raw("version+1")}
	if visibility != "public" {
		values["published"] = false
	}
	_, e = g.DB().Model("bm_growth_assets").Ctx(ctx).Where("id", id).Data(values).Update()
	if e == nil {
		e = version(ctx, p, row["company_id"].Int64(), "asset", id, row["version"].Int()+1, values)
	}
	return e
}
func PublishAsset(ctx context.Context, p *governance.Principal, id string, publish bool) error {
	row, e := Asset(ctx, p, id, "material:publish")
	if e != nil {
		return e
	}
	if publish && (row["visibility"].String() != "public" || row["status"].String() != "ready") {
		return errors.New("only completed public materials may be published")
	}
	_, e = g.DB().Model("bm_growth_assets").Ctx(ctx).Where("id", id).Data(g.Map{"published": publish, "version": gdb.Raw("version+1")}).Update()
	if e == nil {
		delete(row, "storage_key")
		row["published"] = g.NewVar(publish)
		e = version(ctx, p, row["company_id"].Int64(), "asset", id, row["version"].Int()+1, row)
	}
	return e
}
func AttachImage(ctx context.Context, p *governance.Principal, productID int64, assetID, role string, confirmed bool) error {
	product, e := Product(ctx, p, productID, "product:manage")
	if e != nil {
		return e
	}
	asset, e := Asset(ctx, p, assetID, "material:manage")
	if e != nil {
		return e
	}
	if asset["company_id"].Int64() != product["company_id"].Int64() || !strings.HasPrefix(asset["mime"].String(), "image/") {
		return errors.New("image must belong to the same company")
	}
	if role != "main" && role != "side" && role != "label" && role != "packaging" && role != "dimensions" && role != "candidate" {
		return errors.New("invalid image role")
	}
	_, e = g.DB().Model("bm_growth_product_assets").Ctx(ctx).Data(g.Map{"product_id": productID, "asset_id": assetID, "role": role, "confirmed": confirmed, "evidence": "manual"}).OnConflict("product_id,asset_id").Save()
	return e
}
