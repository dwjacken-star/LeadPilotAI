package batch_mail

import (
	"billionmail-core/internal/consts"
	"billionmail-core/internal/service/batch_mail"
	"billionmail-core/internal/service/governance"
	"billionmail-core/internal/service/public"
	"billionmail-core/internal/service/rbac"
	"context"
	"encoding/json"
	"strings"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"

	"billionmail-core/api/batch_mail/v1"
)

func (c *ControllerV1) UpdateTaskInfo(ctx context.Context, req *v1.UpdateTaskInfoReq) (res *v1.UpdateTaskInfoRes, err error) {

	res = &v1.UpdateTaskInfoRes{}
	principal, scopeErr := governance.RequestPrincipal(ctx)
	if scopeErr != nil {
		res.SetError(scopeErr)
		return res, nil
	}

	var task struct {
		Id          int `json:"id"`
		TaskProcess int `json:"task_process"`
		Pause       int `json:"pause"`
	}

	err = g.DB().Model("email_tasks").
		Fields("id, task_process, pause").
		Where("id", req.TaskId).
		Scan(&task)

	if err != nil {
		res.SetError(gerror.New(public.LangCtx(ctx, "Query task failed: {}", err.Error())))
		return nil, err
	}

	if task.Id == 0 {
		res.SetError(gerror.New(public.LangCtx(ctx, "The task does not exist.")))
		return
	}

	executor := batch_mail.GetTaskExecutor(req.TaskId)
	if executor != nil && executor.IsRunning() && task.Pause == 0 {
		res.SetError(gerror.New(public.LangCtx(ctx, "The task is currently running. Please pause the task first.")))
		return
	}
	if task.TaskProcess == 1 && task.Pause == 0 {
		res.SetError(gerror.New("Pause the task before editing"))
		return res, nil
	}
	if err = batch_mail.ValidCC(req.CCEmail); err != nil {
		res.SetError(err)
		return res, nil
	}
	if strings.TrimSpace(req.Subject) == "" || strings.ContainsAny(req.Subject+req.FullName, "\r\n") {
		res.SetError(gerror.New("invalid email headers"))
		return res, nil
	}
	if e := batch_mail.ValidateAuthoring(req.AuthoringConfig); e != nil {
		res.SetError(e)
		return res, nil
	}
	updateData := map[string]interface{}{"cc_email": req.CCEmail, "track_open": req.TrackOpen, "track_click": req.TrackClick}
	if req.AuthoringConfig != nil {
		metadata, _ := json.Marshal(req.AuthoringConfig)
		updateData["authoring_config"] = string(metadata)
	}
	if req.CCEmail != "" {
		updateData["track_open"] = 0
		updateData["track_click"] = 0
	}
	// 0 or 1 fields
	if req.Unsubscribe == 0 || req.Unsubscribe == 1 {
		updateData["unsubscribe"] = req.Unsubscribe
	}

	if req.Addresser != "" {
		updateData["addresser"] = req.Addresser
	}
	if req.Subject != "" {
		updateData["subject"] = req.Subject
	}
	if req.FullName != "" {
		updateData["full_name"] = req.FullName
	}

	if req.Remark != "" {
		updateData["remark"] = req.Remark
	}
	if req.TemplateId > 0 {
		updateData["template_id"] = req.TemplateId
	}
	if req.Threads > 0 {
		updateData["threads"] = req.Threads
	}
	if req.StartTime > 0 {
		updateData["start_time"] = req.StartTime
	}

	if len(req.TagIds) > 0 {
		updateData["tag_ids"] = req.TagIds
	}
	if req.TagLogic == "AND" || req.TagLogic == "OR" || req.TagLogic == "NOT" {
		updateData["tag_logic"] = req.TagLogic
	}
	if len(updateData) == 0 {
		res.SetError(gerror.New(public.LangCtx(ctx, "No valid update fields")))
		return
	}

	err = g.DB().Transaction(ctx, func(ctx context.Context, tx gdb.TX) error {
		current, e := tx.GetOne(`SELECT task_process,pause,dept_id,owner_id,group_id,group_ids FROM email_tasks WHERE id=$1 FOR UPDATE`, req.TaskId)
		if e != nil {
			return e
		}
		if len(current) == 0 {
			return gerror.New("Task not found")
		}
		if principal != nil && !principal.Super {
			if e = principal.CheckOwnership(ctx, "campaign:update", current["dept_id"].Int64(), current["owner_id"].Int64()); e != nil {
				return e
			}
			if current["task_process"].Int() == 2 || current["task_process"].Int() == 4 {
				return gerror.New("Completed or cancelled tasks cannot be edited")
			}
			if req.Addresser != "" || len(req.Senders) > 0 {
				pool, err := batch_mail.NormalizeSenders(req.Senders, req.Addresser)
				if err != nil {
					return err
				}
				if err = batch_mail.CheckSenderPool(ctx, principal, pool); err != nil {
					return err
				}
				encoded, _ := json.Marshal(pool)
				updateData["senders"] = string(encoded)
				updateData["addresser"] = pool[0]
			}
			var groupIDs []int64
			_ = json.Unmarshal(current["group_ids"].Bytes(), &groupIDs)
			groupIDs = batch_mail.NormalizeGroups(groupIDs, current["group_id"].Int64())
			for _, tagID := range req.TagIds {
				if e = principal.Check(ctx, "bm_tags", int64(tagID), "contact:list"); e != nil {
					return e
				}
				tag, err := tx.Model("bm_tags").Where("id", tagID).WhereIn("group_id", groupIDs).Count()
				if err != nil {
					return err
				}
				if tag == 0 {
					return gerror.New("Tag is outside the task's recipient groups")
				}
			}
		}
		if current["task_process"].Int() == 1 && current["pause"].Int() == 0 {
			return gerror.New("Pause the task before editing")
		}
		if req.HTMLContent != nil || req.TemplateId > 0 {
			p := principal
			if p == nil {
				p, e = governance.Load(ctx, rbac.GetCurrentAccountId(ctx))
				if e != nil {
					return e
				}
			}
			content, e := batch_mail.ResolveCampaignContent(ctx, p, int64(req.TemplateId), req.HTMLContent)
			if e != nil {
				return e
			}
			updateData["snapshot_content"] = content
			updateData["template_id"] = req.TemplateId
		}
		if _, e := tx.Model("email_tasks").Where("id", req.TaskId).Data(updateData).Update(); e != nil {
			return e
		}
		return batch_mail.SetTaskWarmup(ctx, int64(req.TaskId), req.Warmup)
	})

	if err != nil {

		res.SetError(gerror.New(public.LangCtx(ctx, "Failed to update task information : {}", err.Error())))
		return nil, err
	}

	_ = public.WriteLog(ctx, public.LogParams{
		Type: consts.LOGTYPE.Task,
		Log:  "Update Task Info :" + req.Subject + " successfully",
		Data: updateData,
	})

	res.SetSuccess(public.LangCtx(ctx, "Task update successful"))
	return
}
