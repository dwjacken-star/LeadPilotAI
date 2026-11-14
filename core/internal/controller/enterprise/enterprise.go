package enterprise

import (
	"context"
	"crypto/rand"
	"encoding/csv"
	"encoding/hex"
	"encoding/json"
	"errors"

	"io"
	"net/mail"

	"strings"
	"time"

	"billionmail-core/internal/service/batch_mail"
	"billionmail-core/internal/service/contactimport"
	"billionmail-core/internal/service/emailvalidation"
	"billionmail-core/internal/service/governance"
	"billionmail-core/internal/service/public"
	"billionmail-core/internal/service/smtpidentity"
	"billionmail-core/internal/service/webmaildelivery"
	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/net/ghttp"
	"github.com/gogf/gf/v2/util/gconv"
	"golang.org/x/crypto/bcrypt"
)

type Controller struct{}

func New() *Controller { return &Controller{} }

type Request struct {
	g.Meta   `path:"/enterprise/action" method:"post" tags:"Department Workspace" summary:"Departmental business operations"`
	Action   string                 `json:"action" v:"required"`
	Type     string                 `json:"type"`
	ID       int64                  `json:"id"`
	Page     int                    `json:"page"`
	PageSize int                    `json:"page_size"`
	Keyword  string                 `json:"keyword"`
	Data     map[string]interface{} `json:"data"`
}
type Response struct {
	Code    int         `json:"code"`
	Success bool        `json:"success"`
	Msg     string      `json:"msg"`
	Data    interface{} `json:"data"`
}

// Public routes remain responsible for validating their scoped subscription/API
// tokens. Everything else outside this workspace is explicitly administrator-only.
func LegacyBoundary(r *ghttp.Request) {
	if legacyBusinessAccess(r) {
		return
	}
	// The shared application header reads this non-sensitive metadata after JWT.
	if r.URL.Path == "/api/settings/get_version" {
		r.Middleware.Next()
		return
	}
	public := map[string]bool{"/api/login": true, "/api/logout": true, "/api/refresh-token": true, "/api/get_validate_code": true, "/api/languages/get": true, "/api/languages/set": true, "/api/subscribe/submit": true, "/api/subscribe/confirm": true, "/api/unsubscribe": true, "/api/unsubscribe_new": true, "/api/unsubscribe/user_group": true, "/api/batch_mail/api/send": true, "/api/batch_mail/api/batch_send": true}
	if public[r.URL.Path] || r.URL.Path == "/api/enterprise/action" || r.URL.Path == "/api/crm/action" || r.URL.Path == "/api/file/download" || r.URL.Path == "/api/contact/group/preview_import" || r.URL.Path == "/api/social/action" || r.URL.Path == "/api/social/upload" || r.URL.Path == "/api/social/file" || r.URL.Path == "/api/social/export" || r.URL.Path == "/api/growth/action" || r.URL.Path == "/api/growth/upload" || r.URL.Path == "/api/growth/file" || r.URL.Path == "/api/growth/export" {
		r.Middleware.Next()
		return
	}
	p, err := governance.Load(r.GetCtx(), r.GetCtxVar("accountId").Int64())
	if err != nil || !p.Super {
		r.Response.WriteJson(Response{Code: 403, Msg: "该入口仅供系统管理员使用，请进入部门工作台"})
		r.ExitAll()
		return
	}
	r.Middleware.Next()
}
func (c *Controller) Action(ctx context.Context, req *Request) (res *Response, err error) {
	res = &Response{Msg: "操作成功"}
	p, err := governance.Load(ctx, g.RequestFromCtx(ctx).GetCtxVar("accountId").Int64())
	if err != nil {
		res.Code = 401
		res.Msg = err.Error()
		return res, nil
	}
	if req.Data == nil {
		req.Data = map[string]interface{}{}
	}
	// Only a system administrator may create in an explicitly selected department.
	if dept := gconv.Int64(req.Data["dept_id"]); dept > 0 && dept != p.DeptID {
		if !p.Super {
			res.Code = 403
			res.Msg = "不能指定其他部门"
			return res, nil
		}
		copy := *p
		p = &copy
		p.DeptID = dept
	}
	data, e := perform(ctx, p, req)
	if e != nil {
		res.Code = 400
		res.Msg = e.Error()
		return res, nil
	}
	res.Success = true
	res.Data = data
	if req.Action != "me" && req.Action != "list" && req.Action != "preview_import" && req.Action != "export" {
		if e = governance.Audit(ctx, p, req.Action+":"+req.Type, req.ID, g.Map{"type": req.Type}); e != nil {
			g.Log().Error(ctx, "审计记录失败", e)
		}
	}
	return res, nil
}
func perform(ctx context.Context, p *governance.Principal, r *Request) (interface{}, error) {
	switch r.Action {
	case "webmail_bindings":
		return webmaildelivery.Bindings(ctx, p, text(r.Data, "mailbox"), r.Keyword, r.Page)
	case "save_webmail_binding":
		return nil, webmaildelivery.SaveBinding(ctx, p, text(r.Data, "mailbox"), gconv.Int64(r.Data["account_id"]))
	case "validation_settings":
		if !p.Super {
			return nil, errors.New("Administrator access required")
		}
		if mode := text(r.Data, "mode"); mode != "" {
			if mode != "import_only" {
				return nil, errors.New("Invalid validation mode")
			}
			if _, e := g.DB().Exec(ctx, `INSERT INTO bm_options(name,value) VALUES('email_validation_mode',$1) ON CONFLICT(name) DO UPDATE SET value=excluded.value`, mode); e != nil {
				return nil, e
			}
		}
		return g.Map{"mode": emailvalidation.Mode(ctx)}, nil
	case "me":
		name, e := g.DB().Model("bm_departments").Ctx(ctx).Where("id", p.DeptID).Value("name")
		if e != nil {
			return nil, e
		}
		return g.Map{"principal": p, "catalog": governance.AssignablePermissionCatalog(), "permission_dependencies": governance.RolePermissionDependencies, "dept_name": name.String()}, nil
	case "list":
		return list(ctx, p, r)
	case "save":
		return save(ctx, p, r)
	case "create_group_import":
		return createGroupImport(ctx, p, r)
	case "delete":
		return nil, remove(ctx, p, r)
	case "create_task":
		fallthrough
	case "preview_audience":
		var config batch_mail.CampaignConfig
		if e := gconv.Scan(r.Data, &config); e != nil {
			return nil, e
		}
		if r.Action == "preview_audience" {
			n, e := batch_mail.PreviewAudience(ctx, p, config)
			return g.Map{"total": n}, e
		}
		var id int64
		e := g.DB().Transaction(ctx, func(ctx context.Context, tx gdb.TX) error {
			var e error
			id, e = batch_mail.CreateGovernedTask(ctx, tx, p, config, false, 0)
			return e
		})
		return g.Map{"id": id}, e
	case "task_control":
		return nil, taskControl(ctx, p, r)
	case "save_schedule":
		return saveSchedule(ctx, p, r)
	case "save_automation":
		return saveAutomation(ctx, p, r)
	case "preview_import", "import":
		return importContacts(ctx, p, r)
	case "export":
		return exportContacts(ctx, p, r)
	case "assign_tags":
		return nil, assignTags(ctx, p, r)
	case "adopt":
		if !p.Super {
			return nil, errors.New("仅系统管理员可重新分配负责人")
		}
		table := ""
		switch r.Type {
		case "tasks":
			table = "email_tasks"
		case "schedules":
			table = "bm_campaign_schedules"
		case "automations":
			table = "bm_automation_rules"
		default:
			return nil, errors.New("不支持接管此资源")
		}
		row, e := g.DB().Model(table).Where("id", r.ID).One()
		if e != nil || len(row) == 0 {
			return nil, errors.New("资源不存在")
		}
		owner, e := governance.Load(ctx, gconv.Int64(r.Data["owner_id"]))
		if e != nil {
			return nil, e
		}
		if !owner.Super && owner.DeptID != row["dept_id"].Int64() {
			return nil, errors.New("负责人必须属于资源所在部门")
		}
		if !owner.Has("campaign:publish") {
			return nil, errors.New("新负责人缺少发布权限")
		}
		_, e = g.DB().Model(table).Where("id", r.ID).Data(g.Map{"owner_id": owner.ID}).Update()
		return nil, e
	case "reconcile":
		if !p.Super {
			return nil, errors.New("仅系统管理员可核对不确定发送结果")
		}
		state := gconv.String(r.Data["state"])
		if state != "accepted" && state != "failed" {
			return nil, errors.New("只能核对为已接受或失败；不会自动重发")
		}
		_, e := g.DB().Exec(ctx, `UPDATE recipient_info SET delivery_state=$2,is_sent=CASE WHEN $2='accepted' THEN 1 ELSE 3 END,last_error=$3 WHERE id=$1 AND delivery_state='unknown'`, r.ID, state, gconv.String(r.Data["reason"]))
		return nil, e
	default:
		return nil, errors.New("未知操作")
	}
}

