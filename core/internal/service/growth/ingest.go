package growth

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"image"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strings"
	"unicode"

	"billionmail-core/internal/service/governance"
	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/util/gconv"
	"github.com/xuri/excelize/v2"
	_ "golang.org/x/image/webp"
)

var headerAliases = map[string]string{"sku": "sku", "itemno": "sku", "itemnumber": "sku", "partnumber": "sku", "产品编号": "sku", "货号": "sku", "内部编号": "sku", "产品编码": "sku", "型号": "sku", "name": "name", "productname": "name", "产品名称": "name", "品名": "name", "英文名称": "name_en", "category": "category", "分类": "category", "产品分类": "category", "oe": "oe", "oem": "oe", "oeno": "oe", "oe号": "oe", "原厂编号": "oe", "voltage": "voltage", "电压": "voltage", "refrigerant": "refrigerant", "冷媒": "refrigerant", "车型": "fitment", "fitment": "fitment", "适配车型": "fitment", "moq": "moq", "最低订量": "moq", "image": "image", "图片": "image", "图片路径": "image", "图片文件名": "image", "imagepath": "image", "cost": "internal_cost", "采购价": "internal_cost", "成本": "internal_cost", "price": "sales_price", "销售价": "sales_price", "currency": "currency", "币种": "currency"}

