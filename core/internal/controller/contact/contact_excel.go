package contact

import (
	"billionmail-core/internal/model/entity"
	"billionmail-core/internal/service/public"
	"context"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"github.com/gogf/gf/v2/util/gvalid"
	"strings"
)

func parseContactWorkbook(ctx context.Context, data string, exclude ...func(string) bool) ([]*entity.Contact, error) {
	rows, err := public.ReadSpreadsheet(data)
	if err != nil {
		return nil, err
	}
	headers := rows[0]
	emailColumn := -1
	for i, col := range headers {
		headers[i] = strings.ToLower(strings.TrimSpace(col))
		if headers[i] == "email" {
			emailColumn = i
		}
	}
	if emailColumn < 0 {
		return nil, fmt.Errorf("Required column: email")
	}
	var out strings.Builder
	writer := csv.NewWriter(&out)
	_ = writer.Write([]string{"email", "active", "attributes"})
	for i, row := range rows[1:] {
		if strings.TrimSpace(strings.Join(row, "")) == "" {
			continue
		}
		get := func(c int) string {
			if c >= len(row) {
				return ""
			}
			return strings.TrimSpace(row[c])
		}
		email := get(emailColumn)
		if len(exclude) > 0 && exclude[0](email) {
			continue
		}
		if gvalid.New().Rules("email").Data(email).Run(ctx) != nil {
			return nil, fmt.Errorf("Row %d: invalid email", i+2)
		}
		active := "1"
		attributes := map[string]string{}
		for c, key := range headers {
			value := get(c)
			if value == "" || key == "email" || key == "" {
				continue
			}
			switch key {
			case "active":
				if value != "0" && value != "1" {
					return nil, fmt.Errorf("Row %d: active must be 0 or 1", i+2)
				}
				active = value
			case "attributes":
				attrs, e := parseJSONAttributes(ctx, value)
				if e != nil {
					return nil, fmt.Errorf("Row %d: attributes must be a JSON object", i+2)
				}
				for k, v := range attrs {
					attributes[k] = v
				}
			default:
				attributes[key] = value
			}
		}
		encoded, _ := json.Marshal(attributes)
		_ = writer.Write([]string{email, active, string(encoded)})
	}
	writer.Flush()
	return parseCSVContent(ctx, out.String())
}