type listing struct {
	table, permission, search, fields string
	admin                             bool
}

var listings = map[string]listing{
	"reports":     {"email_tasks", "report:list", "task_name", `id,task_name,dept_id,owner_id,recipient_count,delivered_count,bounced_count,(SELECT count(*) FROM recipient_info WHERE task_id=email_tasks.id AND delivery_state='accepted') submitted_count,(SELECT count(DISTINCT recipient) FROM mailstat_opened WHERE campaign_id=email_tasks.id) opened_count,(SELECT count(DISTINCT recipient) FROM mailstat_clicked WHERE campaign_id=email_tasks.id) clicked_count`, false},
	"departments": {"bm_departments", "", "name", "*", true},
	"users":       {"account", "", "username", "account_id AS id,username,email,dept_id,status,language,(SELECT json_agg(role_id) FROM account_role ar WHERE ar.account_id=account.account_id) role_ids", true},
	"roles":       {"role", "", "role_name", "role_id AS id,role_name,description,status,data_scope,custom_depts,permissions", true},
	"grants":      {"bm_resource_grants", "", "sender", "*", true},
	"policies":    {"bm_send_policies", "", "scope_key", "*", true},
	"audit":       {"bm_security_audit", "", "action", "*", true},
	"groups":      {"bm_contact_groups", "contact:list", "name", "*", false},
	"contacts":    {"bm_contacts", "contact:list", "email", `*,COALESCE((SELECT status FROM bm_email_verifications v WHERE v.email=lower(trim(bm_contacts.email)) AND v.expires_at>now() ORDER BY checked_at DESC LIMIT 1),(SELECT status FROM bm_email_verifications v WHERE v.cache_key='dns:'||split_part(lower(trim(bm_contacts.email)),'@',2) AND v.expires_at>now())) verification_status,COALESCE((SELECT code FROM bm_email_verifications v WHERE v.email=lower(trim(bm_contacts.email)) AND v.expires_at>now() ORDER BY checked_at DESC LIMIT 1),(SELECT code FROM bm_email_verifications v WHERE v.cache_key='dns:'||split_part(lower(trim(bm_contacts.email)),'@',2) AND v.expires_at>now())) verification_code`, false},
	"tags":        {"bm_tags", "contact:list", "name", "*", false},
	"templates":   {"email_templates", "template:list", "temp_name", "*", false},
	"tasks":       {"email_tasks", "campaign:list", "task_name", "*,(SELECT jsonb_object_agg(s.delivery_state,s.n) FROM (SELECT delivery_state,count(*) n FROM recipient_info WHERE task_id=email_tasks.id GROUP BY delivery_state) s) delivery_counts", false},
	"schedules":   {"bm_campaign_schedules", "schedule:list", "name", "*", false},
	"automations": {"bm_automation_rules", "automation:list", "name", "*", false},
}

