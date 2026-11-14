package social

import (
	"bytes"
	"context"
	"errors"
	"image"
	"image/draw"
	"image/jpeg"
	"io"
	"path/filepath"
	"strings"
	"time"
	"unicode/utf8"

	"billionmail-core/internal/service/governance"
	"billionmail-core/internal/service/growth"
	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/util/gconv"
)

const maxPostImageBytes = 8 * 1024 * 1024

func imageReferences(value interface{}) []string {
	raw := strings.TrimSpace(gconv.String(value))
	if raw == "" || raw == "null" || raw == "[]" {
		return []string{}
	}
	return gconv.Strings(value)
}

func checkPostImage(data []byte) (string, error) {
	c, kind, e := image.DecodeConfig(bytes.NewReader(data))
	if e != nil || c.Width < 1 || c.Height < 1 || int64(c.Width)*int64(c.Height) > 40_000_000 {
		return "", errors.New("invalid image or image exceeds 40 megapixels")
	}
	if kind != "jpeg" && kind != "png" {
		return "", errors.New("publishing images must be JPG or PNG")
	}
	if len(data) > maxPostImageBytes {
		return "", errors.New("publish image exceeds 8 MB")
	}
	if kind == "jpeg" {
		return "image/jpeg", nil
	}
	return "image/png", nil
}

func UploadPostImage(ctx context.Context, p *governance.Principal, variantID int64, name string, input io.Reader) (g.Map, error) {
	_, plan, e := variant(ctx, p, variantID, "social:content:edit")
	if e != nil {
		return nil, e
	}
	data, e := io.ReadAll(io.LimitReader(input, maxPostImageBytes+1))
	if e != nil {
		return nil, e
	}
	mime, e := checkPostImage(data)
	if e != nil {
		return nil, e
	}
	name = filepath.Base(strings.ReplaceAll(name, "\\", "/"))
	ext := ".png"
	if mime == "image/jpeg" {
		ext = ".jpg"
	}
	name = strings.TrimSuffix(name, filepath.Ext(name)) + ext
	row, e := saveAsset(ctx, p, plan["id"].Int64(), "", name, bytes.NewReader(data), true)
	if e != nil {
		return nil, e
	}
	// Existing general materials retain their separate review metadata.
	if row["status"].String() != "ready" {
		_, e = g.DB().Model("bm_social_assets").Ctx(ctx).Where("id", row["id"]).Data(g.Map{"mime": mime, "category": "product_image", "status": "ready", "extracted": encode(g.Map{"purpose": "post_image", "variant_id": variantID, "kind": "original"})}).Update()
		if e != nil {
			return nil, e
		}
	}
	return g.Map{"id": row["id"].String(), "ref": "task:" + row["id"].String(), "filename": row["filename"].String()}, nil
}

// Saving a draft may include fresh post images. Only a reviewer may approve them
// together with the exact saved variant version; publishing still requires approval.
func approveVariantImages(ctx context.Context, p *governance.Principal, id int64) error {
	return g.DB().Transaction(ctx, func(ctx context.Context, tx gdb.TX) error {
		row, plan, e := variant(ctx, p, id, "social:content:review")
		if e != nil {
			return e
		}
		if _, e = tx.GetOne(`SELECT id FROM bm_social_plans WHERE id=$1 FOR UPDATE`, plan["id"]); e != nil {
			return e
		}
		row, e = tx.GetOne(`SELECT * FROM bm_social_variants WHERE id=$1 FOR UPDATE`, id)
		if e != nil {
			return e
		}
		plan, e = tx.GetOne(`SELECT * FROM bm_social_plans WHERE id=$1`, plan["id"])
		if e != nil {
			return e
		}
		c, e := planConfig(plan)
		if e != nil {
			return e
		}
		source, e := sourceContext(ctx, p, plan, c)
		if e != nil {
			return e
		}
		if row["body"].String() == "" || row["plan_version"].Int() != plan["version"].Int() || encode(source) != encode(object(row["source_snapshot"])) {
			return errors.New("content or source changed; review the current version")
		}
		refs := imageReferences(row["images"])
		if _, e = readMedia(ctx, p, plan, refs, false); e != nil {
			return e
		}
		video, e := videoForVariant(ctx, p, plan, row["platform"].String(), row.Map(), false, true)
		if e != nil {
			return e
		}
		if video != nil {
			if _, e = tx.Model("bm_social_assets").Where("id", strings.TrimPrefix(video.ID, "task:")).Where("plan_id", plan["id"]).Where("extracted->>'purpose'=?", "post_video").Data("approved", true).Update(); e != nil {
				return e
			}
		}
		for _, ref := range refs {
			if strings.HasPrefix(ref, "task:") {
				_, e = tx.Model("bm_social_assets").Where("id", strings.TrimPrefix(ref, "task:")).Where("plan_id", plan["id"]).Where("extracted->>'purpose'=?", "post_image").Data("approved", true).Update()
				if e != nil {
					return e
				}
			}
		}
		_, e = tx.Model("bm_social_variants").Where("id", id).Data(g.Map{"state": "approved", "approved_by": p.ID, "approved_at": time.Now()}).Update()
		return e
	})
}

