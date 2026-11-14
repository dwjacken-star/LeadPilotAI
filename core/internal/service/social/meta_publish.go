package social

import (
	"context"
	"errors"
	"net/url"
	"strings"
	"time"

	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/util/gconv"
)

type metaProgress struct {
	Fingerprint   string   `json:"fingerprint"`
	Children      []string `json:"children,omitempty"`
	ReadyChildren int      `json:"ready_children"`
	AIGenerated   bool     `json:"ai_generated"`
	Container     string   `json:"container,omitempty"`
	RemoteID      string   `json:"remote_id,omitempty"`
	Pending       string   `json:"pending,omitempty"`
	Polls         int      `json:"polls"`
	CreatedAt     int64    `json:"created_at"`
	MediaType     string   `json:"media_type,omitempty"`
	UploadSession string   `json:"upload_session,omitempty"`
	UploadURL     string   `json:"upload_url,omitempty"`
	FileHandle    string   `json:"file_handle,omitempty"`
	VideoID       string   `json:"video_id,omitempty"`
	Offset        int64    `json:"offset"`
	Uploaded      bool     `json:"uploaded"`
	FinishSent    bool     `json:"finish_sent"`
}

type metaCall func(method, path string, body g.Map, mutation bool) (g.Map, error)

func processing() error { return &RemoteError{Kind: "media_processing", RetryAfter: time.Minute} }

// Each remote mutation is durably fenced before submission. A process stopping
// between submission and receipt persistence leaves Pending set, never a retry.
func advanceMeta(platform, owner, text string, mediaURLs []string, progress *metaProgress, save func() error, call metaCall) (PublishResult, error) {
	if platform != "instagram" && platform != "threads" {
		return PublishResult{}, errors.New("invalid container publishing platform")
	}
	if progress.Pending != "" {
		return PublishResult{}, &RemoteError{Kind: "unresolved_provider_operation", Unknown: true}
	}
	if progress.RemoteID != "" {
		return PublishResult{ID: progress.RemoteID, State: "published"}, nil
	}
	if progress.CreatedAt == 0 {
		progress.CreatedAt = time.Now().Unix()
		if err := save(); err != nil {
			return PublishResult{}, err
		}
	}
	if time.Now().Unix()-progress.CreatedAt > 23*60*60 {
		return PublishResult{}, errors.New("media operation expired; review before scheduling a new delivery")
	}
	createPath, publishPath, textKey, statusKey := owner+"/media", owner+"/media_publish", "caption", "status_code"
	if platform == "threads" {
		createPath, publishPath, textKey, statusKey = owner+"/threads", owner+"/threads_publish", "text", "status"
	}
	mutate := func(action, path string, payload g.Map) (g.Map, error) {
		progress.Pending = action
		if err := save(); err != nil {
			return nil, err
		}
		result, err := call("POST", path, payload, true)
		if err != nil {
			var remote *RemoteError
			if errors.As(err, &remote) && !remote.Unknown && remote.Status >= 400 {
				progress.Pending = ""
				if saveErr := save(); saveErr != nil {
					return nil, &RemoteError{Kind: "provider_receipt_storage_failed", Unknown: true}
				}
				return nil, err
			}
			return nil, &RemoteError{Kind: "provider_mutation_outcome_unknown", Unknown: true}
		}
		if gconv.String(result["id"]) == "" {
			return nil, &RemoteError{Kind: "missing_container_or_media_id", Unknown: true}
		}
		progress.Pending = ""
		return result, nil
	}
	if progress.Container == "" {
		if len(mediaURLs) > 1 {
			if len(progress.Children) < len(mediaURLs) {
				payload := g.Map{"image_url": mediaURLs[len(progress.Children)], "is_carousel_item": true}
				if platform == "threads" {
					payload["media_type"] = "IMAGE"
				}
				result, err := mutate("create_child", createPath, payload)
				if err != nil {
					return PublishResult{}, err
				}
				progress.Children = append(progress.Children, gconv.String(result["id"]))
				if err = save(); err != nil {
					return PublishResult{}, &RemoteError{Kind: "container_receipt_storage_failed", Unknown: true}
				}
				return PublishResult{}, processing()
			}
			if progress.ReadyChildren < len(progress.Children) {
				child := progress.Children[progress.ReadyChildren]
				result, err := call("GET", url.PathEscape(child)+"?fields="+statusKey, nil, false)
				if err != nil {
					return PublishResult{}, err
				}
				state := gconv.String(result[statusKey])
				if state == "ERROR" || state == "EXPIRED" {
					return PublishResult{}, errors.New("carousel child processing failed or expired")
				}
				if state != "FINISHED" {
					progress.Polls++
					if err = save(); err != nil {
						return PublishResult{}, err
					}
					if progress.Polls >= 10 {
						return PublishResult{}, errors.New("media processing polling limit reached")
					}
					return PublishResult{}, processing()
				}
				progress.ReadyChildren++
				if err = save(); err != nil {
					return PublishResult{}, err
				}
				if progress.ReadyChildren < len(progress.Children) {
					return PublishResult{}, processing()
				}
			}
		}
		payload := g.Map{textKey: text}
		if progress.AIGenerated {
			if platform == "instagram" {
				payload["is_ai_generated"] = true
			} else {
				payload["is_made_with_ai"] = true
			}
		}
		if len(mediaURLs) > 1 {
			payload["media_type"] = "CAROUSEL"
			payload["children"] = strings.Join(progress.Children, ",")
		} else if len(mediaURLs) == 1 {
			if progress.MediaType == "reels" || progress.MediaType == "video" {
				payload["video_url"] = mediaURLs[0]
				payload["media_type"] = "REELS"
				if platform == "threads" {
					payload["media_type"] = "VIDEO"
				}
			} else {
				payload["image_url"] = mediaURLs[0]
				if platform == "threads" {
					payload["media_type"] = "IMAGE"
				}
			}
		} else {
			if platform != "threads" {
				return PublishResult{}, errors.New("Instagram cannot publish text only")
			}
			payload["media_type"] = "TEXT"
		}
		result, err := mutate("create_container", createPath, payload)
		if err != nil {
			return PublishResult{}, err
		}
		progress.Container = gconv.String(result["id"])
		if err = save(); err != nil {
			return PublishResult{}, &RemoteError{Kind: "container_receipt_storage_failed", Unknown: true}
		}
		return PublishResult{}, processing()
	}
	status, err := call("GET", url.PathEscape(progress.Container)+"?fields="+statusKey, nil, false)
	if err != nil {
		return PublishResult{}, err
	}
	switch gconv.String(status[statusKey]) {
	case "ERROR", "EXPIRED":
		return PublishResult{}, errors.New("media container failed or expired")
	case "PUBLISHED":
		// The provider confirms a publish but not its post ID. Do not invent a
		// receipt or issue another final mutation.
		return PublishResult{}, &RemoteError{Kind: "published_container_receipt_requires_reconciliation", Unknown: true}
	case "FINISHED":
	default:
		progress.Polls++
		if err = save(); err != nil {
			return PublishResult{}, err
		}
		if progress.Polls >= 10 {
			return PublishResult{}, errors.New("media processing polling limit reached")
		}
		return PublishResult{}, processing()
	}
	result, err := mutate("publish_container", publishPath, g.Map{"creation_id": progress.Container})
	if err != nil {
		return PublishResult{}, err
	}
	progress.RemoteID = gconv.String(result["id"])
	if err = save(); err != nil {
		return PublishResult{}, &RemoteError{Kind: "publish_receipt_storage_failed", Unknown: true}
	}
	return PublishResult{ID: progress.RemoteID, State: "published"}, nil
}

