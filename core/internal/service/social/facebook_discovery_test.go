package social

import (
	"github.com/gogf/gf/v2/frame/g"
	"testing"
)

func TestFacebookGranularPageTargets(t *testing.T) {
	scopes := []string{"pages_manage_posts", "pages_show_list", "pages_read_engagement", "pages_read_user_content", "read_insights"}
	data := g.Map{"is_valid": true, "app_id": "app", "user_id": "user", "granular_scopes": []g.Map{
		{"scope": "pages_manage_posts", "target_ids": []string{"123", "456", "not-a-page"}},
		{"scope": "pages_show_list", "target_ids": []string{"123", "789"}},
		{"scope": "pages_read_user_content", "target_ids": []string{"789"}},
	}}
	pages, e := facebookPageTargets(data, "app", "user", scopes)
	if e != nil || len(pages) != 1 || !has(pages["123"], "pages_manage_posts") {
		t.Fatalf("explicit Page grants lost: %v %v", pages, e)
	}
	if !has(pages["123"], "read_insights") || has(pages["123"], "pages_read_user_content") {
		t.Fatal("per-Page and global permissions were not distinguished")
	}
	for _, bad := range []struct{ app, user string }{{"other", "user"}, {"app", "other"}} {
		if _, e := facebookPageTargets(data, bad.app, bad.user, scopes); e == nil {
			t.Fatal("token identity mismatch accepted")
		}
	}
	if _, e := facebookPageTargets(data, "app", "user", []string{"pages_show_list"}); e == nil {
		t.Fatal("ungranted publishing permission accepted")
	}
	data["is_valid"] = false
	if _, e := facebookPageTargets(data, "app", "user", scopes); e == nil {
		t.Fatal("expired token accepted")
	}
}
