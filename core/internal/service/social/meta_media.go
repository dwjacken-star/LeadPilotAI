package social

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"billionmail-core/internal/service/growth"
	"github.com/gogf/gf/v2/frame/g"
)

func mediaSigningKey() ([]byte, error) {
	key, err := os.ReadFile(filepath.Join(growth.StorageRoot(), ".ai-master-key"))
	if err != nil || len(key) != 32 {
		return nil, errors.New("existing media signing key unavailable")
	}
	return key, nil
}

func mediaSignature(key []byte, id int64, index int, expiry int64, digest string) string {
	mac := hmac.New(sha256.New, key)
	fmt.Fprintf(mac, "social-media-v1:%d:%d:%d:%s", id, index, expiry, digest)
	return hex.EncodeToString(mac.Sum(nil))
}

func deliveryMediaURLs(ctx context.Context, id int64, media []MediaFile) ([]string, error) {
	c, err := config(ctx)
	if err != nil {
		return nil, err
	}
	if !validMediaBase(c.MediaFetchURL) || !c.MediaFetchConfirmed {
		return nil, errors.New("verified signed media endpoint required")
	}
	key, err := mediaSigningKey()
	if err != nil {
		return nil, err
	}
	expiry := time.Now().Add(2 * time.Hour).Unix()
	out := []string{}
	for index, item := range media {
		digest := mediaDigest(item)
		query := url.Values{"delivery": {strconv.FormatInt(id, 10)}, "index": {strconv.Itoa(index)}, "expires": {strconv.FormatInt(expiry, 10)}, "digest": {digest}, "signature": {mediaSignature(key, id, index, expiry, digest)}}
		out = append(out, c.MediaFetchURL+"?"+query.Encode())
	}
	return out, nil
}

// Bearer fetch capability only for an exact, approved delivery image. The
// account/actor/plan are checked again; this is not a public asset endpoint.
func SignedDeliveryMedia(ctx context.Context, id int64, index int, expiry int64, digest, signature string) (MediaFile, error) {
	denied := errors.New("media fetch unavailable")
	if id <= 0 || index < 0 || index >= 10 || expiry < time.Now().Unix() || expiry > time.Now().Add(2*time.Hour+time.Minute).Unix() || len(digest) != 64 || len(signature) != 64 {
		return MediaFile{}, denied
	}
	key, err := mediaSigningKey()
	if err != nil {
		return MediaFile{}, denied
	}
	expected := mediaSignature(key, id, index, expiry, digest)
	if !hmac.Equal([]byte(expected), []byte(signature)) {
		return MediaFile{}, denied
	}
	row, err := g.DB().Model("bm_social_deliveries d").Ctx(ctx).LeftJoin("bm_social_plans p", "p.id=d.plan_id").Where("d.id", id).WhereNull("d.deleted_at").Fields("d.*,p.dept_id").One()
	if err != nil || len(row) == 0 || (row["status"].String() != "publishing" && row["status"].String() != "retry") {
		return MediaFile{}, denied
	}
	p, err := workerUser(ctx, row["publisher_id"].Int64(), row["dept_id"].Int64(), "social:publish")
	if err != nil {
		return MediaFile{}, denied
	}
	account, err := accountAccess(ctx, p, row["account_id"].Int64(), "social:publish", true)
	if err != nil {
		return MediaFile{}, denied
	}
	if err = validatePublishingAccount(ctx, account); err != nil {
		return MediaFile{}, denied
	}
	if _, err = requirePlatform(ctx, account["platform"].String(), true); err != nil {
		return MediaFile{}, denied
	}
	plan, err := scoped(ctx, p, "bm_social_plans", row["plan_id"].Int64(), "social:publish")
	if err != nil || plan["status"].String() != "scheduled" {
		return MediaFile{}, denied
	}
	refs := imageReferences(object(row["snapshot"])["images"])
	snapshot := object(row["snapshot"])
	if ref, ok := snapshot["video"].(string); ok && ref != "" {
		if index != 0 || len(refs) > 0 || requireVideoAccount(ctx, account) != nil {
			return MediaFile{}, denied
		}
		file, e := readVideo(ctx, p, plan, ref, true, false)
		if e != nil || mediaDigest(*file) != digest || encode(snapshot["media_manifest"]) != encode(imageDigests([]MediaFile{*file})) {
			return MediaFile{}, denied
		}
		return *file, nil
	}
	if index >= len(refs) {
		return MediaFile{}, denied
	}
	media, err := Media(ctx, p, plan, []string{refs[index]})
	if err != nil || len(media) != 1 || hashString(string(media[0].Data)) != digest {
		return MediaFile{}, denied
	}
	return media[0], nil
}
