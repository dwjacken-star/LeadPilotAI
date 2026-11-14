package social

import (
	"context"
	"encoding/json"
	"errors"
	"sort"
	"time"

	"billionmail-core/internal/service/governance"
	"billionmail-core/internal/service/growth"
	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/frame/g"
)

func authorizationTask(ctx context.Context, p *governance.Principal, id int64) (gdb.Record, error) {
	if e := require(p, "social:account:manage"); e != nil {
		return nil, e
	}
	if e := p.CheckOwnership(ctx, "social:account:manage", p.DeptID, p.ID); e != nil {
		return nil, e
	}
	row, e := g.DB().Model("bm_social_oauth_states").Ctx(ctx).Where("id", id).Where("owner_id", p.ID).Where("dept_id", p.DeptID).One()
	if e != nil {
		return nil, e
	}
	if len(row) == 0 {
		return nil, errors.New("authorization task unavailable")
	}
	return row, nil
}

func pendingPayload(row gdb.Record) (pendingAuthorization, error) {
	var pending pendingAuthorization
	raw, e := growth.SocialCredential(row["encrypted_payload"].String(), true)
	if e != nil {
		return pending, errors.New("pending authorization unavailable")
	}
	e = json.Unmarshal([]byte(raw), &pending)
	return pending, e
}

func AuthorizationStatus(ctx context.Context, p *governance.Principal, id int64) (g.Map, error) {
	row, e := authorizationTask(ctx, p, id)
	if e != nil {
		return nil, e
	}
	status := row["status"].String()
	if row["expires_at"].Time().Before(time.Now()) && (status == "pending" || status == "ready") {
		status = "expired"
		_, e = g.DB().Model("bm_social_oauth_states").Ctx(ctx).Where("id", id).WhereIn("status", []string{"pending", "ready"}).Data(g.Map{"status": status, "encrypted_payload": ""}).Update()
		if e != nil {
			return nil, e
		}
	}
	candidates := []g.Map{}
	if status == "ready" {
		pending, e := pendingPayload(row)
		if e != nil {
			return nil, e
		}
		for _, page := range pending.Pages {
			candidates = append(candidates, g.Map{"external_id": page.ExternalID, "name": page.Name, "provider_name": page.ProviderName, "provider_user": page.ProviderUser})
		}
	}
	return g.Map{"id": id, "status": status, "platform": row["platform"], "target_type": row["target_type"], "expires_at": row["expires_at"], "expected_account_id": row["expected_account_id"].Int64(), "candidates": candidates, "error": row["error"].String()}, nil
}

func CancelAuthorization(ctx context.Context, p *governance.Principal, id int64) error {
	if _, e := authorizationTask(ctx, p, id); e != nil {
		return e
	}
	_, e := g.DB().Model("bm_social_oauth_states").Ctx(ctx).Where("id", id).WhereIn("status", []string{"pending", "ready", "failed"}).Data(g.Map{"status": "cancelled", "encrypted_payload": ""}).Update()
	return e
}