func headerKey(s string) string {
	return strings.Map(func(r rune) rune {
		if unicode.IsSpace(r) || r == '_' || r == '-' || r == '.' {
			return -1
		}
		return unicode.ToLower(r)
	}, strings.TrimSpace(s))
}
func mapHeaders(ctx context.Context, p *governance.Principal, headers []string) (map[int]string, error) {
	mapping := map[int]string{}
	hasSKU, hasName := false, false
	for i, h := range headers {
		key := headerAliases[headerKey(h)]
		if key == "" {
			key = cleanText(strings.TrimSpace(h), 100)
		}
		if key != "" {
			mapping[i] = key
		}
		hasSKU = hasSKU || key == "sku"
		hasName = hasName || key == "name"
	}
	if hasSKU && hasName {
		return mapping, nil
	}
	type entry struct {
		Column int    `json:"column"`
		Field  string `json:"field"`
	}
	var result struct {
		Mapping []entry `json:"mapping"`
	}
	input := AIInput{Instruction: "Map spreadsheet column indexes (zero based) to sku, name, category, oe, fitment, voltage, refrigerant, moq, image, currency, sales_price, internal_cost, or ignore. Do not infer values. Preserve identifying codes as text.", Data: headers, Schema: schema(g.Map{"mapping": g.Map{"type": "array", "items": schema(g.Map{"column": g.Map{"type": "integer"}, "field": strSchema()})}})}
	if e := IntelligenceProvider.Extract(ctx, p, "header_mapping", input, &result); e != nil {
		return nil, e
	}
	allowed := map[string]bool{"sku": true, "name": true, "category": true, "oe": true, "fitment": true, "voltage": true, "refrigerant": true, "moq": true, "image": true, "currency": true, "sales_price": true, "internal_cost": true, "ignore": true}
	for _, m := range result.Mapping {
		if m.Column < 0 || m.Column >= len(headers) || !allowed[m.Field] {
			return nil, errors.New("AI returned invalid header mapping")
		}
		mapping[m.Column] = m.Field
	}
	return mapping, nil
}
func ingestSheet(ctx context.Context, p *governance.Principal, asset gdb.Record) error {
	if e := require(p, "product:import"); e != nil {
		return e
	}
	file, e := BlobPath(asset)
	if e != nil {
		return e
	}
	book, e := excelize.OpenFile(file, excelize.Options{UnzipSizeLimit: 256 * 1024 * 1024})
	if e != nil {
		return errors.New("unable to read XLSX workbook")
	}
	defer book.Close()
	total := 0
	for _, sheet := range book.GetSheetList() {
		if sheet == templateInstructionsSheet {
			continue
		}
		rows, e := book.Rows(sheet)
		if e != nil {
			return e
		}
		if !rows.Next() {
			rows.Close()
			continue
		}
		headers, e := rows.Columns()
		if e != nil {
			rows.Close()
			return e
		}
		var mapping map[int]string
		if manual := gconv.Map(jsonMap(asset["extracted"])["header_mapping"]); len(manual) > 0 {
			mapping = map[int]string{}
			for key, value := range manual {
				index := gconv.Int(key)
				if index >= 0 && index < len(headers) {
					mapping[index] = gconv.String(value)
				}
			}
			e = nil
		} else {
			mapping, e = mapHeaders(ctx, p, headers)
		}
		if e != nil {
			rows.Close()
			return e
		}
		rowNumber := 1
		for rows.Next() {
			rowNumber++
			total++
			if total > 100000 {
				rows.Close()
				return errors.New("workbook exceeds 100000 data rows")
			}
			if ctx.Err() != nil {
				rows.Close()
				return ctx.Err()
			}
			values, e := rows.Columns()
			if e != nil {
				rows.Close()
				return e
			}
			if strings.TrimSpace(strings.Join(values, "")) == "" {
				continue
			}
			proposed := g.Map{}
			for index, key := range mapping {
				if index < len(values) && key != "ignore" {
					value := strings.TrimSpace(values[index])
					if value != "" {
						proposed[key] = value
					}
				}
			}
			sku := cleanText(gconv.String(proposed["sku"]), 128)
			name := cleanText(gconv.String(proposed["name"]), 300)
			state, reason := "imported", ""
			productID := int64(0)
			if sku == "" || name == "" {
				state, reason = "review", "sku_or_name_missing"
			} else {
				productID, reason, e = applyProductRow(ctx, p, asset, sheet, rowNumber, sku, name, proposed, false)
				if e != nil {
					rows.Close()
					return e
				}
				if reason != "" {
					state = "review"
				}
			}
			_, e = g.DB().Model("bm_growth_import_rows").Ctx(ctx).Data(g.Map{"asset_id": asset["id"], "sheet": sheet, "source_row": rowNumber, "sku": sku, "state": state, "reason": reason, "proposed": jsonText(proposed), "product_id": nullableID(productID)}).OnConflict("asset_id,sheet,source_row").Save()
			if e != nil {
				rows.Close()
				return e
			}
		}
		if e = rows.Error(); e != nil {
			rows.Close()
			return e
		}
		rows.Close()
		// Anchor-based images are extracted individually rather than relying on AI
		// to see embedded spreadsheet pictures.
		cells, _ := book.GetPictureCells(sheet)
		for _, cell := range cells {
			pictures, e := book.GetPictures(sheet, cell)
			if e != nil {
				continue
			}
			_, number, _ := excelize.CellNameToCoordinates(cell)
			record, _ := g.DB().Model("bm_growth_import_rows").Where("asset_id", asset["id"]).Where("sheet", sheet).Where("source_row", number).One()
			for index, picture := range pictures {
				child, e := saveBlob(ctx, p, asset["company_id"].Int64(), fmt.Sprintf("%s/%s-%s-%d%s", asset["filename"].String(), sheet, cell, index, picture.Extension), asset["id"].String(), "products", bytes.NewReader(picture.File), asset["ai_allowed"].Bool())
				if e != nil {
					return e
				}
				if e = inspectImage(ctx, p, child); e != nil {
					return e
				}
				if record["product_id"].Int64() > 0 {
					_, e = g.DB().Model("bm_growth_product_assets").Data(g.Map{"product_id": record["product_id"], "asset_id": child["id"], "role": "side", "confirmed": true, "evidence": sheet + "!" + cell}).OnConflict("product_id,asset_id").Save()
					if e != nil {
						return e
					}
				}
			}
		}
	}
	return matchImages(ctx, p, asset["company_id"].Int64())
}
func nullableID(id int64) interface{} {
	if id == 0 {
		return nil
	}
	return id
}
func applyProductRow(ctx context.Context, p *governance.Principal, asset gdb.Record, sheet string, row int, sku, name string, fields g.Map, force bool) (int64, string, error) {
	if _, e := company(ctx, p, asset["company_id"].Int64(), "product:import", false); e != nil {
		return 0, "", e
	}
	existing, e := g.DB().Model("bm_growth_products").Ctx(ctx).Where("company_id", asset["company_id"]).Where("sku", sku).One()
	if e != nil {
		return 0, "", e
	}
	proposed := g.Map{}
	for k, v := range fields {
		if k != "sku" && k != "name" && k != "category" {
			proposed[k] = v
		}
	}
	category := gconv.String(fields["category"])
	if len(existing) == 0 {
		values := g.Map{"company_id": asset["company_id"], "dept_id": p.DeptID, "owner_id": p.ID, "sku": sku, "name": name, "category": category, "fields": jsonText(proposed), "source_asset": asset["id"], "source_location": jsonText(g.Map{"sheet": sheet, "row": row})}
		id, e := g.DB().Model("bm_growth_products").Ctx(ctx).Data(values).InsertAndGetId()
		return id, "", e
	}
	id := existing["id"].Int64()
	if !existing["deleted_at"].IsNil() {
		return id, "product_archived", nil
	}
	if !p.Has("product:manage") {
		return id, "product_update_permission_required", nil
	}
	current := jsonMap(existing["fields"])
	protected := map[string]bool{}
	for _, k := range gconv.Strings(existing["manual_keys"]) {
		protected[k] = true
	}
	conflict := false
	for k, v := range proposed {
		if protected[k] {
			continue
		}
		if old := gconv.String(current[k]); old != "" && old != gconv.String(v) && !force {
			conflict = true
		}
	}
	if !protected["name"] && existing["name"].String() != name && !force {
		conflict = true
	}
	if category != "" && !protected["category"] && existing["category"].String() != "" && existing["category"].String() != category && !force {
		conflict = true
	}
	if conflict {
		return id, "existing_values_differ", nil
	}
	for k, v := range proposed {
		if !protected[k] {
			current[k] = v
		}
	}
	values := g.Map{"fields": jsonText(current)}
	if !protected["name"] {
		values["name"] = name
	}
	if category != "" && !protected["category"] {
		values["category"] = category
	}
	// Re-uploading identical data does not create false material versions.
	if jsonText(current) == jsonText(jsonMap(existing["fields"])) && (values["name"] == nil || values["name"] == existing["name"].String()) && (values["category"] == nil || values["category"] == existing["category"].String()) {
		return id, "", nil
	}
	values["version"] = gdb.Raw("version+1")
	_, e = g.DB().Model("bm_growth_products").Ctx(ctx).Where("id", id).Data(values).Update()
	return id, "", e
}
func ResolveImportRow(ctx context.Context, p *governance.Principal, id int64, data g.Map) error {
	row, e := g.DB().Model("bm_growth_import_rows").Ctx(ctx).Where("id", id).One()
	if e != nil || len(row) == 0 {
		return errors.New("import row not found")
	}
	asset, e := Asset(ctx, p, row["asset_id"].String(), "material:manage")
	if e != nil {
		return e
	}
	if !p.Has("product:manage") {
		return errors.New("product maintenance permission required")
	}
	proposed := gconv.Map(data)
	if len(proposed) == 0 {
		proposed = jsonMap(row["proposed"])
	}
	if strings.TrimSpace(gconv.String(proposed["sku"])) == "" || strings.TrimSpace(gconv.String(proposed["name"])) == "" {
		return errors.New("SKU and product name required")
	}
	productID, reason, e := applyProductRow(ctx, p, asset, row["sheet"].String(), row["source_row"].Int(), gconv.String(proposed["sku"]), gconv.String(proposed["name"]), proposed, true)
	if e != nil {
		return e
	}
	if reason != "" {
		return errors.New(reason)
	}
	_, e = g.DB().Model("bm_growth_import_rows").Ctx(ctx).Where("id", id).Data(g.Map{"state": "imported", "reason": "", "product_id": productID, "proposed": jsonText(proposed)}).Update()
	return e
}
func safeArchiveName(name string) bool {
	normalized := strings.ReplaceAll(name, "\\", "/")
	return normalized != "" && !strings.HasPrefix(normalized, "/") && !strings.Contains(normalized, ":") && !strings.ContainsRune(normalized, 0) && path.Clean(normalized) == normalized && !strings.HasPrefix(normalized, "../") && normalized != ".."
}
func ingestArchive(ctx context.Context, p *governance.Principal, asset gdb.Record, office bool) (string, error) {
	file, e := BlobPath(asset)
	if e != nil {
		return "", e
	}
	reader, e := zip.OpenReader(file)
	if e != nil {
		return "", errors.New("invalid ZIP container")
	}
	defer reader.Close()
	if len(reader.File) > 5000 {
		return "", errors.New("archive exceeds 5000 entries")
	}
	var total uint64
	var text strings.Builder
	for _, entry := range reader.File {
		if entry.FileInfo().IsDir() {
			continue
		}
		if !safeArchiveName(entry.Name) || entry.Mode()&os.ModeSymlink != 0 {
			return "", errors.New("unsafe archive entry")
		}
		total += entry.UncompressedSize64
		if total > 512*1024*1024 || entry.UncompressedSize64 > 32*1024*1024 {
			return "", errors.New("archive expansion limit reached")
		}
		extension := strings.ToLower(filepath.Ext(entry.Name))
		isImage := extension == ".png" || extension == ".jpg" || extension == ".jpeg" || extension == ".webp"
		if !office && !isImage {
			continue
		}
		if office && !isImage && extension != ".xml" {
			continue
		}
		r, e := entry.Open()
		if e != nil {
			return "", e
		}
		raw, e := io.ReadAll(io.LimitReader(r, 32*1024*1024+1))
		r.Close()
		if e != nil || len(raw) > 32*1024*1024 {
			return "", errors.New("invalid archive data")
		}
		if isImage {
			child, e := saveBlob(ctx, p, asset["company_id"].Int64(), asset["filename"].String()+"/"+entry.Name, asset["id"].String(), "products", bytes.NewReader(raw), asset["ai_allowed"].Bool())
			if e != nil {
				return "", e
			}
			if e = inspectImage(ctx, p, child); e != nil {
				return "", e
			}
			if child["ai_allowed"].Bool() {
				if _, e = Queue(ctx, p, "ingest", child["id"].String(), "ingest-child:"+child["id"].String(), g.Map{}); e != nil {
					return "", e
				}
			}
		}
		if office && extension == ".xml" && (strings.HasPrefix(entry.Name, "word/") || strings.HasPrefix(entry.Name, "ppt/slides/")) {
			decoder := xml.NewDecoder(bytes.NewReader(raw))
			for {
				token, e := decoder.Token()
				if e == io.EOF {
					break
				}
				if e != nil {
					return "", errors.New("invalid office document XML")
				}
				if chars, ok := token.(xml.CharData); ok {
					text.Write(chars)
					text.WriteByte(' ')
				}
				if text.Len() > 2*1024*1024 {
					return "", errors.New("document text limit reached")
				}
			}
		}
	}
	if e = matchImages(ctx, p, asset["company_id"].Int64()); e != nil {
		return "", e
	}
	return text.String(), nil
}
func inspectImage(ctx context.Context, p *governance.Principal, asset gdb.Record) error {
	file, e := BlobPath(asset)
	if e != nil {
		return e
	}
	f, e := os.Open(file)
	if e != nil {
		return e
	}
	cfg, _, e := image.DecodeConfig(f)
	f.Close()
	if e != nil || cfg.Width < 1 || cfg.Height < 1 || int64(cfg.Width)*int64(cfg.Height) > 40000000 {
		return errors.New("invalid image or image exceeds 40 megapixels")
	}
	extracted := jsonMap(asset["extracted"])
	extracted["width"], extracted["height"] = cfg.Width, cfg.Height
	_, e = g.DB().Model("bm_growth_assets").Ctx(ctx).Where("id", asset["id"]).Data(g.Map{"status": "ready", "category": "product_image", "extracted": jsonText(extracted)}).Update()
	return e
}
func imageMatchesSKU(name, sku string) bool {
	name = strings.ToUpper(strings.ReplaceAll(name, "\\", "/"))
	sku = strings.ToUpper(sku)
	for _, component := range strings.Split(name, "/") {
		if component == sku {
			return true
		}
	}
	expression := regexp.MustCompile(`(^|[^A-Z0-9])` + regexp.QuoteMeta(sku) + `([^A-Z0-9]|$)`)
	return expression.MatchString(strings.TrimSuffix(name, filepath.Ext(name)))
}
func matchImages(ctx context.Context, p *governance.Principal, companyID int64) error {
	products, e := g.DB().Model("bm_growth_products").Ctx(ctx).Where("company_id", companyID).WhereNull("deleted_at").Fields("id,sku,fields").All()
	if e != nil {
		return e
	}
	images, e := g.DB().Model("bm_growth_assets").Ctx(ctx).Where("company_id", companyID).Where("mime LIKE ?", "image/%").WhereNull("deleted_at").All()
	if e != nil {
		return e
	}
	skuIndex, pathIndex := map[string][]int64{}, map[string][]int64{}
	for _, product := range products {
		skuIndex[strings.ToUpper(product["sku"].String())] = append(skuIndex[strings.ToUpper(product["sku"].String())], product["id"].Int64())
		imagePath := gconv.String(jsonMap(product["fields"])["image"])
		if imagePath != "" {
			key := strings.ToUpper(strings.ReplaceAll(imagePath, "\\", "/"))
			pathIndex[key] = append(pathIndex[key], product["id"].Int64())
		}
	}
	for _, image := range images {
		ids, explicit := indexedImageMatches(image["filename"].String(), skuIndex, pathIndex)
		if len(ids) != 1 {
			continue
		}
		_, e = g.DB().Model("bm_growth_product_assets").Ctx(ctx).Data(g.Map{"product_id": ids[0], "asset_id": image["id"], "role": "side", "confirmed": true, "evidence": map[bool]string{true: "table_image_path", false: "sku_filename"}[explicit]}).InsertIgnore()
		if e != nil {
			return e
		}
	}
	return nil
}