func list(ctx context.Context, p *governance.Principal, r *Request) (interface{}, error) {
	if r.Page < 1 {
		r.Page = 1
	}
	if r.PageSize < 1 {
		r.PageSize = 20
	}
	if r.PageSize > 200 {
		r.PageSize = 200
	}
	if r.Type == "senders" {
		q := g.DB().Model("mailbox").Ctx(ctx).Where("active", 1)
		if !p.Super {
			q = q.Where(`username IN (SELECT sender FROM bm_resource_grants WHERE dept_id=? AND active)`, p.DeptID)
		}
		if r.Keyword != "" {
			q = q.Where("username ILIKE ?", "%"+r.Keyword+"%")
		}
		if ids := gconv.Strings(r.Data["ids"]); len(ids) > 0 {
			q = q.WhereIn("username", ids)
		}
		n, e := q.Count()
		if e != nil {
			return nil, e
		}
		// Count all filtered rows before applying the multi-column projection.
		// Otherwise the ORM generates COUNT(username,domain,active), invalid in PostgreSQL.
		rows, e := q.Fields("username,domain,active").Order(gdb.Raw(smtpidentity.SenderOrder)).Page(r.Page, r.PageSize).All()
		if rows == nil {
			rows = gdb.Result{}
		}
		defaultSender := ""
		if len(rows) > 0 && r.Page == 1 && r.Keyword == "" && len(gconv.Strings(r.Data["ids"])) == 0 {
			defaultSender = rows[0]["username"].String()
		}
		return g.Map{"list": rows, "total": n, "default_sender": defaultSender}, e
	}
	if r.Type == "deliveries" || r.Type == "runs" || r.Type == "enrollments" {
		parent, code := "email_tasks", "campaign:list"
		if r.Type == "runs" {
			parent = "bm_campaign_schedules"
			code = "schedule:list"
		}
		if r.Type == "enrollments" {
			parent = "bm_automation_rules"
			code = "automation:list"
		}
		if e := p.Check(ctx, parent, r.ID, code); e != nil {
			return nil, e
		}
		table, column := "recipient_info", "task_id"
		if r.Type == "runs" {
			table, column = "bm_campaign_runs", "schedule_id"
		}
		if r.Type == "enrollments" {
			table, column = "bm_automation_enrollments", "rule_id"
		}
		q := g.DB().Model(table).Where(column, r.ID)
		remoteStatus := `(SELECT m.status FROM mailstat_message_ids mid JOIN mailstat_send_mails m ON m.postfix_message_id=mid.postfix_message_id WHERE mid.message_id IN(recipient_info.message_id,btrim(recipient_info.message_id,'<>'),'<'||btrim(recipient_info.message_id,'<>')||'>') AND lower(trim(m.recipient))=lower(trim(recipient_info.recipient)) ORDER BY m.log_time_millis DESC LIMIT 1)`
		if r.Type == "deliveries" {
			if state := text(r.Data, "delivery_state"); state != "" {
				q = q.Where("(CASE WHEN "+remoteStatus+"='bounced' THEN 'failed' ELSE delivery_state END)=?", state)
			}
		}
		n, e := q.Count()
		if e != nil {
			return nil, e
		}
		if r.Type == "deliveries" {
			q = q.Fields("recipient_info.*, " + remoteStatus + " remote_status,(SELECT m.description FROM mailstat_message_ids mid JOIN mailstat_send_mails m ON m.postfix_message_id=mid.postfix_message_id WHERE mid.message_id IN(recipient_info.message_id,btrim(recipient_info.message_id,'<>'),'<'||btrim(recipient_info.message_id,'<>')||'>') AND lower(trim(m.recipient))=lower(trim(recipient_info.recipient)) ORDER BY m.log_time_millis DESC LIMIT 1) remote_error")
		}
		rows, e := q.OrderDesc("id").Page(r.Page, r.PageSize).All()
		if rows == nil {
			rows = gdb.Result{}
		}
		if e == nil {
			e = addListLabels(ctx, p, r.Type, rows)
		}
		return g.Map{"list": rows, "total": n}, e
	}
	spec, ok := listings[r.Type]
	if !ok {
		return nil, errors.New("未知列表")
	}
	if spec.admin && !p.Super {
		return nil, errors.New("仅系统管理员可管理组织和系统资源")
	}
	if !spec.admin && !p.Has(spec.permission) {
		return nil, errors.New("无权查看")
	}
	q := g.DB().Model(spec.table).Ctx(ctx)
	if ids := gconv.Int64s(r.Data["ids"]); len(ids) > 0 {
		pk := "id"
		if r.Type == "users" {
			pk = "account_id"
		}
		if r.Type == "roles" {
			pk = "role_id"
		}
		q = q.WhereIn(pk, ids)
	}
	if !spec.admin {
		predicate, e := p.Predicate(ctx, spec.permission, "")
		if e != nil {
			return nil, e
		}
		q = q.Where(predicate)
	}
	if r.Keyword != "" && spec.search != "" {
		q = q.Where(spec.search+" ILIKE ?", "%"+r.Keyword+"%")
	}
	if group := gconv.Int64(r.Data["group_id"]); group > 0 && (r.Type == "contacts" || r.Type == "tags") {
		q = q.Where("group_id", group)
	}
	if ids := gconv.Int64s(r.Data["group_ids"]); len(ids) > 0 && (r.Type == "contacts" || r.Type == "tags") {
		q = q.WhereIn("group_id", ids)
	}
	n, e := q.Count()
	if e != nil {
		return nil, e
	}
	order := "id DESC"
	if r.Type == "users" {
		order = "account_id DESC"
	}
	if r.Type == "roles" {
		order = "role_id DESC"
	}
	if r.Type == "policies" {
		order = "scope,scope_key"
	}
	rows, e := q.Fields(spec.fields).Order(order).Page(r.Page, r.PageSize).All()
	if rows == nil {
		rows = gdb.Result{}
	}
	if e == nil {
		e = addListLabels(ctx, p, r.Type, rows)
	}
	return g.Map{"list": rows, "total": n}, e
}
func text(d map[string]interface{}, key string) string {
	return strings.TrimSpace(gconv.String(d[key]))
}
func jsonText(v interface{}) string { b, _ := json.Marshal(v); return string(b) }
func save(ctx context.Context, p *governance.Principal, r *Request) (interface{}, error) {
	if r.Type == "departments" || r.Type == "users" || r.Type == "roles" || r.Type == "grants" || r.Type == "policies" {
		if !p.Super {
			return nil, errors.New("仅系统管理员可修改组织与发信资源")
		}
		return saveAdmin(ctx, p, r)
	}
	spec, ok := listings[r.Type]
	if !ok || spec.admin {
		return nil, errors.New("不支持保存此资源")
	}
	code := "contact:create"
	if r.Type == "templates" {
		code = "template:create"
	}
	if r.ID > 0 {
		code = strings.Replace(code, ":create", ":update", 1)
		if e := p.Check(ctx, spec.table, r.ID, code); e != nil {
			return nil, e
		}
	}
	if !p.Has(code) {
		return nil, errors.New("无权修改")
	}
	d := g.Map{}
	switch r.Type {
	case "groups":
		if text(r.Data, "name") == "" {
			return nil, errors.New("分组名称不能为空")
		}
		d["name"] = text(r.Data, "name")
		d["description"] = text(r.Data, "description")
		if r.ID == 0 {
			token := make([]byte, 12)
			if _, e := rand.Read(token); e != nil {
				return nil, e
			}
			d["token"] = hex.EncodeToString(token)
		}
	case "contacts":
		email := strings.ToLower(text(r.Data, "email"))
		a, e := mail.ParseAddress(email)
		if e != nil || a.Address != email {
			return nil, errors.New("邮箱格式无效")
		}
		d["email"] = email
		d["active"] = gconv.Int(r.Data["active"])
		d["status"] = gconv.Int(r.Data["status"])
		if d["active"] != 0 && d["active"] != 1 || d["status"] != 0 && d["status"] != 1 {
			return nil, errors.New("联系人状态无效")
		}
		d["attribs"] = jsonText(r.Data["attribs"])
		if r.Data["attribs"] == nil {
			d["attribs"] = "{}"
		}
		fallthrough
	case "tags":
		group := gconv.Int64(r.Data["group_id"])
		if e := p.Check(ctx, "bm_contact_groups", group, code); e != nil {
			return nil, e
		}
		gRow, e := g.DB().Model("bm_contact_groups").Where("id", group).One()
		if e != nil {
			return nil, e
		}
		if !p.Super && gRow["dept_id"].Int64() != p.DeptID {
			return nil, errors.New("不能把数据加入其他部门的联系人组")
		}
		if r.ID > 0 {
			old, e := g.DB().Model(spec.table).Where("id", r.ID).One()
			if e != nil {
				return nil, e
			}
			if old["dept_id"].Int64() != gRow["dept_id"].Int64() {
				return nil, errors.New("跨部门移动需单独迁移")
			}
		}
		d["group_id"] = group
		d["dept_id"] = gRow["dept_id"]
		if r.Type == "tags" {
			d["name"] = text(r.Data, "name")
			d["color"] = text(r.Data, "color")
			if d["name"] == "" {
				return nil, errors.New("标签名称不能为空")
			}
		}
	case "templates":
		if text(r.Data, "temp_name") == "" || text(r.Data, "content") == "" {
			return nil, errors.New("模板名称和内容不能为空")
		}
		d["temp_name"] = text(r.Data, "temp_name")
		d["content"] = gconv.String(r.Data["content"])
		d["add_type"] = 0
	default:
		return nil, errors.New("请使用专用操作修改任务或计划")
	}
	if r.ID == 0 {
		d["owner_id"] = p.ID
		if d["dept_id"] == nil {
			d["dept_id"] = p.DeptID
		}
		if e := p.CheckOwnership(ctx, code, gconv.Int64(d["dept_id"]), p.ID); e != nil {
			return nil, e
		}
		id, e := g.DB().Model(spec.table).Ctx(ctx).Data(d).InsertAndGetId()
		return g.Map{"id": id}, e
	}
	_, e := g.DB().Model(spec.table).Ctx(ctx).Where("id", r.ID).Data(d).Update()
	return g.Map{"id": r.ID}, e
}
func saveAdmin(ctx context.Context, p *governance.Principal, r *Request) (interface{}, error) {
	var id = r.ID
	err := g.DB().Transaction(ctx, func(ctx context.Context, tx gdb.TX) error {
		if _, e := tx.Exec(`SELECT pg_advisory_xact_lock(614807)`); e != nil {
			return e
		}
		d := g.Map{}
		table, pk := "", "id"
		switch r.Type {
		case "departments":
			table = "bm_departments"
			name := text(r.Data, "name")
			if name == "" {
				return errors.New("部门名称不能为空")
			}
			parent := gconv.Int64(r.Data["parent_id"])
			if parent == r.ID && parent > 0 {
				return errors.New("部门不能归属自己")
			}
			if parent > 0 && r.ID > 0 {
				v, e := tx.GetValue(`WITH RECURSIVE children AS(SELECT id FROM bm_departments WHERE id=$1 UNION ALL SELECT d.id FROM bm_departments d JOIN children c ON d.parent_id=c.id) SELECT count(*) FROM children WHERE id=$2`, r.ID, parent)
				if e != nil {
					return e
				}
				if v.Int() > 0 {
					return errors.New("部门不能移动到下级部门")
				}
			}
			d["name"] = name
			d["parent_id"] = nil
			if parent > 0 {
				d["parent_id"] = parent
			}
			d["active"] = gconv.Bool(r.Data["active"])
			if r.ID == 1 && !gconv.Bool(r.Data["active"]) {
				return errors.New("不能停用默认部门")
			}
		case "roles":
			table = "role"
			pk = "role_id"
			if r.ID > 0 {
				v, e := tx.GetValue(`SELECT role_name FROM role WHERE role_id=$1`, r.ID)
				if e != nil {
					return e
				}
				if v.String() == "admin" {
					return errors.New("超级管理员角色不可修改")
				}
			}
			name := text(r.Data, "role_name")
			if name == "" || name == "admin" {
				return errors.New("角色名称无效")
			}
			scope := text(r.Data, "data_scope")
			if scope != "all" && scope != "department" && scope != "descendants" && scope != "self" && scope != "custom" {
				return errors.New("数据范围无效")
			}
			permissions, e := governance.NormalizeRolePermissions(gconv.Strings(r.Data["permissions"]))
			if e != nil {
				return e
			}
			d = g.Map{"role_name": name, "description": text(r.Data, "description"), "status": gconv.Int(r.Data["status"]), "data_scope": scope, "custom_depts": jsonText(gconv.Int64s(r.Data["custom_depts"])), "permissions": jsonText(permissions)}
		case "users":
			table = "account"
			pk = "account_id"
			if r.ID > 0 {
				v, e := tx.GetValue(`SELECT count(*) FROM account_role ar JOIN role r USING(role_id) WHERE ar.account_id=$1 AND r.role_name='admin'`, r.ID)
				if e != nil {
					return e
				}
				if v.Int() > 0 {
					return errors.New("原始超级管理员账号通过专用运维入口管理")
				}
			}
			username := text(r.Data, "username")
			if username == "" {
				return errors.New("用户名不能为空")
			}
			dept := gconv.Int64(r.Data["dept_id"])
			if dept == 0 {
				dept = p.DeptID
			}
			d = g.Map{"username": username, "email": text(r.Data, "email"), "dept_id": dept, "status": gconv.Int(r.Data["status"]), "language": "zh"}
			password := gconv.String(r.Data["password"])
			if r.ID == 0 || password != "" {
				if len([]rune(password)) < 8 || len(password) > 72 {
					return errors.New("密码至少8个字符")
				}
				hash, e := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
				if e != nil {
					return e
				}
				d["password"] = string(hash)
			}
		case "grants":
			table = "bm_resource_grants"
			d = g.Map{"dept_id": gconv.Int64(r.Data["dept_id"]), "sender": strings.ToLower(text(r.Data, "sender")), "active": gconv.Bool(r.Data["active"])}
		case "policies":
			var policy governance.Policy
			if e := gconv.Scan(r.Data["policy"], &policy); e != nil {
				return e
			}
			if e := policy.Validate(); e != nil {
				return e
			}
			scope := text(r.Data, "scope")
			valid := map[string]bool{"global": true, "department": true, "sender": true, "ip": true, "provider": true}
			if !valid[scope] {
				return errors.New("无效策略范围")
			}
			key := text(r.Data, "scope_key")
			if scope == "global" {
				key = "*"
			}
			if key == "" {
				return errors.New("范围标识不能为空")
			}
			_, e := tx.Exec(`INSERT INTO bm_send_policies VALUES($1,$2,$3::jsonb) ON CONFLICT(scope,scope_key) DO UPDATE SET policy=excluded.policy`, scope, key, jsonText(policy))
			return e
		}
		if r.ID == 0 {
			var e error
			id, e = tx.Model(table).Data(d).InsertAndGetId()
			if e != nil {
				return e
			}
		} else {
			_, e := tx.Model(table).Where(pk, r.ID).Data(d).Update()
			if e != nil {
				return e
			}
		}
		if r.Type == "users" {
			if _, e := tx.Exec(`DELETE FROM account_role WHERE account_id=$1`, id); e != nil {
				return e
			}
			for _, role := range gconv.Int64s(r.Data["role_ids"]) {
				v, e := tx.GetValue(`SELECT count(*) FROM role WHERE role_id=$1 AND role_name<>'admin' AND status=1`, role)
				if e != nil {
					return e
				}
				if v.Int() == 0 {
					return errors.New("角色不存在或不能分配")
				}
				if _, e = tx.Exec(`INSERT INTO account_role(account_id,role_id) VALUES($1,$2)`, id, role); e != nil {
					return e
				}
			}
			_, e := tx.Exec(`UPDATE account SET token_version=token_version+1 WHERE account_id=$1`, id)
			return e
		}
		if r.Type == "roles" {
			_, e := tx.Exec(`UPDATE account SET token_version=token_version+1 WHERE account_id IN(SELECT account_id FROM account_role WHERE role_id=$1)`, id)
			return e
		}
		return nil
	})
	return g.Map{"id": id}, err
}
func remove(ctx context.Context, p *governance.Principal, r *Request) error {
	spec, ok := listings[r.Type]
	if !ok || spec.admin {
		return errors.New("组织与系统资源请停用；不支持直接删除")
	}
	code := "contact:delete"
	if r.Type == "templates" {
		code = "template:delete"
	}
	if r.Type != "groups" && r.Type != "contacts" && r.Type != "tags" && r.Type != "templates" {
		return errors.New("任务和计划请取消或停用")
	}
	if e := p.Check(ctx, spec.table, r.ID, code); e != nil {
		return e
	}
	if r.Type == "groups" {
		n, e := g.DB().Model("bm_contacts").Where("group_id", r.ID).Count()
		if e != nil {
			return e
		}
		if n > 0 {
			return errors.New("分组内仍有联系人，不能删除")
		}
	}
	_, e := g.DB().Model(spec.table).Where("id", r.ID).Delete()
	return e
}
func taskControl(ctx context.Context, p *governance.Principal, r *Request) error {
	op := text(r.Data, "operation")
	code := "campaign:pause"
	if op == "publish" || op == "resume" || op == "recheck" {
		code = "campaign:publish"
	}
	if op == "cancel" {
		code = "campaign:cancel"
	}
	if e := p.Check(ctx, "email_tasks", r.ID, code); e != nil {
		return e
	}
	if op != "publish" && op != "resume" && op != "pause" && op != "cancel" && op != "policy" && op != "recheck" {
		return errors.New("未知任务操作")
	}
	if op == "policy" {
		var policy governance.Policy
		if e := gconv.Scan(r.Data["policy"], &policy); e != nil {
			return e
		}
		if e := policy.Validate(); e != nil {
			return e
		}
		_, e := g.DB().Exec(ctx, `UPDATE email_tasks SET send_policy=$2::jsonb,next_attempt_at=now(),wait_reason='' WHERE id=$1`, r.ID, jsonText(policy))
		return e
	}
	if op == "recheck" {
		return batch_mail.RecheckTaskRecipients(ctx, r.ID)
	}
	if op == "publish" || op == "resume" {
		row, e := g.DB().Model("email_tasks").Where("id", r.ID).One()
		if e != nil {
			return e
		}
		if row["task_process"].Int() == 2 || row["task_process"].Int() == 4 {
			return errors.New("已结束的任务不能重新启动，请创建新批次")
		}
		if e = governance.SenderAllowed(ctx, p, row["addresser"].String()); e != nil {
			return e
		}
		_, e = g.DB().Exec(ctx, `UPDATE email_tasks SET pause=0,task_process=CASE WHEN task_process=3 THEN 1 ELSE task_process END,next_attempt_at=now(),wait_reason='' WHERE id=$1`, r.ID)
		return e
	}
	if op == "cancel" {
		return g.DB().Transaction(ctx, func(ctx context.Context, tx gdb.TX) error {
			if _, e := tx.Exec(`UPDATE email_tasks SET pause=1,task_process=4 WHERE id=$1`, r.ID); e != nil {
				return e
			}
			_, e := tx.Exec(`UPDATE recipient_info SET delivery_state='skipped',is_sent=3,last_error='任务已取消' WHERE task_id=$1 AND delivery_state IN ('pending','retry','claimed','validation_hold')`, r.ID)
			return e
		})
	}
	_, e := g.DB().Exec(ctx, `UPDATE email_tasks SET pause=1 WHERE id=$1`, r.ID)
	return e
}
func saveSchedule(ctx context.Context, p *governance.Principal, r *Request) (interface{}, error) {
	if r.ID == 0 {
		if e := p.CheckOwnership(ctx, "schedule:manage", p.DeptID, p.ID); e != nil {
			return nil, e
		}
	}
	if !p.Has("schedule:manage") || !p.Has("campaign:publish") {
		return nil, errors.New("需要计划管理和任务发布权限")
	}
	var spec batch_mail.ScheduleSpec
	if e := gconv.Scan(r.Data, &spec); e != nil {
		return nil, e
	}
	if e := spec.Validate(); e != nil {
		return nil, e
	}
	if e := batch_mail.ValidateCampaign(ctx, p, spec.Campaign); e != nil {
		return nil, e
	}
	next, e := time.Parse(time.RFC3339, spec.NextAt)
	if e != nil {
		return nil, errors.New("首次执行时间必须包含时区")
	}
	var ends interface{}
	if spec.EndsAt != "" {
		v, e := time.Parse(time.RFC3339, spec.EndsAt)
		if e != nil || !v.After(next) {
			return nil, errors.New("结束时间必须晚于首次执行时间")
		}
		ends = v
	}
	d := g.Map{"name": spec.Name, "config": jsonText(spec.Campaign), "cadence": spec.Cadence, "timezone": spec.Timezone, "local_time": spec.LocalTime, "weekdays": jsonText(spec.Weekdays), "month_day": spec.MonthDay, "next_run_at": next, "ends_at": ends, "max_runs": spec.MaxRuns, "active": gconv.Bool(r.Data["active"])}
	if spec.MonthDay == 0 {
		d["month_day"] = 1
	}
	if r.ID > 0 {
		if e = p.Check(ctx, "bm_campaign_schedules", r.ID, "schedule:manage"); e != nil {
			return nil, e
		}
		_, e = g.DB().Model("bm_campaign_schedules").Where("id", r.ID).Data(d).Update()
		return g.Map{"id": r.ID}, e
	}
	d["dept_id"] = p.DeptID
	d["owner_id"] = p.ID
	id, e := g.DB().Model("bm_campaign_schedules").Data(d).InsertAndGetId()
	return g.Map{"id": id}, e
}
func saveAutomation(ctx context.Context, p *governance.Principal, r *Request) (interface{}, error) {
	if r.ID == 0 {
		if e := p.CheckOwnership(ctx, "automation:manage", p.DeptID, p.ID); e != nil {
			return nil, e
		}
	}
	if !p.Has("automation:manage") || !p.Has("campaign:publish") {
		return nil, errors.New("需要自动化管理和任务发布权限")
	}
	var config batch_mail.AutomationConfig
	if e := gconv.Scan(r.Data["config"], &config); e != nil {
		return nil, e
	}
	groups := batch_mail.NormalizeGroups(config.Campaign.GroupIDs, config.Campaign.GroupID)
	if len(groups) != 1 {
		return nil, errors.New("Automation enrollment requires one contact group")
	}
	config.Campaign.GroupID = groups[0]
	if e := config.Validate(ctx, p); e != nil {
		return nil, e
	}
	d := g.Map{"name": text(r.Data, "name"), "config": jsonText(config), "group_id": config.Campaign.GroupID, "active": gconv.Bool(r.Data["active"])}
	if r.ID > 0 {
		if e := p.Check(ctx, "bm_automation_rules", r.ID, "automation:manage"); e != nil {
			return nil, e
		}
		if gconv.Bool(r.Data["active"]) {
			d["activated_at"] = gdb.Raw("CASE WHEN NOT active THEN now() ELSE activated_at END")
		}
		_, e := g.DB().Model("bm_automation_rules").Where("id", r.ID).Data(d).Update()
		return g.Map{"id": r.ID}, e
	}
	d["dept_id"] = p.DeptID
	d["owner_id"] = p.ID
	id, e := g.DB().Model("bm_automation_rules").Data(d).InsertAndGetId()
	return g.Map{"id": id}, e
}
func importContacts(ctx context.Context, p *governance.Principal, r *Request) (interface{}, error) {
	keywords, keywordErr := public.NormalizeImportKeywords(gconv.Strings(r.Data["filter_keywords"]))
	if keywordErr != nil {
		return nil, keywordErr
	}
	filtered := 0
	if !p.Has("contact:import") {
		return nil, errors.New("无导入权限")
	}
	group := gconv.Int64(r.Data["group_id"])
	if e := p.Check(ctx, "bm_contact_groups", group, "contact:import"); e != nil {
		return nil, e
	}
	row, e := g.DB().Model("bm_contact_groups").Where("id", group).One()
	if e != nil {
		return nil, e
	}
	if !p.Super && row["dept_id"].Int64() != p.DeptID {
		return nil, errors.New("不能导入其他部门分组")
	}
	content := gconv.String(r.Data["csv"])
	if gconv.String(r.Data["file_type"]) == "xlsx" {
		return importWorkbook(ctx, p, r, group, row["dept_id"].Int64(), content)
	}

	if len(content) > 20*1024*1024 {
		return nil, errors.New("CSV最大20MB，请分批导入")
	}
	reader := csv.NewReader(strings.NewReader(strings.TrimPrefix(content, "\ufeff")))
	reader.FieldsPerRecord = -1
	invalid := []g.Map{}
	valid := []g.Map{}
	seen := map[string]bool{}
	line := 0
	headers := []string{}
	for {
		fields, err := reader.Read()
		if err == io.EOF {
			break
		}
		line++
		if err != nil {
			invalid = append(invalid, g.Map{"row": line, "email": "", "reason": err.Error()})
			continue
		}
		if len(fields) == 0 {
			continue
		}
		email := strings.ToLower(strings.TrimSpace(fields[0]))
		if line == 1 && email == "email" {
			headers = fields
			continue
		}
		if public.MatchesImportKeywords(email, keywords) {
			filtered++
			continue
		}
		parsed, e := mail.ParseAddress(email)
		if e != nil || parsed.Address != email {
			invalid = append(invalid, g.Map{"row": line, "email": email, "reason": "邮箱格式无效"})
			continue
		}
		if seen[email] {
			continue
		}
		seen[email] = true
		attrs := g.Map{}
		if len(fields) > 1 {
			attrs["name"] = fields[1]
		}
		for i, key := range headers {
			if i >= len(fields) || i == 0 || i == 1 || fields[i] == "" {
				continue
			}
			key = strings.ToLower(strings.TrimSpace(key))
			if key == "attributes" {
				var extra map[string]interface{}
				if json.Unmarshal([]byte(fields[i]), &extra) != nil {
					invalid = append(invalid, g.Map{"row": line, "email": email, "reason": "Invalid attributes JSON"})
					attrs = nil
					break
				}
				for k, v := range extra {
					attrs[k] = v
				}
			} else {
				attrs[key] = fields[i]
			}
		}
		if attrs == nil {
			continue
		}
		valid = append(valid, g.Map{"email": email, "group_id": group, "dept_id": row["dept_id"], "owner_id": p.ID, "active": 1, "status": gconv.Int(gconv.Bool(r.Data["confirmed"])), "attribs": jsonText(attrs)})
	}
	inserted := int64(0)
	emails := []string{}
	for _, row := range valid {
		emails = append(emails, gconv.String(row["email"]))
	}
	sender, e := contactimport.ResolveSender(ctx, p, text(r.Data, "validation_sender"))
	if e != nil {
		return nil, e
	}
	checks := emailvalidation.Recipients(ctx, sender, emails)
	verified := valid[:0]
	pending := []g.Map{}
	for _, item := range valid {
		email := gconv.String(item["email"])
		check := checks[email]
		if check.Status == emailvalidation.Valid {
			verified = append(verified, item)
		} else if check.Status == emailvalidation.Invalid {
			invalid = append(invalid, g.Map{"email": email, "reason": check.Code})
		} else {
			pending = append(pending, g.Map{"email": email, "reason": check.Code})
		}
	}
	valid = verified

	if r.Action == "import" && len(valid) > 0 {
		e = g.DB().Transaction(ctx, func(ctx context.Context, tx gdb.TX) error {
			for start := 0; start < len(valid); start += 500 {
				end := start + 500
				if end > len(valid) {
					end = len(valid)
				}
				result, e := tx.Model("bm_contacts").OnConflict("group_id,email").InsertIgnore(valid[start:end])
				if e != nil {
					return e
				}
				n, _ := result.RowsAffected()
				inserted += n
			}
			return nil
		})
	}
	return g.Map{"valid": len(valid), "invalid": invalid, "inserted": inserted, "filtered_count": filtered, "pending": pending}, e
}
func exportContacts(ctx context.Context, p *governance.Principal, r *Request) (interface{}, error) {
	if !p.Has("contact:export") {
		return nil, errors.New("无导出权限")
	}
	where, e := p.Predicate(ctx, "contact:export", "")
	if e != nil {
		return nil, e
	}
	q := g.DB().Model("bm_contacts").Where(where)
	if group := gconv.Int64(r.Data["group_id"]); group > 0 {
		q = q.Where("group_id", group)
	}
	rows, e := q.Fields("id,email,group_id,active,status").Order("id").Limit(100001).All()
	if e != nil {
		return nil, e
	}
	if len(rows) > 100000 {
		return nil, errors.New("单次导出最多10万条，请按分组导出")
	}
	var b strings.Builder
	w := csv.NewWriter(&b)
	_ = w.Write([]string{"id", "email", "group_id", "active", "status"})
	for _, row := range rows {
		email := row["email"].String()
		if strings.ContainsAny(email[:1], "=+-@") {
			email = "'" + email
		}
		_ = w.Write([]string{row["id"].String(), email, row["group_id"].String(), row["active"].String(), row["status"].String()})
	}
	w.Flush()
	return g.Map{"csv": b.String()}, w.Error()
}
func assignTags(ctx context.Context, p *governance.Principal, r *Request) error {
	ids := gconv.Int64s(r.Data["contact_ids"])
	tags := gconv.Int64s(r.Data["tag_ids"])
	if len(ids) > 500 || len(tags) > 100 {
		return errors.New("请分批设置标签")
	}
	for _, id := range ids {
		if e := p.Check(ctx, "bm_contacts", id, "contact:update"); e != nil {
			return e
		}
	}
	for _, id := range tags {
		if e := p.Check(ctx, "bm_tags", id, "contact:update"); e != nil {
			return e
		}
	}
	return g.DB().Transaction(ctx, func(ctx context.Context, tx gdb.TX) error {
		for _, id := range ids {
			for _, tag := range tags {
				v, e := tx.GetValue(`SELECT count(*) FROM bm_contacts c JOIN bm_tags t ON t.group_id=c.group_id AND t.dept_id=c.dept_id WHERE c.id=$1 AND t.id=$2`, id, tag)
				if e != nil {
					return e
				}
				if v.Int() == 0 {
					return errors.New("联系人和标签必须属于同一分组和部门")
				}
				if _, e = tx.Exec(`INSERT INTO bm_contact_tags(contact_id,tag_id) VALUES($1,$2) ON CONFLICT DO NOTHING`, id, tag); e != nil {
					return e
				}
			}
		}
		return nil
	})
}
