package social

import (
	"context"
	"os"
	"strings"
	"testing"

	"billionmail-core/internal/service/governance"
	"billionmail-core/internal/service/growth"
	"billionmail-core/internal/testsupport"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/util/gconv"
	"github.com/google/uuid"
)

type fixtureImageGenerator struct {
	fixtureImageEditor
	generated int
}

func (f *fixtureImageGenerator) Generate(ctx context.Context, p *governance.Principal, in growth.ImageGenerateInput) ([]byte, error) {
	f.generated++
	if !strings.Contains(in.Prompt, "not a real product photo") || !strings.Contains(in.Prompt, "campaign background") || in.Size != "1024x1024" {
		f.t.Fatal("generation instructions or illustration safeguards lost")
	}
	return f.data, nil
}

func TestPromptImagesReuseReviewedCandidates(t *testing.T) {
	testsupport.Database(t)
	ctx := context.Background()
	p, e := governance.Load(ctx, 1)
	must(t, e)
	root := ".social-image-generation-test-" + uuid.NewString()
	t.Setenv("MAPLEMAIL_GROWTH_DATA", root)
	t.Cleanup(func() { _ = os.RemoveAll(root) })
	company, e := growth.SaveCompany(ctx, p, 0, g.Map{"name": "Prompt image fixture", "profile": g.Map{"business": "Automotive parts"}})
	must(t, e)
	must(t, growth.PublishCompany(ctx, p, company))
	c := PlanConfig{Platforms: []string{"facebook"}, Markets: []Market{{Country: "US", Language: "en", Timezone: "UTC", LocalTime: "10:22:37"}}, Cadence: "once", DailyPosts: 3, MinIntervalMinutes: 5}
	id, e := SavePlan(ctx, p, 0, "Prompt image plan", company, c)
	must(t, e)
	must(t, ManualVariants(ctx, p, id))
	v, e := g.DB().Model("bm_social_variants").Where("plan_id", id).Value("id")
	must(t, e)
	must(t, SaveVariant(ctx, p, v.Int64(), g.Map{"body": "Reviewed campaign body"}))
	must(t, ApproveVariant(ctx, p, v.Int64()))
	old := growth.SocialImageEditor
	t.Cleanup(func() { growth.SocialImageEditor = old })
	fake := &fixtureImageGenerator{fixtureImageEditor: fixtureImageEditor{t: t, data: postImageFixture(t, 99)}}
	growth.SocialImageEditor = fake
	if _, e = QueuePostImages(ctx, p, v.Int64(), nil, "fixture-image", "", "1024x1024"); e == nil {
		t.Fatal("empty prompt generation accepted")
	}
	queued, e := QueuePostImages(ctx, p, v.Int64(), nil, "fixture-image", "campaign background", "1024x1024")
	must(t, e)
	if _, e = QueuePostImages(ctx, p, v.Int64(), nil, "fixture-image", "campaign background", "1024x1024"); e == nil {
		t.Fatal("duplicate active prompt generation accepted")
	}
	jobID := gconv.Int64s(queued["jobs"])[0]
	worked, e := ProcessJob(ctx)
	must(t, e)
	if !worked || fake.generated != 1 || fake.calls != 0 {
		t.Fatal("prompt generation did not reuse the existing image worker")
	}
	job, e := g.DB().Model("bm_social_jobs").Where("id", jobID).One()
	must(t, e)
	if job["status"].String() != "completed" {
		t.Fatal("prompt generation job failed", job["error"])
	}
	result, e := PostImages(ctx, p, v.Int64())
	must(t, e)
	candidates := gconv.Maps(result["candidates"])
	if len(candidates) != 1 || gconv.Bool(candidates[0]["approved"]) {
		t.Fatal("generated image did not remain a review candidate")
	}
	current, e := g.DB().Model("bm_social_variants").Where("id", v).One()
	must(t, e)
	if current["state"].String() != "approved" || len(imageReferences(current["images"])) != 0 {
		t.Fatal("prompt generation silently changed approved content")
	}
	ref := "task:" + gconv.String(candidates[0]["id"])
	plan, e := scoped(ctx, p, "bm_social_plans", id, "social:publish")
	must(t, e)
	if _, e = Media(ctx, p, plan, []string{ref}); e == nil {
		t.Fatal("generated image was publishable before selection and review")
	}
	must(t, SaveVariant(ctx, p, v.Int64(), g.Map{"body": "Reviewed campaign body", "images": []string{ref}}))
	must(t, ApproveVariant(ctx, p, v.Int64()))
	_, e = Media(ctx, p, plan, []string{ref})
	must(t, e)
}