func ConfirmAuthorization(ctx context.Context, p *governance.Principal, id int64, externalIDs []string, allowProviderChange bool) (g.Map, error) {
	if _, e := authorizationTask(ctx, p, id); e != nil {
		return nil, e
	}
	if len(externalIDs) < 1 || len(externalIDs) > 30 {
		return nil, errors.New("select 1-30 authorized targets")
	}
	selected := map[string]bool{}
	for _, key := range externalIDs {
		if key == "" || selected[key] {
			return nil, errors.New("invalid or duplicate authorization target")
		}
		selected[key] = true
	}
	e := g.DB().Transaction(ctx, func(ctx context.Context, tx gdb.TX) error {
		row, e := tx.GetOne(`SELECT * FROM bm_social_oauth_states WHERE id=$1 AND owner_id=$2 AND dept_id=$3 FOR UPDATE`, id, p.ID, p.DeptID)
		if e != nil {
			return e
		}
		if len(row) == 0 || row["status"].String() != "ready" || !row["expires_at"].Time().After(time.Now()) {
			return errors.New("authorization task is not ready or has expired")
		}
		platform, targetType := row["platform"].String(), row["target_type"].String()
		c, e := requirePlatform(ctx, platform, false)
		if e != nil {
			return e
		}
		if authorizationFingerprint(c, targetType) != row["app_fingerprint"].String() {
			return errors.New("application configuration changed; start a new connection")
		}
		pending, e := pendingPayload(row)
		if e != nil {
			return e
		}
		if pending.ExpiresAt != nil && !pending.ExpiresAt.After(time.Now()) {
			return errors.New("provider authorization expired; reconnect")
		}
		pages := []ConnectedPage{}
		for _, page := range pending.Pages {
			if selected[page.ExternalID] {
				pages = append(pages, page)
			}
		}
		if len(pages) != len(selected) {
			return errors.New("selected target was not authorized")
		}
		expectedID := row["expected_account_id"].Int64()
		if expectedID > 0 {
			expected, e := scoped(ctx, p, "bm_social_accounts", expectedID, "social:account:manage")
			if e != nil {
				return e
			}
			if len(pages) != 1 || expected["platform"].String() != platform || expected["target_type"].String() != targetType || expected["external_id"].String() != pages[0].ExternalID || expected["publish_mode"].String() != "automatic" {
				return errors.New("the authorized identity does not match the reconnect target")
			}
		}
		refresh := ""
		if pending.Refresh != "" {
			refresh, e = growth.SocialCredential(pending.Refresh, false)
			if e != nil {
				return e
			}
		}
		sort.Slice(pages, func(i, j int) bool { return pages[i].ExternalID < pages[j].ExternalID })
		for _, page := range pages {
			caps := accountCapabilities(platform, targetType, page.Scopes)
			if page.Token == "" || page.ProviderUser == "" || !caps["publish"].(bool) {
				return errors.New("target lacks verified publishing authorization")
			}
			if _, e = tx.Exec(`SELECT pg_advisory_xact_lock(hashtextextended($1,0))`, "social:"+platform+":"+targetType+":"+page.ExternalID); e != nil {
				return e
			}
			prior, e := tx.GetOne(`SELECT * FROM bm_social_accounts WHERE platform=$1 AND target_type=$2 AND external_id=$3 FOR UPDATE`, platform, targetType, page.ExternalID)
			if e != nil {
				return e
			}
			if len(prior) > 0 {
				if prior["dept_id"].Int64() != p.DeptID {
					return errors.New("target belongs to another department; use an administrator grant")
				}
				if _, e = scoped(ctx, p, "bm_social_accounts", prior["id"].Int64(), "social:account:manage"); e != nil {
					return e
				}
				if prior["provider_user"].String() != page.ProviderUser && !allowProviderChange {
					return errors.New("authorizing identity changed; explicit confirmation required")
				}
				if expectedID > 0 && expectedID != prior["id"].Int64() {
					return errors.New("reconnect target changed")
				}
			} else if expectedID > 0 {
				return errors.New("reconnect target unavailable")
			}
			encrypted, e := growth.SocialCredential(page.Token, false)
			if e != nil {
				return e
			}
			values := g.Map{"platform": platform, "target_type": targetType, "publish_mode": "automatic", "external_id": page.ExternalID, "name": page.Name, "dept_id": p.DeptID, "owner_id": p.ID, "provider_user": page.ProviderUser, "provider_name": page.ProviderName, "token": encrypted, "refresh_token": refresh, "expires_at": pending.ExpiresAt, "refresh_expires_at": pending.RefreshExpiry, "scopes": encode(page.Scopes), "capabilities": encode(caps), "status": "connected", "error": "", "updated_at": time.Now()}
			if platform == "facebook" {
				values["upload_token"], values["upload_token_fingerprint"] = "", ""
				if c.VideoPublishing && page.UploadToken != "" {
					uploadToken, err := growth.SocialCredential(page.UploadToken, false)
					if err != nil {
						return err
					}
					values["upload_token"] = uploadToken
					values["upload_token_fingerprint"] = authorizationFingerprint(c, targetType)
				}
			}
			if platform == "instagram" || platform == "threads" {
				values["authorization_fingerprint"] = authorizationFingerprint(c, targetType)
				values["client_business_id"] = page.BusinessID
			}
			if len(prior) > 0 {
				delete(values, "owner_id")
				delete(values, "dept_id")
				_, e = tx.Model("bm_social_accounts").Where("id", prior["id"]).Data(values).Update()
			} else {
				_, e = tx.Model("bm_social_accounts").Data(values).Insert()
			}
			if e != nil {
				return e
			}
		}
		if _, e = tx.Model("bm_social_oauth_states").Where("id", id).Data(g.Map{"status": "confirmed", "encrypted_payload": "", "error": ""}).Update(); e != nil {
			return e
		}
		return securityAudit(ctx, p, "authorization_confirmed", id)
	})
	return g.Map{"connected": len(externalIDs)}, e
}

func PurgeAuthorizationTasks(ctx context.Context) error {
	_, e := g.DB().Model("bm_social_oauth_states").Ctx(ctx).Where("expires_at<=now()").WhereIn("status", []string{"pending", "ready", "failed"}).Data(g.Map{"status": "expired", "encrypted_payload": ""}).Update()
	return e
}