func PostImages(ctx context.Context, p *governance.Principal, id int64) (g.Map, error) {
	_, plan, e := variant(ctx, p, id, "social:plan:view")
	if e != nil {
		return nil, e
	}
	assets, e := g.DB().Model("bm_social_assets").Ctx(ctx).Where("plan_id", plan["id"]).Where("extracted->>'variant_id'=?", gconv.String(id)).Fields("id,filename,status,error,approved,extracted").Order("created_at DESC").Limit(100).All()
	if e != nil {
		return nil, e
	}
	candidates := gdb.Result{}
	for _, asset := range assets {
		if object(asset["extracted"])["kind"] == "ai_result" {
			candidates = append(candidates, asset)
		}
	}
	jobs, e := g.DB().Model("bm_social_jobs").Ctx(ctx).Where("plan_id", plan["id"]).Where("kind", "image_edit").Where("payload->>'variant_id'=?", gconv.String(id)).Fields("id,status,error,resource_id,payload,created_at").Order("id DESC").Limit(50).All()
	return g.Map{"candidates": candidates, "images": assets, "jobs": jobs}, e
}

func QueuePostImages(ctx context.Context, p *governance.Principal, id int64, refs []string, model, prompt, size string) (g.Map, error) {
	_, plan, e := variant(ctx, p, id, "social:content:edit")
	if e != nil {
		return nil, e
	}
	if len(refs) > 10 {
		return nil, errors.New("select at most 10 images")
	}
	generate := len(refs) == 0
	if generate && strings.TrimSpace(prompt) == "" {
		return nil, errors.New("enter image generation instructions")
	}
	if generate {
		if _, ok := growth.SocialImageEditor.(growth.ImageGenerator); !ok {
			return nil, errors.New("configured image provider does not support prompt generation")
		}
	}
	if utf8.RuneCountInString(prompt) > 3000 {
		return nil, errors.New("image instructions exceed 3000 characters")
	}
	if size != "1024x1024" && size != "1536x1024" && size != "1024x1536" {
		return nil, errors.New("select a supported image size")
	}
	if e = growth.ValidateSocialImageModel(ctx, p, model); e != nil {
		return nil, e
	}
	if _, e = readMedia(ctx, p, plan, refs, false); e != nil {
		return nil, e
	}
	for _, ref := range refs {
		if e = checkImageAIAllowed(ctx, p, ref); e != nil {
			return nil, e
		}
	}
	if generate {
		refs = []string{""}
	}
	jobs := []int64{}
	e = g.DB().Transaction(ctx, func(ctx context.Context, tx gdb.TX) error {
		if _, e := tx.GetOne(`SELECT id FROM bm_social_plans WHERE id=$1 FOR UPDATE`, plan["id"]); e != nil {
			return e
		}
		for _, ref := range refs {
			q := tx.Model("bm_social_jobs").Where("plan_id", plan["id"]).Where("kind", "image_edit").Where("resource_id", ref).WhereIn("status", []string{"queued", "running"})
			if generate {
				q = q.Where("payload->>'variant_id'=?", gconv.String(id))
			}
			n, e := q.Count()
			if e != nil {
				return e
			}
			if n > 0 {
				return errors.New("image processing is already queued; wait for its result")
			}
			job, e := queue(ctx, p, plan["id"].Int64(), 0, "image_edit", ref, g.Map{"variant_id": id, "model": model, "prompt": prompt, "size": size, "generate": generate})
			if e != nil {
				return e
			}
			jobs = append(jobs, job)
		}
		return nil
	})
	return g.Map{"jobs": jobs}, e
}

func checkImageAIAllowed(ctx context.Context, p *governance.Principal, ref string) error {
	parts := strings.SplitN(ref, ":", 2)
	if len(parts) != 2 {
		return errors.New("invalid image reference")
	}
	var row gdb.Record
	var e error
	if parts[0] == "task" {
		row, e = Asset(ctx, p, parts[1], "social:content:edit")
	} else {
		row, e = growth.Asset(ctx, p, parts[1], "material:list")
	}
	if e != nil {
		return e
	}
	if !row["ai_allowed"].Bool() {
		return errors.New("this image is not permitted for AI processing")
	}
	return nil
}

