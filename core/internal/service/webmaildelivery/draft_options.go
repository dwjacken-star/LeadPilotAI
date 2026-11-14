package webmaildelivery

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/util/gconv"
	"regexp"
)

var draftKeyPattern = regexp.MustCompile(`^[cd]:[a-zA-Z0-9_-]{1,120}$`)

func DraftOptions(ctx context.Context, mailbox, key string) (g.Map, error) {
	if !draftKeyPattern.MatchString(key) {
		return nil, nil
	}
	v, e := g.DB().Model("bm_webmail_draft_options").Ctx(ctx).Where("mailbox", mailbox).Where("draft_key", key).Value("options")
	if e != nil {
		return nil, e
	}
	if v.IsEmpty() {
		return nil, nil
	}
	var options g.Map
	e = json.Unmarshal(v.Bytes(), &options)
	return options, e
}
func SaveDraftOptions(ctx context.Context, mailbox string, data g.Map) (g.Map, error) {
	key := gconv.String(data["draft_key"])
	if !draftKeyPattern.MatchString(key) {
		return nil, errors.New("invalid draft identifier")
	}
	options := gconv.Map(data["options"])
	clean := g.Map{}
	for _, k := range []string{"unsubscribe", "track_click", "track_open", "warmup"} {
		clean[k] = gconv.Bool(options[k])
	}
	raw, e := json.Marshal(clean)
	if e != nil {
		return nil, e
	}
	keys := []string{key}
	if uid := gconv.String(data["draft_uid"]); uid != "" {
		if !draftKeyPattern.MatchString("d:" + uid) {
			return nil, errors.New("invalid draft UID")
		}
		keys = append(keys, "d:"+uid)
	}
	for _, k := range keys {
		if _, e = g.DB().Exec(ctx, `INSERT INTO bm_webmail_draft_options(mailbox,draft_key,options) VALUES($1,$2,$3) ON CONFLICT(mailbox,draft_key) DO UPDATE SET options=excluded.options,updated_at=now()`, mailbox, k, string(raw)); e != nil {
			return nil, e
		}
	}
	return g.Map{"saved": true}, nil
}
