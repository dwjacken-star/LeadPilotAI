package mail_boxes

import (
	"billionmail-core/internal/service/public"
	"encoding/csv"
	"fmt"
	"strconv"
	"strings"
)

func mailboxWorkbookCSV(data string, exclude ...func(string) bool) (string, error) {
	rows, err := public.ReadSpreadsheet(data)
	if err != nil {
		return "", err
	}
	headers := rows[0]
	found := false
	usernameColumn := -1
	quotaColumn := -1
	for i, col := range headers {
		headers[i] = strings.ToLower(strings.TrimSpace(col))
		if headers[i] == "username" {
			found = true
			usernameColumn = i
		}
		if headers[i] == "quota_mb" {
			headers[i] = "quota"
			quotaColumn = i
		}
	}
	if !found {
		return "", fmt.Errorf("Required column: username")
	}
	var out strings.Builder
	writer := csv.NewWriter(&out)
	_ = writer.Write(headers)
	for i, row := range rows[1:] {
		if strings.TrimSpace(strings.Join(row, "")) == "" {
			continue
		}
		values := make([]string, len(headers))
		copy(values, row)
		if len(exclude) > 0 && exclude[0](values[usernameColumn]) {
			continue
		}
		if quotaColumn >= 0 && values[quotaColumn] != "" {
			quota, e := strconv.ParseInt(values[quotaColumn], 10, 64)
			if e != nil || quota < 1 || quota > 1048576 {
				return "", fmt.Errorf("Row %d: quota_mb must be between 1 and 1048576", i+2)
			}
			values[quotaColumn] = strconv.FormatInt(quota*1024, 10)
		}
		_ = writer.Write(values)
	}
	writer.Flush()
	return out.String(), writer.Error()
}