func processPostImage(ctx context.Context, p *governance.Principal, job gdb.Record) error {
	payload := object(job["payload"])
	id := gconv.Int64(payload["variant_id"])
	_, plan, e := variant(ctx, p, id, "social:content:edit")
	if e != nil {
		return e
	}
	if plan["id"].Int64() != job["plan_id"].Int64() {
		return errors.New("image job does not belong to this plan")
	}
	ref := job["resource_id"].String()
	var data []byte
	if gconv.Bool(payload["generate"]) {
		generator, ok := growth.SocialImageEditor.(growth.ImageGenerator)
		if !ok {
			return errors.New("configured image provider does not support prompt generation")
		}
		prompt := "Create a professional B2B social campaign illustration. Do not fabricate product specifications, certifications, logos or factual claims. This is an illustration, not a real product photo. Do not include text unless requested. Follow these visual instructions: " + gconv.String(payload["prompt"])
		data, e = generator.Generate(ctx, p, growth.ImageGenerateInput{Model: gconv.String(payload["model"]), Prompt: prompt, Size: gconv.String(payload["size"])})
	} else {
		if e = checkImageAIAllowed(ctx, p, ref); e != nil {
			return e
		}
		media, err := readMedia(ctx, p, plan, []string{ref}, false)
		if err != nil {
			return err
		}
		// Outputs remain separate candidates, never replacing reviewed snapshots.
		prompt := "Edit this real product photo for a consistent professional B2B social campaign. Improve framing, background, lighting and visual cleanliness. Preserve the exact product geometry, components, proportions, color, texture, labels, logos, model numbers and all functional details. Do not invent parts, markings, claims or certifications. Do not add text unless explicitly requested. Keep the whole product visible. Apply these additional visual requirements consistently, without overriding product fidelity: " + gconv.String(payload["prompt"])
		data, e = growth.SocialImageEditor.Edit(ctx, p, growth.ImageEditInput{Model: gconv.String(payload["model"]), Prompt: prompt, Size: gconv.String(payload["size"]), Filename: media[0].Filename, MIME: media[0].MIME, Image: media[0].Data})
	}
	if e != nil {
		return e
	}
	data, mime, e := compactPostImage(data)
	if e != nil {
		return e
	}
	parent := ""
	if strings.HasPrefix(ref, "task:") {
		parent = strings.TrimPrefix(ref, "task:")
	}
	// Job-specific filenames preserve candidates from earlier regeneration runs.
	ext := ".png"
	if mime == "image/jpeg" {
		ext = ".jpg"
	}
	row, e := saveAsset(ctx, p, plan["id"].Int64(), parent, "AI-"+job["id"].String()+ext, bytes.NewReader(data), true)
	if e != nil {
		return e
	}
	_, e = g.DB().Model("bm_social_assets").Ctx(ctx).Where("id", row["id"]).Data(g.Map{"mime": mime, "category": "product_image", "status": "ready", "approved": false, "extracted": encode(g.Map{"purpose": "post_image", "kind": "ai_result", "generated_from_prompt": gconv.Bool(payload["generate"]), "variant_id": id, "source_ref": ref, "model": payload["model"], "prompt": payload["prompt"], "size": payload["size"], "job_id": job["id"].Int64()})}).Update()
	return e
}

// Large PNG results can be safely encoded as JPG after compositing transparent
// pixels on white. This utility is shared by the provider result validation.
func compactPostImage(data []byte) ([]byte, string, error) {
	if mime, e := checkPostImage(data); e == nil {
		return data, mime, nil
	}
	c, _, e := image.DecodeConfig(bytes.NewReader(data))
	if e != nil || int64(c.Width)*int64(c.Height) > 40_000_000 {
		return nil, "", errors.New("invalid AI image dimensions")
	}
	decoded, _, e := image.Decode(bytes.NewReader(data))
	if e != nil {
		return nil, "", errors.New("invalid AI image")
	}
	canvas := image.NewRGBA(decoded.Bounds())
	draw.Draw(canvas, canvas.Bounds(), image.White, image.Point{}, draw.Src)
	draw.Draw(canvas, canvas.Bounds(), decoded, decoded.Bounds().Min, draw.Over)
	var out bytes.Buffer
	if e = jpeg.Encode(&out, canvas, &jpeg.Options{Quality: 90}); e != nil {
		return nil, "", e
	}
	mime, e := checkPostImage(out.Bytes())
	return out.Bytes(), mime, e
}