// Index-based association avoids compiling a regular expression for every
// image/product pair in large product libraries.
func indexedImageMatches(name string, skus, paths map[string][]int64) ([]int64, bool) {
	name = strings.ToUpper(strings.ReplaceAll(name, "\\", "/"))
	matched := map[int64]bool{}
	parts := strings.Split(name, "/")
	explicit := false
	for i := range parts {
		for _, id := range paths[strings.Join(parts[i:], "/")] {
			matched[id] = true
			explicit = true
		}
	}
	if !explicit {
		for _, part := range parts {
			if strings.EqualFold(filepath.Ext(part), ".zip") {
				continue
			}
			part = strings.TrimSuffix(part, filepath.Ext(part))
			starts, ends := []int{0}, []int{len(part)}
			for i, r := range part {
				if !unicode.IsLetter(r) && !unicode.IsDigit(r) {
					starts = append(starts, i+len(string(r)))
					ends = append(ends, i)
				}
			}
			for _, start := range starts {
				for _, end := range ends {
					if end > start && end-start <= 512 {
						for _, id := range skus[part[start:end]] {
							matched[id] = true
						}
					}
				}
			}
		}
	}
	ids := []int64{}
	for id := range matched {
		ids = append(ids, id)
	}
	return ids, explicit
}
func recognizeDocument(ctx context.Context, p *governance.Principal, asset gdb.Record, text string, fileData []byte, imageData []byte) error {
	latest, err := Asset(ctx, p, asset["id"].String(), "material:recognize")
	if err != nil {
		return err
	}
	asset["ai_allowed"] = latest["ai_allowed"]
	if !asset["ai_allowed"].Bool() {
		_, e := g.DB().Model("bm_growth_assets").Ctx(ctx).Where("id", asset["id"]).Data(g.Map{"content": cleanText(text, 2*1024*1024)}).Update()
		return e
	}
	var result struct {
		Category string `json:"category"`
		Summary  string `json:"summary"`
		Facts    []struct {
			Key      string `json:"key"`
			Value    string `json:"value"`
			Evidence string `json:"evidence"`
			Location string `json:"location"`
		} `json:"facts"`
		SKU []string `json:"sku_candidates"`
	}
	input := AIInput{Instruction: "Classify company/supplier/customer documents and images. Categories: company_profile, brand, catalog, certificate, sales_policy, technical, faq, quotation, internal, product_image. Extract only evidence-backed facts. Bank/payment/cost/contracts are internal. A photo is not proof of fitment/voltage. Keep SKU guesses as candidates. Do not change company records.", Data: g.Map{"filename": asset["filename"].String(), "text": text}, Schema: schema(g.Map{"category": strSchema(), "summary": strSchema(), "facts": g.Map{"type": "array", "items": schema(g.Map{"key": strSchema(), "value": strSchema(), "evidence": strSchema(), "location": strSchema()})}, "sku_candidates": stringsSchema()})}
	input.Image, input.ImageMIME = imageData, asset["mime"].String()
	input.File, input.Filename = fileData, filepath.Base(asset["filename"].String())
	if e := IntelligenceProvider.Extract(ctx, p, "material_recognition", input, &result); e != nil {
		return e
	}
	allowed := map[string]bool{"company_profile": true, "brand": true, "catalog": true, "certificate": true, "sales_policy": true, "technical": true, "faq": true, "quotation": true, "internal": true, "product_image": true}
	if !allowed[result.Category] {
		return errors.New("unrecognized material category")
	}
	_, e := g.DB().Model("bm_growth_assets").Ctx(ctx).Where("id", asset["id"]).Data(g.Map{"category": result.Category, "content": cleanText(text, 2*1024*1024), "extracted": jsonText(result)}).Update()
	return e
}
func ingest(ctx context.Context, p *governance.Principal, id string) error {
	asset, e := Asset(ctx, p, id, "material:recognize")
	if e != nil {
		return e
	}
	file, e := BlobPath(asset)
	if e != nil {
		return e
	}
	if _, e = g.DB().Model("bm_growth_assets").Ctx(ctx).Where("id", id).Data(g.Map{"status": "processing", "error": "", "published": false, "version": gdb.Raw("version+1")}).Update(); e != nil {
		return e
	}
	extension := strings.ToLower(filepath.Ext(asset["filename"].String()))
	var text string
	switch {
	case extension == ".xlsx" && asset["category"].String() == "product_sheet":
		e = ingestSheet(ctx, p, asset)
	case extension == ".zip":
		_, e = ingestArchive(ctx, p, asset, false)
	case extension == ".docx" || extension == ".pptx":
		text, e = ingestArchive(ctx, p, asset, true)
		if e == nil {
			e = recognizeDocument(ctx, p, asset, text, nil, nil)
		}
	case strings.HasPrefix(asset["mime"].String(), "image/"):
		e = inspectImage(ctx, p, asset)
		if e == nil && asset["ai_allowed"].Bool() {
			var data []byte
			data, e = os.ReadFile(file)
			if e == nil {
				e = recognizeDocument(ctx, p, asset, "", nil, data)
			}
		}
		if e == nil {
			e = matchImages(ctx, p, asset["company_id"].Int64())
		}
	case extension == ".pdf":
		var data []byte
		data, e = os.ReadFile(file)
		if e == nil {
			e = recognizeDocument(ctx, p, asset, "", data, nil)
		}
	case extension == ".xlsx":
		var book *excelize.File
		book, e = excelize.OpenFile(file)
		if e == nil {
			var all strings.Builder
			for _, sheet := range book.GetSheetList() {
				if sheet == templateInstructionsSheet {
					continue
				}
				var rows *excelize.Rows
				rows, e = book.Rows(sheet)
				if e != nil {
					break
				}
				for rows.Next() {
					values, err := rows.Columns()
					if err != nil {
						e = err
						break
					}
					all.WriteString(strings.Join(values, "\t") + "\n")
					if all.Len() > 1000000 {
						e = errors.New("split large company spreadsheets")
						break
					}
				}
				rows.Close()
				if e != nil {
					break
				}
			}
			book.Close()
			if e == nil {
				e = recognizeDocument(ctx, p, asset, all.String(), nil, nil)
			}
		}
	default:
		var data []byte
		data, e = os.ReadFile(file)
		if e == nil {
			e = recognizeDocument(ctx, p, asset, string(data), nil, nil)
		}
	}
	state, errorText := "ready", ""
	if e != nil {
		state, errorText = "failed", e.Error()
	}
	_, _ = g.DB().Model("bm_growth_assets").Where("id", id).Data(g.Map{"status": state, "error": errorText}).Update()
	if e == nil {
		latest, err := Asset(ctx, p, id, "material:recognize")
		if err == nil {
			delete(latest, "storage_key")
			e = version(ctx, p, asset["company_id"].Int64(), "asset", id, latest["version"].Int(), latest)
		}
	}
	return e
}
func ImportRows(ctx context.Context, p *governance.Principal, assetID string, page int) (g.Map, error) {
	if _, e := Asset(ctx, p, assetID, "material:list"); e != nil {
		return nil, e
	}
	q := g.DB().Model("bm_growth_import_rows").Ctx(ctx).Where("asset_id", assetID)
	n, e := q.Count()
	if e != nil {
		return nil, e
	}
	if page < 1 {
		page = 1
	}
	rows, e := q.Clone().Order("sheet,source_row").Page(page, 20).All()
	counts, e2 := q.Clone().Fields("state,count(*) AS count").Group("state").All()
	if e == nil {
		e = e2
	}
	return g.Map{"list": rows, "total": n, "counts": counts}, e
}
