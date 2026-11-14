package social

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"sort"

	"billionmail-core/internal/service/growth"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/util/gconv"
)

var facebookPageID = regexp.MustCompile(`^[0-9]{1,32}$`)

// Only explicitly granted Page IDs from Meta's validated token introspection
// may supplement /me/accounts. No arbitrary Page ID or public search is used.
func facebookPageTargets(data g.Map, appID, userID string, scopes []string) (map[string][]string, error) {
	if !gconv.Bool(data["is_valid"]) || gconv.String(data["app_id"]) != appID || gconv.String(data["user_id"]) != userID {
		return nil, errors.New("Facebook token identity does not match this application and user")
	}
	if !has(scopes, "pages_manage_posts") {
		return nil, errors.New("Facebook has not granted pages_manage_posts")
	}
	grants := map[string]map[string]bool{}
	restrictedScopes := map[string]bool{}
	for _, entry := range gconv.Maps(data["granular_scopes"]) {
		scope := gconv.String(entry["scope"])
		if !has(scopes, scope) {
			continue
		}
		restrictedScopes[scope] = true
		for _, id := range gconv.Strings(entry["target_ids"]) {
			if !facebookPageID.MatchString(id) {
				continue
			}
			if grants[id] == nil {
				grants[id] = map[string]bool{}
			}
			grants[id][scope] = true
		}
	}
	result := map[string][]string{}
	for id, granted := range grants {
		if !granted["pages_manage_posts"] || (!granted["pages_show_list"] && !granted["pages_read_engagement"]) {
			continue
		}
		for _, scope := range scopes {
			// Page capabilities are limited to the grants for this particular Page.
			if granted[scope] || !restrictedScopes[scope] {
				result[id] = append(result[id], scope)
			}
		}
	}
	if len(result) > 50 {
		return nil, errors.New("more than 50 explicitly granted Pages; reconnect a smaller selection")
	}
	return result, nil
}

func facebookInspectToken(ctx context.Context, c PlatformConfig, appToken, token string) (g.Map, error) {
	if e := reserveAPI(ctx, "facebook"); e != nil {
		return nil, e
	}
	u := "https://graph.facebook.com/" + c.Version + "/debug_token?" + url.Values{"input_token": {token}}.Encode()
	req, e := http.NewRequestWithContext(ctx, "GET", u, nil)
	if e != nil {
		return nil, e
	}
	req.Header.Set("Authorization", "Bearer "+appToken)
	data, _, e := execute(req, false)
	if e != nil {
		return nil, e
	}
	return gconv.Map(data["data"]), nil
}

func facebookGranularPages(ctx context.Context, token, userID string, scopes []string) ([]ConnectedPage, error) {
	c, e := requirePlatform(ctx, "facebook", false)
	if e != nil {
		return nil, e
	}
	secret, e := growth.SocialCredential(c.ClientSecret, true)
	if e != nil {
		return nil, errors.New("Facebook application credential unavailable")
	}
	appToken := c.ClientID + "|" + secret
	info, e := facebookInspectToken(ctx, c, appToken, token)
	if e != nil {
		return nil, fmt.Errorf("Facebook token grants: %w", e)
	}
	targets, e := facebookPageTargets(info, c.ClientID, userID, scopes)
	if e != nil {
		return nil, e
	}
	ids := []string{}
	for id := range targets {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	if len(ids) == 0 {
		return nil, errors.New("Meta token has no explicitly granted publishing Page; reconnect and select the company Page, and check the Page access role")
	}
	pages := []ConnectedPage{}
	for _, id := range ids {
		row, _, e := request(ctx, "facebook", token, "GET", id+"?fields=id,name,access_token", nil, false)
		if e != nil {
			return nil, fmt.Errorf("Facebook explicitly granted Page: %w", e)
		}
		pageToken := gconv.String(row["access_token"])
		if gconv.String(row["id"]) != id || pageToken == "" {
			return nil, errors.New("Facebook did not issue a token for the explicitly granted Page; check Page access role")
		}
		pageInfo, e := facebookInspectToken(ctx, c, appToken, pageToken)
		if e != nil {
			return nil, e
		}
		profileID := gconv.String(pageInfo["profile_id"])
		if !gconv.Bool(pageInfo["is_valid"]) || gconv.String(pageInfo["app_id"]) != c.ClientID || gconv.String(pageInfo["type"]) != "PAGE" || (profileID != "" && profileID != id) {
			return nil, errors.New("Facebook returned an invalid or mismatched Page token")
		}
		granted := []string{}
		for _, scope := range targets[id] {
			if has(gconv.Strings(pageInfo["scopes"]), scope) {
				granted = append(granted, scope)
			}
		}
		if !has(granted, "pages_manage_posts") {
			return nil, errors.New("Facebook Page token has no content publishing permission")
		}
		pages = append(pages, ConnectedPage{ExternalID: id, Name: gconv.String(row["name"]), Token: pageToken, ProviderUser: userID, Scopes: granted})
	}
	return pages, nil
}
