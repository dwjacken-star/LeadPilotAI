package email_template

import (
	v1 "billionmail-core/api/email_template/v1"
	templates "billionmail-core/internal/service/email_template"
	"context"
)

func (c *ControllerV1) Options(ctx context.Context, req *v1.OptionsReq) (res *v1.OptionsRes, err error) {
	res = &v1.OptionsRes{}
	res.Data, err = templates.CreationOptions(ctx)
	if err != nil {
		return nil, err
	}
	res.SetSuccess("Success")
	return
}
