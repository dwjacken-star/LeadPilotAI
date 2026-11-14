package public

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"github.com/xuri/excelize/v2"
	"strings"
)

const SpreadsheetMIME = "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet"

func ReadSpreadsheet(encoded string) ([][]string, error) {
	if strings.HasPrefix(encoded, "data:") {
		parts := strings.SplitN(encoded, ",", 2)
		if len(parts) != 2 {
			return nil, fmt.Errorf("Invalid Excel file")
		}
		encoded = parts[1]
	}
	if len(encoded) > 7*1024*1024 {
		return nil, fmt.Errorf("Excel file exceeds 5 MB")
	}
	raw, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil || len(raw) > 5*1024*1024 {
		return nil, fmt.Errorf("Invalid Excel file; upload an .xlsx workbook")
	}
	book, err := excelize.OpenReader(bytes.NewReader(raw), excelize.Options{UnzipSizeLimit: 32 * 1024 * 1024})
	if err != nil {
		return nil, fmt.Errorf("Invalid Excel workbook")
	}
	defer book.Close()
	sheets := book.GetSheetList()
	if len(sheets) == 0 {
		return nil, fmt.Errorf("Excel workbook has no worksheet")
	}
	rows, err := book.Rows(sheets[0])
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result [][]string
	for rows.Next() {
		if len(result) > 10000 {
			return nil, fmt.Errorf("Maximum 10000 rows per import")
		}
		row, e := rows.Columns()
		if e != nil {
			return nil, e
		}
		result = append(result, row)
	}
	if err := rows.Error(); err != nil {
		return nil, err
	}
	if len(result) < 2 {
		return nil, fmt.Errorf("Excel workbook must include headers and data")
	}
	return result, nil
}

func BuildImportWorkbook(mailbox bool, language string) ([]byte, error) {
	book := excelize.NewFile()
	defer book.Close()
	sheet := book.GetSheetName(0)
	headers := []interface{}{"email", "name", "company", "country", "attributes"}
	example := []interface{}{"buyer@example.com", "Alex", "Example Trading", "US", `{"source":"trade fair"}`}
	hint := "Replace the example with your contacts. email is required; attributes is an optional JSON object. Other columns become contact attributes."
	if mailbox {
		headers = []interface{}{"username", "full_name", "password", "quota_mb", "active"}
		example = []interface{}{"sales@example.com", "Sales", "", 5120, 1}
		hint = "Replace the example with a mailbox in an existing domain. Leave password blank to use the default. quota_mb is in MB; active: 1 enabled, 0 disabled. Existing mailboxes are skipped."
	}
	if language == "zh" {
		hint = "请用实际联系人替换示例。email 为必填邮箱，attributes 为选填 JSON 对象，其他列会保存为联系人属性。"
		if mailbox {
			hint = "请用系统已添加的域名替换示例。password 留空使用默认密码，quota_mb 单位为 MB，active：1 启用、0 停用。已存在邮箱会跳过，不修改原密码。"
		}
	}
	if language == "ja" {
		hint = "例を連絡先に置き換えてください。email は必須、attributes は任意の JSON オブジェクトです。他の列は属性として保存されます。"
		if mailbox {
			hint = "登録済みドメインのメールボックスを入力してください。パスワード空欄は既定値、quota_mb は MB、active は 1 有効・0 無効です。既存メールボックスは変更しません。"
		}
	}
	if err := book.SetSheetRow(sheet, "A1", &headers); err != nil {
		return nil, err
	}
	_ = book.SetSheetRow(sheet, "A2", &example)
	_ = book.SetColWidth(sheet, "A", "E", 26)
	_ = book.SetColWidth(sheet, "A", "A", 32)
	style, _ := book.NewStyle(&excelize.Style{Font: &excelize.Font{Bold: true, Color: "FFFFFF"}, Fill: excelize.Fill{Type: "pattern", Color: []string{"176F65"}, Pattern: 1}})
	_ = book.SetCellStyle(sheet, "A1", "E1", style)
	_, _ = book.NewSheet("Instructions")
	_ = book.SetCellValue("Instructions", "A1", hint)
	_ = book.SetColWidth("Instructions", "A", "A", 100)
	data, err := book.WriteToBuffer()
	if err != nil {
		return nil, err
	}
	return data.Bytes(), nil
}
