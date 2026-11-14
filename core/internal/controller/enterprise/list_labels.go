package enterprise

import (
	"context"
	"strings"

	"billionmail-core/internal/service/governance"
	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/util/gconv"
)

type labelReference struct {
	row         gdb.Record
	field, kind string
	ids         []int64
	multiple    bool
}

// Resolve only references in the current page, in batches. Keep original IDs for
// editing and apply the referenced resource's read scope before returning names.
func addListLabels(ctx context.Context, p *governance.Principal, kind string, rows gdb.Result) error {
	refs := []labelReference{}
	add := func(row gdb.Record, field, target string, value interface{}, multiple bool) {
		ids := []int64{}
		if multiple {
			ids = gconv.Int64s(value)
		} else if id := gconv.Int64(value); id > 0 {
			ids = append(ids, id)
		}
		refs = append(refs, labelReference{row, field, target, ids, multiple})
	}
	for _, row := range rows {
		if row["dept_id"] != nil {
			add(row, "dept_name", "departments", row["dept_id"], false)
		}
		switch kind {
		case "departments":
			add(row, "parent_name", "departments", row["parent_id"], false)
		case "users":
			add(row, "role_names", "roles", row["role_ids"].Interfaces(), true)
		case "roles":
			add(row, "custom_dept_names", "departments", row["custom_depts"].Interfaces(), true)
		case "contacts", "tags", "automations":
			add(row, "group_name", "groups", row["group_id"], false)
		case "policies":
			if row["scope"].String() == "department" {
				add(row, "scope_name", "departments", row["scope_key"], false)
			}
		case "runs":
			add(row, "task_name", "tasks", row["task_id"], false)
		case "enrollments":
			add(row, "contact_email", "contacts", row["contact_id"], false)
		case "audit":
			add(row, "account_name", "users", row["account_id"], false)
			parts := strings.Split(row["action"].String(), ":")
			if len(parts) > 1 {
				add(row, "resource_name", parts[1], row["resource_id"], false)
			}
		}
		if kind == "tasks" || kind == "schedules" {
			config := row.Map()
			if kind == "schedules" {
				config = gconv.Map(row["config"])
			}
			ids := gconv.Int64s(config["group_ids"])
			if len(ids) == 0 && gconv.Int64(config["group_id"]) > 0 {
				ids = []int64{gconv.Int64(config["group_id"])}
			}
			add(row, "group_names", "groups", ids, true)
			add(row, "template_name", "templates", config["template_id"], false)
		}
	}
	lookup := map[string]map[int64]string{}
	targets := map[string]map[int64]bool{}
	for _, ref := range refs {
		if targets[ref.kind] == nil {
			targets[ref.kind] = map[int64]bool{}
		}
		for _, id := range ref.ids {
			if id > 0 {
				targets[ref.kind][id] = true
			}
		}
	}
	for target, selected := range targets {
		lookup[target] = map[int64]string{}
		if len(selected) == 0 {
			continue
		}
		spec, ok := listings[target]
		if !ok {
			continue
		}
		pk, name := "id", spec.search
		if target == "users" {
			pk = "account_id"
		}
		if target == "roles" {
			pk = "role_id"
		}
		if name == "" {
			continue
		}
		ids := make([]int64, 0, len(selected))
		for id := range selected {
			ids = append(ids, id)
		}
		q := g.DB().Model(spec.table).Ctx(ctx).WhereIn(pk, ids)
		if !spec.admin {
			predicate, err := p.Predicate(ctx, spec.permission, "")
			if err != nil {
				return err
			}
			q = q.Where(predicate)
		} else if !p.Super && target != "departments" {
			continue
		}
		found, err := q.Fields(pk + " AS label_id," + name + " AS label_name").All()
		if err != nil {
			return err
		}
		for _, item := range found {
			lookup[target][item["label_id"].Int64()] = item["label_name"].String()
		}
	}
	for _, ref := range refs {
		if ref.multiple {
			names := []string{}
			for _, id := range ref.ids {
				names = append(names, lookup[ref.kind][id])
			}
			ref.row[ref.field] = g.NewVar(names)
		} else {
			name := ""
			if len(ref.ids) > 0 {
				name = lookup[ref.kind][ref.ids[0]]
			}
			ref.row[ref.field] = g.NewVar(name)
		}
	}
	return nil
}
