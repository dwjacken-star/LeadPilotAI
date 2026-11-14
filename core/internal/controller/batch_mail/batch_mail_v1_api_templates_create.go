package batch_mail

import (
	"billionmail-core/api/batch_mail/v1"
	"billionmail-core/internal/consts"
	batchservice "billionmail-core/internal/service/batch_mail"
	"billionmail-core/internal/service/governance"
	"billionmail-core/internal/service/public"
	"billionmail-core/internal/service/rbac"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"net"
	"strings"
	"time"

	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
)

func generateApiKey() (string, error) {
	bytes := make([]byte, 32)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	return hex.EncodeToString(bytes), nil
}

func (c *ControllerV1) ApiTemplatesCreate(ctx context.Context, req *v1.ApiTemplatesCreateReq) (res *v1.ApiTemplatesCreateRes, err error) {
	res = &v1.ApiTemplatesCreateRes{}
	pool, err := batchservice.NormalizeSenders(req.Senders, req.Addresser)
	if err != nil {
		return nil, err
	}
	if err = batchservice.ValidCC(req.CCEmail); err != nil {
		return nil, err
	}
	principal, err := governance.Load(ctx, rbac.GetCurrentAccountId(ctx))
	if err != nil {
		return nil, err
	}
	if err = batchservice.CheckSenderPool(ctx, principal, pool); err != nil {
		return nil, err
	}
	if req.CCEmail != "" {
		req.TrackOpen = 0
		req.TrackClick = 0
	}
	req.Addresser = pool[0]
	encodedPool, _ := json.Marshal(pool)

	// check if template exists
	count, err := g.DB().Model("email_templates").Where("id", req.TemplateId).Count()
	if err != nil {
		return nil, err
	}
	if count == 0 {
		return nil, gerror.New(public.LangCtx(ctx, "Email template does not exist"))
	}

	// generate API key
	apiKey, err := generateApiKey()
	if err != nil {
		return nil, err
	}

	tx, err := g.DB().Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer func() {
		if err != nil {
			tx.Rollback()
		}
	}()

	// create API template
	result, err := tx.Model("api_templates").Insert(g.Map{
		"api_key":     apiKey,
		"api_name":    req.ApiName,
		"template_id": req.TemplateId,
		"group_id":    req.GroupId,
		"subject":     req.Subject,
		"addresser":   req.Addresser,
		"dept_id":     principal.DeptID, "owner_id": principal.ID,
		"senders": string(encodedPool), "cc_email": req.CCEmail, "sender_cursor": 0,
		"full_name":            req.FullName,
		"unsubscribe":          req.Unsubscribe,
		"track_open":           req.TrackOpen,
		"track_click":          req.TrackClick,
		"active":               req.Active,
		"expire_time":          0,
		"last_key_update_time": time.Now().Unix(),
	})

	if err != nil {
		return nil, err
	}

	apiId, err := result.LastInsertId()
	if err != nil {
		return nil, err
	}

	if len(req.IpWhitelist) > 0 {
		now := time.Now().Unix()
		for _, ip := range req.IpWhitelist {
			ip = strings.TrimSpace(ip)
			if ip == "" {
				continue
			}

			if net.ParseIP(ip) == nil {
				continue
			}

			_, err = tx.Model("api_ip_whitelist").Insert(g.Map{
				"api_id":      apiId,
				"ip":          strings.TrimSpace(ip),
				"create_time": now,
			})
			if err != nil {
				return nil, err
			}
		}
	}

	if err = tx.Commit(); err != nil {
		return nil, err
	}
	_ = public.WriteLog(ctx, public.LogParams{
		Type: consts.LOGTYPE.SendAPI,
		Log:  "Create API template :" + req.ApiName + " successfully",
		Data: req,
	})

	res.SetSuccess(public.LangCtx(ctx, "Create API successfully"))
	return res, nil
}
