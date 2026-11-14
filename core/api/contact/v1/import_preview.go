package v1

import (
	"billionmail-core/utility/types/api_v1"
	"github.com/gogf/gf/v2/frame/g"
)

type PreviewImportReq struct {
	g.Meta           `path:"/contact/group/preview_import" method:"post" tags:"Contact" summary:"Preview Excel contacts without writing data"`
	FileData         string   `json:"file_data"`
	JobID            string   `json:"job_id"`
	Async            bool     `json:"async"`
	RetryUnconfirmed bool     `json:"retry_unconfirmed"`
	ValidationSender string   `json:"validation_sender"`
	FilterKeywords   []string `json:"filter_keywords"`
	Page             int      `json:"page"`
	Status           string   `json:"status" v:"in:all,valid,pending,queued,checking,unconfirmed,filtered,invalid,duplicate"`
	DownloadInvalid  bool     `json:"download_invalid"`
	DownloadPending  bool     `json:"download_pending"`
}
type PreviewImportRes struct{ api_v1.StandardRes }
