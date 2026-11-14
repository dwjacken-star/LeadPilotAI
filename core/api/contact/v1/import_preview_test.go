package v1

import (
	"context"
	"testing"

	"github.com/gogf/gf/v2/util/gvalid"
)

func TestPreviewImportStatusValidation(t *testing.T) {
	for _, status := range []string{"", "all", "valid", "pending", "filtered", "invalid", "duplicate"} {
		t.Run(status, func(t *testing.T) {
			err := gvalid.New().Data(PreviewImportReq{FileData: "workbook", Status: status}).Run(context.Background())
			if err != nil {
				t.Fatalf("preview filter %q rejected by request validation: %v", status, err)
			}
		})
	}
	if err := gvalid.New().Data(PreviewImportReq{FileData: "workbook", Status: "unsupported"}).Run(context.Background()); err == nil {
		t.Fatal("unsupported preview status accepted")
	}
}
