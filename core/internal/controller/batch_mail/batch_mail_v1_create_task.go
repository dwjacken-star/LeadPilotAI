package batch_mail

import (
	"billionmail-core/internal/consts"
	"billionmail-core/internal/service/batch_mail"
	"billionmail-core/internal/service/public"
	"context"

	"github.com/gogf/gf/v2/errors/gerror"

	"billionmail-core/api/batch_mail/v1"
)

func (c *ControllerV1) CreateTask(ctx context.Context, req *v1.CreateTaskReq) (res *v1.CreateTaskRes, err error) {
	res = &v1.CreateTaskRes{}

	if err = validateCreateTaskRequest(req); err != nil {
		res.SetError(err)
		return
	}

	addType := 0
	// create task and import recipients
	res.Data.Id, err = batch_mail.CreateTaskWithRecipients(ctx, req, addType)
	if err != nil {
		res.SetError(err)
		return
	}

	_ = public.WriteLog(ctx, public.LogParams{
		Type: consts.LOGTYPE.Task,
		Log:  "Create task :" + req.Subject + " successfully",
		Data: req,
	})

	res.SetSuccess(public.LangCtx(ctx, "Task created successfully"))
	return
}

func validateCreateTaskRequest(req *v1.CreateTaskReq) error {

	if req.GroupId <= 0 && len(req.GroupIds) == 0 {
		return gerror.New("Must select a contact group")
	}

	if len(req.TagIds) > 0 {
		if req.TagLogic != "" && req.TagLogic != "AND" && req.TagLogic != "OR" && req.TagLogic != "NOT" {
			return gerror.New("Tag logic must be AND or OR or NOT")
		}

		if req.TagLogic == "" {
			req.TagLogic = "AND"
		}
	}

	return nil
}
