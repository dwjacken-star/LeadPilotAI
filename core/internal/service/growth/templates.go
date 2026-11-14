package growth

import (
	"archive/zip"
	"bytes"
	"context"
	"errors"
	"strings"

	"billionmail-core/internal/service/governance"
	"billionmail-core/internal/service/public"
	"github.com/xuri/excelize/v2"
)

const templateInstructionsSheet = "_MapleMailInstructions"

// Download templates use import/upload permissions, not product export rights.
// The selected company's ownership is checked before returning even a blank file.
func UploadTemplate(ctx context.Context, p *governance.Principal, companyID int64, kind, language string) ([]byte, string, string, error) {
	if _, e := company(ctx, p, companyID, "material:upload", false); e != nil {
		return nil, "", "", e
	}
	if kind == "products" {
		if e := require(p, "product:import"); e != nil {
			return nil, "", "", e
		}
	}
	local := func(zh, en, ja string) string {
		if strings.HasPrefix(language, "zh") {
			return zh
		}
		if strings.HasPrefix(language, "ja") {
			return ja
		}
		return en
	}
	if kind == "images" {
		var out bytes.Buffer
		archive := zip.NewWriter(&out)
		if _, e := archive.Create("images/"); e != nil {
			return nil, "", "", e
		}
		file, e := archive.Create("README.txt")
		if e != nil {
			return nil, "", "", e
		}
		guide := local("图片压缩包填写说明\n1. 将真实产品图片放入 images 文件夹。\n2. 文件名使用产品 SKU，例如 000001-main.jpg、000001-side.jpg；SKU 必须与产品表一致。\n3. 也可在产品表 Image 列填写 images/000001-main.jpg 进行精确匹配。\n4. 支持 JPG、PNG、WEBP。不要将无关或敏感文件放入压缩包。\n5. 删除本说明文件后压缩为 ZIP 并上传；此模板不包含虚构产品图片。", "Product image ZIP instructions\n1. Place real product images in images/.\n2. Use exact product SKUs in filenames, e.g. 000001-main.jpg, 000001-side.jpg.\n3. Alternatively use Image=images/000001-main.jpg in the product workbook for exact matching.\n4. JPG, PNG and WEBP are supported. Exclude unrelated or sensitive files.\n5. Remove README.txt, ZIP your images and upload. No fabricated images are included.", "画像 ZIP の使用方法\n1. 実際の製品画像を images/ に入れてください。\n2. ファイル名は製品 SKU と一致させます（例: 000001-main.jpg）。\n3. 製品表の Image 列に images/000001-main.jpg と入力して照合できます。\n4. JPG、PNG、WEBP に対応します。\n5. README.txt を削除し、画像を ZIP に圧縮してアップロードしてください。")
		if _, e = file.Write([]byte(guide)); e != nil {
			return nil, "", "", e
		}
		if e = archive.Close(); e != nil {
			return nil, "", "", e
		}
		return out.Bytes(), "application/zip", "MapleMail-image-upload-template.zip", nil
	}
	if kind != "products" && kind != "company" {
		return nil, "", "", errors.New("unsupported upload template")
	}
	book := excelize.NewFile()
	defer book.Close()
	headerStyle, e := book.NewStyle(&excelize.Style{Font: &excelize.Font{Bold: true, Color: "FFFFFF"}, Fill: excelize.Fill{Type: "pattern", Color: []string{"176249"}, Pattern: 1}})
	if e != nil {
		return nil, "", "", e
	}
	textStyle, e := book.NewStyle(&excelize.Style{NumFmt: 49})
	if e != nil {
		return nil, "", "", e
	}
	var headers []interface{}
	var notes [][]interface{}
	if kind == "products" {
		headers = []interface{}{"SKU", "Name", "Category", "OE", "Fitment", "Voltage", "Refrigerant", "MOQ", "Image", "Currency", "Price", "Description"}
		notes = [][]interface{}{
			{local("字段", "Field", "項目"), local("说明", "Instructions", "説明"), local("示例（不自动导入）", "Example (not imported)", "例（取込対象外）")},
			{"SKU", local("必填；以文本保存，保留前导零", "Required; text preserves leading zeros", "必須。文字列として先頭ゼロを保持"), "000001"},
			{"Name", local("必填；产品名称", "Required product name", "必須の製品名"), "Automotive A/C Compressor"},
			{"Category", local("产品类别，可选", "Optional product category", "製品分類（任意）"), "A/C Parts"},
			{"OE / Fitment", local("原厂编号、适配车型；只填写确认的资料", "OE codes and verified vehicle fitment only", "OE 番号と確認済み適合車種"), "0012345678"},
			{"Voltage / Refrigerant / MOQ", local("电压、冷媒、起订量，可选", "Optional voltage, refrigerant and minimum order quantity", "電圧・冷媒・最小注文数量（任意）"), "12V / R134a / 10"},
			{"Image", local("图片包内相对路径；也可按 SKU 文件名自动匹配", "Relative path in ZIP; SKU filenames can also be matched", "ZIP 内の相対パス。SKU 名でも照合可能"), "images/000001-main.jpg"},
			{"Currency / Price", local("币种和销售价，可选；勿填写内部采购成本", "Optional sales currency and price; exclude internal cost", "販売通貨と価格（任意）。社内原価は記載しない"), "USD / 100.00"},
			{"Description", local("已确认的产品说明；可增加其他业务字段列", "Verified description; additional columns are supported", "確認済みの製品説明。列追加も可能"), ""},
			{local("使用方法", "How to use", "使用方法"), local("在 Sheet1 从第2行填写真实产品，再上传到产品库。说明页自动跳过；空模板不会创建示例产品。", "Enter real products from row 2 of Sheet1 and upload in Products. This instructions sheet is skipped; examples are never imported.", "Sheet1 の2行目から実際の製品を入力し製品一覧からアップロード。説明シートの例は取り込みません。"), ""},
		}
	} else {
		headers = []interface{}{local("资料项目", "Company field", "会社情報項目"), local("实际内容（请填写）", "Actual content (fill in)", "実際の内容（入力）"), local("来源或备注", "Source / notes", "出典・備考")}
		fields := [][3]string{{"公司名称", "Legal company name", "会社正式名"}, {"品牌", "Brand", "ブランド"}, {"公司介绍", "Company introduction", "会社紹介"}, {"官方网站", "Official website", "公式サイト"}, {"主营产品", "Main products", "主要製品"}, {"目标市场", "Target markets", "対象市場"}, {"地址", "Address", "住所"}, {"联系人", "Contact person", "担当者"}, {"邮箱", "Contact email", "連絡先メール"}, {"电话", "Telephone", "電話番号"}, {"认证及证书", "Verified certifications", "確認済み認証"}, {"质保与售后", "Warranty / after-sales", "保証・アフターサービス"}, {"最低订量及交期", "MOQ / lead time", "最小注文数量・納期"}, {"服务与优势", "Services and capabilities", "サービスと強み"}}
		if e = book.SetSheetRow("Sheet1", "A1", &headers); e != nil {
			return nil, "", "", e
		}
		for i, f := range fields {
			row := []interface{}{local(f[0], f[1], f[2]), "", ""}
			cell, _ := excelize.CoordinatesToCellName(1, i+2)
			if e = book.SetSheetRow("Sheet1", cell, &row); e != nil {
				return nil, "", "", e
			}
		}
		notes = [][]interface{}{{local("使用说明", "Instructions", "使用方法")}, {local("在实际内容列填写真实公司信息，再上传到资料库，并勾选允许 AI 识别。请删除不适用的项目；不要上传银行、密码等敏感内容。识别后可人工确认入库及发布。", "Fill in actual company facts, remove irrelevant rows, then upload in Materials with AI recognition enabled. Exclude bank details, passwords and other secrets. Review extracted facts before applying or publishing.", "実際の会社情報を入力し、不要な項目を削除して資料一覧から AI 認識を有効にしてアップロード。銀行情報・パスワード等は記載しないでください。認識結果を確認してから登録・公開します。")}}
	}
	if e = book.SetSheetRow("Sheet1", "A1", &headers); e != nil {
		return nil, "", "", e
	}
	end, _ := excelize.ColumnNumberToName(len(headers))
	if e = book.SetColStyle("Sheet1", "A:"+end, textStyle); e != nil {
		return nil, "", "", e
	}
	if e = book.SetCellStyle("Sheet1", "A1", end+"1", headerStyle); e != nil {
		return nil, "", "", e
	}
	if e = book.SetColWidth("Sheet1", "A", end, 24); e != nil {
		return nil, "", "", e
	}
	if _, e = book.NewSheet(templateInstructionsSheet); e != nil {
		return nil, "", "", e
	}
	for i, row := range notes {
		cell, _ := excelize.CoordinatesToCellName(1, i+1)
		if e = book.SetSheetRow(templateInstructionsSheet, cell, &row); e != nil {
			return nil, "", "", e
		}
	}
	book.SetColWidth(templateInstructionsSheet, "A", "A", 28)
	book.SetColWidth(templateInstructionsSheet, "B", "B", 100)
	book.SetColWidth(templateInstructionsSheet, "C", "C", 40)
	book.SetActiveSheet(0)
	body, e := book.WriteToBuffer()
	if e != nil {
		return nil, "", "", e
	}
	return body.Bytes(), public.SpreadsheetMIME, "MapleMail-" + kind + "-upload-template.xlsx", nil
}
