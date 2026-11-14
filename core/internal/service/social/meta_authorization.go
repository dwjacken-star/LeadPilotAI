package social

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"time"

	"billionmail-core/internal/service/growth"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/util/gconv"
)

func validateMetaRemoteTarget(ctx context.Context, platform, token, owner string) error {
	c, err := requirePlatform(ctx, platform, true)
	if err != nil {
		return err
	}
	if platform == "instagram" {
		secret, err := growth.SocialCredential(c.ClientSecret, true)
		if err != nil {
			return errors.New("application credential unavailable")
		}
		pages, _, err := completeMetaAuthorization(ctx, platform, c, secret, token, g.Map{})
		if err != nil {
			return err
		}
		for _, page := range pages {
			if page.ExternalID == owner {
				return nil
			}
		}
		return errors.New("Instagram account is no longer assigned to this authorization")
	}
	perms, _, err := request(ctx, platform, token, "GET", "me/permissions", nil, false)
	if err != nil {
		return err
	}
	scopes := []string{}
	for _, permission := range gconv.Maps(perms["data"]) {
		if permission["status"] == "granted" {
			scopes = append(scopes, gconv.String(permission["permission"]))
		}
	}
	if !gconv.Bool(capabilities(platform, scopes)["publish"]) {
		return errors.New("Threads publishing authorization was revoked")
	}
	me, _, err := request(ctx, platform, token, "GET", "me?fields=id", nil, false)
	if err != nil {
		return err
	}
	if gconv.String(me["id"]) != owner {
		return errors.New("Threads authorization target mismatch")
	}
	return nil
}

func completeMetaAuthorization(ctx context.Context, platform string, c PlatformConfig, secret, token string, response g.Map) ([]ConnectedPage, g.Map, error) {
	if platform == "threads" {
		if err := reserveAPI(ctx, platform); err != nil {
			return nil, response, err
		}
		req, err := http.NewRequestWithContext(ctx, "GET", "https://graph.threads.com/access_token?"+url.Values{"grant_type": {"th_exchange_token"}, "client_secret": {secret}, "access_token": {token}}.Encode(), nil)
		if err != nil {
			return nil, response, err
		}
		extended, _, err := execute(req, false)
		if err != nil {
			return nil, response, err
		}
		token = gconv.String(extended["access_token"])
		if token == "" {
			return nil, response, errors.New("Threads returned no long-lived token")
		}
		response = extended
	}
	scopes := []string{}
	if platform == "instagram" {
		if err := reserveAPI(ctx, platform); err != nil {
			return nil, response, err
		}
		req, _ := http.NewRequestWithContext(ctx, "GET", endpoint(platform, c, "debug_token?"+url.Values{"input_token": {token}}.Encode()), nil)
		req.Header.Set("Authorization", "Bearer "+c.ClientID+"|"+secret)
		info, _, err := execute(req, false)
		if err != nil {
			return nil, response, err
		}
		data := gconv.Map(info["data"])
		if !gconv.Bool(data["is_valid"]) || gconv.String(data["app_id"]) != c.ClientID {
			return nil, response, errors.New("Instagram token application mismatch or invalid authorization")
		}
		scopes = gconv.Strings(data["scopes"])
		if expiry := gconv.Int64(data["expires_at"]); expiry > 0 {
			seconds := expiry - time.Now().Unix()
			if seconds <= 0 {
				return nil, response, errors.New("Instagram authorization expired")
			}
			response["expires_in"] = seconds
		}
	} else {
		perms, _, err := request(ctx, platform, token, "GET", "me/permissions", nil, false)
		if err != nil {
			return nil, response, err
		}
		for _, permission := range gconv.Maps(perms["data"]) {
			if permission["status"] == "granted" {
				scopes = append(scopes, gconv.String(permission["permission"]))
			}
		}
	}
	if !gconv.Bool(capabilities(platform, scopes)["publish"]) {
		return nil, response, errors.New("provider did not grant the required publishing permissions")
	}
	if platform == "threads" {
		me, _, err := request(ctx, platform, token, "GET", "me?fields=id,username", nil, false)
		if err != nil {
			return nil, response, err
		}
		id := gconv.String(me["id"])
		if !facebookPageID.MatchString(id) {
			return nil, response, errors.New("Threads identity unavailable")
		}
		name := str(gconv.String(me["username"]), 200)
		return []ConnectedPage{{ExternalID: id, Name: name, Token: token, ProviderUser: id, ProviderName: name, Scopes: scopes}}, response, nil
	}
	me, _, err := request(ctx, platform, token, "GET", "me?fields=id,client_business_id", nil, false)
	if err != nil {
		return nil, response, err
	}
	business, identity := gconv.String(me["client_business_id"]), gconv.String(me["id"])
	if business == "" || identity == "" {
		return nil, response, errors.New("Unified Login business identity unavailable; verify selected configuration")
	}
	pages := []ConnectedPage{}
	cursor := ""
	for batch := 0; batch < 3; batch++ {
		query := url.Values{"fields": {"id,username"}, "limit": {"30"}}
		if cursor != "" {
			query.Set("after", cursor)
		}
		list, _, err := request(ctx, platform, token, "GET", "me/assigned_instagram_accounts?"+query.Encode(), nil, false)
		if err != nil {
			return nil, response, err
		}
		for _, entry := range gconv.Maps(list["data"]) {
			id := gconv.String(entry["id"])
			if !facebookPageID.MatchString(id) {
				return nil, response, errors.New("invalid Instagram account identifier")
			}
			pages = append(pages, ConnectedPage{ExternalID: id, Name: str(gconv.String(entry["username"]), 200), Token: token, ProviderUser: identity, ProviderName: "Business integration", BusinessID: business, Scopes: scopes})
		}
		paging := gconv.Map(list["paging"])
		if paging["next"] == nil {
			if len(pages) == 0 || len(pages) > 30 {
				return nil, response, errors.New("select 1-30 Instagram professional accounts")
			}
			return pages, response, nil
		}
		next := gconv.String(gconv.Map(paging["cursors"])["after"])
		if next == "" || next == cursor {
			return nil, response, errors.New("invalid Instagram discovery pagination")
		}
		cursor = next
	}
	return nil, response, errors.New("too many Instagram accounts; reconnect a smaller selection")
}

func readMetaPost(ctx context.Context, platform, token, id, owner string) (PublishResult, error) {
	fields := "id,owner,permalink"
	data, _, err := request(ctx, platform, token, "GET", url.PathEscape(id)+"?fields="+fields, nil, false)
	if err != nil {
		return PublishResult{}, err
	}
	if gconv.String(gconv.Map(data["owner"])["id"]) != owner {
		return PublishResult{}, errors.New("Meta post owner mismatch or unavailable")
	}
	if gconv.String(data["id"]) != id {
		return PublishResult{}, errors.New("published media identity mismatch")
	}
	return PublishResult{ID: id, URL: gconv.String(data["permalink"]), State: "published"}, nil
}