func publishMetaDelivery(ctx context.Context, deliveryID int64, platform, token, owner string, in PublishInput, mediaURLs []string) (PublishResult, error) {
	if err := validateMetaContent(platform, in.Body, in.Link, in.Images, in.Video); err != nil {
		return PublishResult{}, err
	}
	media := append([]MediaFile{}, in.Images...)
	if in.Video != nil {
		if err := validateVideo(platform, in.MediaType, in.Video.Video); err != nil {
			return PublishResult{}, err
		}
		media = append(media, *in.Video)
	}
	// Deliberately separate from the caller's account-lane transaction: provider
	// receipts must survive a rollback of the outer publishing transaction.
	storeCtx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	row, err := g.DB().Model("bm_social_provider_operations").Ctx(storeCtx).Where("delivery_id", deliveryID).One()
	if err != nil {
		return PublishResult{}, err
	}
	progress := metaProgress{}
	if len(row) > 0 {
		if err = gconv.Scan(object(row["progress"]), &progress); err != nil {
			return PublishResult{}, err
		}
	}
	fingerprint := hashString(encode(g.Map{"platform": platform, "owner": owner, "body": in.Body, "link": in.Link, "images": imageDigests(media), "media_type": in.MediaType}))
	if progress.Fingerprint != "" && progress.Fingerprint != fingerprint {
		return PublishResult{}, errors.New("immutable publishing media changed; review and reschedule")
	}
	progress.Fingerprint = fingerprint
	progress.MediaType = in.MediaType
	for _, item := range media {
		if item.AIGenerated {
			progress.AIGenerated = true
		}
	}
	save := func() error {
		_, err := g.DB().Model("bm_social_provider_operations").Ctx(storeCtx).Data(g.Map{"delivery_id": deliveryID, "progress": encode(progress), "updated_at": time.Now()}).OnConflict("delivery_id").Save()
		return err
	}
	text := in.Body
	if in.Link != "" {
		text += "\n\n" + in.Link
	}
	return advanceMeta(platform, owner, text, mediaURLs, &progress, save, func(method, path string, body g.Map, mutation bool) (g.Map, error) {
		if mutation && (strings.HasSuffix(path, "/media_publish") || strings.HasSuffix(path, "/threads_publish")) {
			if err := validateMetaRemoteTarget(ctx, platform, token, owner); err != nil {
				return nil, &RemoteError{Kind: "authorization_revalidation_failed", Status: 403}
			}
		}
		result, _, err := request(ctx, platform, token, method, path, body, mutation)
		return result, err
	})
}

func imageDigests(media []MediaFile) []g.Map {
	out := []g.Map{}
	for _, item := range media {
		entry := g.Map{"id": item.ID, "hash": mediaDigest(item), "ai_generated": item.AIGenerated}
		if item.Video != nil {
			entry["video_manifest"] = item.Video
			entry["size"] = item.Size
		}
		out = append(out, entry)
	}
	return out
}
