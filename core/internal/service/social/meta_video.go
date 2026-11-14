package social

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"billionmail-core/internal/service/governance"
	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/util/gconv"
)

type VideoManifest struct {
	Width         int     `json:"width"`
	Height        int     `json:"height"`
	FPS           float64 `json:"fps"`
	Duration      float64 `json:"duration"`
	Codec         string  `json:"codec"`
	PixelFormat   string  `json:"pixel_format"`
	Bitrate       int64   `json:"bitrate"`
	PeakBitrate   int64   `json:"peak_bitrate"`
	AudioCodec    string  `json:"audio_codec"`
	AudioRate     int     `json:"audio_rate"`
	AudioChannels int     `json:"audio_channels"`
	AudioBitrate  int64   `json:"audio_bitrate"`
	ClosedGOP     bool    `json:"closed_gop"`
	FixedRate     bool    `json:"fixed_rate"`
	FastStart     bool    `json:"fast_start"`
}

type probeBuffer struct{ bytes.Buffer }

func (b *probeBuffer) Write(p []byte) (int, error) {
	if b.Len()+len(p) > 8*1024*1024 {
		return 0, errors.New("video inspection output limit")
	}
	return b.Buffer.Write(p)
}

func rational(raw string) float64 {
	parts := strings.Split(raw, "/")
	if len(parts) != 2 {
		return 0
	}
	a, e := strconv.ParseFloat(parts[0], 64)
	if e != nil {
		return 0
	}
	b, e := strconv.ParseFloat(parts[1], 64)
	if e != nil || b <= 0 {
		return 0
	}
	return a / b
}

func sha256File(reader io.Reader) string {
	h := sha256.New()
	n, e := io.Copy(h, io.LimitReader(reader, MaxUploadBytes+1))
	if e != nil || n <= 0 || n > MaxUploadBytes {
		return ""
	}
	return hex.EncodeToString(h.Sum(nil))
}

func mediaDigest(item MediaFile) string {
	if item.Video != nil {
		return item.SHA256
	}
	return hashString(string(item.Data))
}

func parseVideoProbe(data []byte) (*VideoManifest, error) {
	var raw struct {
		Streams []struct {
			Index      int    `json:"index"`
			Type       string `json:"codec_type"`
			Codec      string `json:"codec_name"`
			Pixel      string `json:"pix_fmt"`
			Width      int    `json:"width"`
			Height     int    `json:"height"`
			Rate       string `json:"avg_frame_rate"`
			Nominal    string `json:"r_frame_rate"`
			Bitrate    string `json:"bit_rate"`
			SampleRate string `json:"sample_rate"`
			Channels   int    `json:"channels"`
			BFrames    int    `json:"has_b_frames"`
			FieldOrder string `json:"field_order"`
		} `json:"streams"`
		Format struct {
			Duration string `json:"duration"`
			Name     string `json:"format_name"`
		} `json:"format"`
		Packets []struct {
			Index    int    `json:"stream_index"`
			Duration string `json:"duration_time"`
			Time     string `json:"pts_time"`
			Flags    string `json:"flags"`
			Size     string `json:"size"`
		} `json:"packets"`
	}
	if json.Unmarshal(data, &raw) != nil || !strings.Contains(raw.Format.Name, "mp4") {
		return nil, errors.New("invalid MP4 inspection")
	}
	m := &VideoManifest{}
	videoIndex := -1
	videos, audios := 0, 0
	for _, s := range raw.Streams {
		switch s.Type {
		case "video":
			videos++
			videoIndex = s.Index
			m.Width, m.Height, m.Codec, m.PixelFormat = s.Width, s.Height, s.Codec, s.Pixel
			m.FPS = rational(s.Rate)
			m.FixedRate = m.FPS > 0 && math.Abs(m.FPS-rational(s.Nominal)) < 0.001
			m.ClosedGOP = s.BFrames == 0 && (s.FieldOrder == "progressive" || s.FieldOrder == "unknown")
			m.Bitrate, _ = strconv.ParseInt(s.Bitrate, 10, 64)
		case "audio":
			audios++
			m.AudioCodec = s.Codec
			m.AudioChannels = s.Channels
			m.AudioRate, _ = strconv.Atoi(s.SampleRate)
			m.AudioBitrate, _ = strconv.ParseInt(s.Bitrate, 10, 64)
		default:
			return nil, errors.New("video contains unsupported auxiliary streams")
		}
	}
	m.Duration, _ = strconv.ParseFloat(raw.Format.Duration, 64)
	if videos != 1 || audios > 1 || math.IsNaN(m.Duration) || math.IsInf(m.Duration, 0) || m.Duration <= 0 {
		return nil, errors.New("invalid video streams or duration")
	}
	count := 0
	lastAt := 0.0
	windowTimes := []float64{}
	windowBytes := []int64{}
	totalBytes := int64(0)
	lastKey := 0.0
	end := 0.0
	for _, p := range raw.Packets {
		if p.Index != videoIndex {
			continue
		}
		d, e := strconv.ParseFloat(p.Duration, 64)
		if e != nil || math.IsNaN(d) || math.IsInf(d, 0) || d <= 0 || m.FPS <= 0 || math.Abs(d-1/m.FPS) > 0.002 {
			m.FixedRate = false
		}
		at, e := strconv.ParseFloat(p.Time, 64)
		if e != nil || math.IsNaN(at) || math.IsInf(at, 0) {
			return nil, errors.New("invalid packet timestamps")
		}
		if count == 0 && (!strings.Contains(p.Flags, "K") || math.Abs(at) > 0.1) {
			m.ClosedGOP = false
		}
		if count > 0 && math.Abs(at-lastAt-1/m.FPS) > 0.002 {
			m.FixedRate = false
		}
		lastAt = at
		size, e := strconv.ParseInt(p.Size, 10, 64)
		if e != nil || size <= 0 || size > MaxUploadBytes {
			return nil, errors.New("bounded packet sizes required for bitrate inspection")
		}
		windowTimes = append(windowTimes, at)
		windowBytes = append(windowBytes, size)
		totalBytes += size
		for len(windowTimes) > 1 && at-windowTimes[0] >= 1 {
			totalBytes -= windowBytes[0]
			windowTimes = windowTimes[1:]
			windowBytes = windowBytes[1:]
		}
		if totalBytes*8 > m.PeakBitrate {
			m.PeakBitrate = totalBytes * 8
		}
		if strings.Contains(p.Flags, "K") {
			if at-lastKey > 5.1 {
				m.ClosedGOP = false
			}
			lastKey = at
		}
		count++
		end = math.Max(end, at+d)
	}
	if count < 1 || math.Abs(end-m.Duration) > 0.2 || m.Duration-lastKey > 5.1 {
		return nil, errors.New("complete bounded packet inspection required")
	}
	return m, nil
}

func fastStartMP4(path string) bool {
	f, e := os.Open(path)
	if e != nil {
		return false
	}
	defer f.Close()
	info, e := f.Stat()
	if e != nil || info.Size() > MaxUploadBytes {
		return false
	}
	brandOK := false
	for pos := int64(0); pos+8 <= info.Size(); {
		var header [8]byte
		if _, e = f.ReadAt(header[:], pos); e != nil {
			return false
		}
		size := int64(binary.BigEndian.Uint32(header[:4]))
		kind := string(header[4:])
		if size < 8 || pos+size > info.Size() {
			return false
		}
		if kind == "mdat" {
			return false
		}
		if kind == "ftyp" && size >= 16 {
			var brand [4]byte
			if _, e = f.ReadAt(brand[:], pos+8); e != nil {
				return false
			}
			switch string(brand[:]) {
			case "isom", "iso2", "mp41", "mp42", "avc1":
				brandOK = true
			}
		}
		if kind == "moov" {
			if size > 8*1024*1024 {
				return false
			}
			box := make([]byte, size)
			if _, e = f.ReadAt(box, pos); e != nil {
				return false
			}
			return brandOK && !bytes.Contains(box, []byte("edts"))
		}
		pos += size
	}
	return false
}

func inspectVideo(ctx context.Context, path string) (*VideoManifest, error) {
	tool, e := exec.LookPath("ffprobe")
	if e != nil {
		return nil, errors.New("video inspection unavailable: ffprobe required; video publishing disabled")
	}
	if !fastStartMP4(path) {
		return nil, errors.New("MP4 requires moov before mdat and no edit lists; no automatic transcoding")
	}
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	command := exec.CommandContext(ctx, tool, "-v", "error", "-protocol_whitelist", "file,pipe", "-format_whitelist", "mov", "-enable_drefs", "0", "-show_streams", "-show_format", "-show_packets", "-show_entries", "stream=index,codec_type,codec_name,pix_fmt,width,height,avg_frame_rate,r_frame_rate,bit_rate,sample_rate,channels,has_b_frames,field_order:format=duration,format_name:packet=stream_index,duration_time,pts_time,flags,size", "-of", "json", path)
	var output probeBuffer
	command.Stdout = &output
	command.Stderr = io.Discard
	if command.Run() != nil {
		return nil, errors.New("video inspection failed or exceeded 20-second/output limit")
	}
	m, e := parseVideoProbe(output.Bytes())
	if e != nil {
		return nil, e
	}
	m.FastStart = true
	return m, nil
}

func validateVideo(platform, kind string, m *VideoManifest) error {
	if m == nil || !m.FastStart || !m.FixedRate || !m.ClosedGOP || m.Codec != "h264" || m.PixelFormat != "yuv420p" || m.Width <= 0 || m.Width > 1920 || m.Height <= 0 || m.Height > 3840 || m.FPS < 24 || m.FPS > 60 || m.Duration < 3 || m.Bitrate <= 0 || m.Bitrate > 25000000 || m.PeakBitrate <= 0 || m.PeakBitrate > 25000000 {
		return errors.New("phase-one video requires inspected fast-start progressive H.264/yuv420p, fixed 24-60 FPS and closed GOP (max 5 seconds), <=25 Mbps")
	}
	if m.AudioCodec != "" && (m.AudioCodec != "aac" || m.AudioRate != 48000 || m.AudioChannels < 1 || m.AudioChannels > 2 || m.AudioBitrate <= 0 || m.AudioBitrate > 128000) {
		return errors.New("video audio requires AAC 48kHz, 1-2 channels, <=128kbps")
	}
	ratio := float64(m.Width) / float64(m.Height)
	switch platform {
	case "instagram":
		if kind != "reels" || m.Duration > 900 || ratio < 0.01 || ratio > 10 {
			return errors.New("Instagram video must be a Reel, 3-900 seconds within supported aspect ratio")
		}
	case "threads":
		if kind != "video" || m.Duration > 300 || ratio < 0.1 || ratio > 10 {
			return errors.New("Threads video must be 3-300 seconds within 10:1 aspect ratio")
		}
	case "facebook":
		if kind != "video" && kind != "reels" {
			return errors.New("select Facebook video or Reels")
		}
		if m.Duration > 600 {
			return errors.New("phase-one Facebook video limit is ten minutes")
		}
		if kind == "reels" && (m.Duration > 90 || m.Width < 540 || m.Height < 960 || math.Abs(ratio-9.0/16) > 0.01 || (m.AudioCodec != "" && (m.AudioChannels != 2 || m.AudioBitrate < 128000))) {
			return errors.New("Facebook Reels require 540x960 minimum, 9:16, 3-90 seconds and stereo AAC 128kbps when audio is present")
		}
	default:
		return errors.New("video publishing is not supported on this platform")
	}
	return nil
}

func readVideo(ctx context.Context, p *governance.Principal, plan gdb.Record, ref string, approved, reinspect bool) (*MediaFile, error) {
	if !strings.HasPrefix(ref, "task:") {
		return nil, errors.New("select a private task video")
	}
	row, e := Asset(ctx, p, strings.TrimPrefix(ref, "task:"), sourcePermission(p))
	if e != nil {
		return nil, e
	}
	if row["plan_id"].Int64() != plan["id"].Int64() || row["status"].String() != "ready" || row["mime"].String() != "video/mp4" || row["size"].Int64() <= 0 || row["size"].Int64() > MaxUploadBytes || (approved && !row["approved"].Bool()) || object(row["extracted"])["purpose"] != "post_video" {
		return nil, errors.New("inspect and approve this task video first")
	}
	path, e := assetPath(row)
	if e != nil {
		return nil, e
	}
	f, e := os.Open(path)
	if e != nil {
		return nil, e
	}
	defer f.Close()
	h := sha256File(f)
	if h == "" || h != row["sha256"].String() {
		return nil, errors.New("video checksum changed; upload and review again")
	}
	var manifest VideoManifest
	if e = gconv.Scan(object(row["extracted"])["video_manifest"], &manifest); e != nil {
		return nil, errors.New("video inspection manifest missing")
	}
	if reinspect {
		current, e := inspectVideo(ctx, path)
		if e != nil {
			return nil, e
		}
		if encode(current) != encode(manifest) {
			return nil, errors.New("video inspection changed; upload and review again")
		}
	}
	return &MediaFile{ID: ref, Filename: filepath.Base(row["filename"].String()), MIME: "video/mp4", Path: path, SHA256: h, Size: row["size"].Int64(), Video: &manifest, AIGenerated: gconv.Bool(object(row["extracted"])["ai_generated"])}, nil
}

func UploadPostVideo(ctx context.Context, p *governance.Principal, id int64, name string, input io.Reader, ai bool) (g.Map, error) {
	v, plan, e := variant(ctx, p, id, "social:content:edit")
	if e != nil {
		return nil, e
	}
	if strings.ToLower(filepath.Ext(name)) != ".mp4" || v["platform"].String() == "linkedin" {
		return nil, errors.New("select MP4 video for a supported Meta platform")
	}
	if _, e = exec.LookPath("ffprobe"); e != nil {
		return nil, errors.New("video upload preparation disabled: ffprobe inspection required")
	}
	row, e := saveAsset(ctx, p, plan["id"].Int64(), "", filepath.Base(strings.ReplaceAll(name, "\\", "/")), input, false)
	if e != nil {
		return nil, e
	}
	path, e := assetPath(row)
	if e != nil {
		return nil, e
	}
	m, e := inspectVideo(ctx, path)
	if e != nil {
		if row["status"].String() != "ready" {
			_, _ = g.DB().Model("bm_social_assets").Ctx(ctx).Where("id", row["id"]).Data(g.Map{"status": "failed", "error": e.Error()}).Update()
		}
		return nil, e
	}
	kind := "video"
	if v["platform"].String() == "instagram" {
		kind = "reels"
	}
	if e = validateVideo(v["platform"].String(), kind, m); e != nil {
		if row["status"].String() != "ready" {
			_, _ = g.DB().Model("bm_social_assets").Ctx(ctx).Where("id", row["id"]).Data(g.Map{"status": "failed", "error": e.Error()}).Update()
		}
		return nil, e
	}
	if row["status"].String() == "ready" {
		if object(row["extracted"])["purpose"] != "post_video" || gconv.Bool(object(row["extracted"])["ai_generated"]) != ai || encode(object(row["extracted"])["video_manifest"]) != encode(m) {
			return nil, errors.New("existing video metadata differs; use a new filename and review without changing approved media")
		}
		return g.Map{"id": row["id"].String(), "reference": "task:" + row["id"].String(), "filename": row["filename"].String(), "video_manifest": m}, nil
	}
	_, e = g.DB().Model("bm_social_assets").Ctx(ctx).Where("id", row["id"]).Data(g.Map{"mime": "video/mp4", "status": "ready", "approved": false, "category": "post_video", "extracted": encode(g.Map{"purpose": "post_video", "variant_id": id, "ai_generated": ai, "video_manifest": m})}).Update()
	return g.Map{"id": row["id"].String(), "reference": "task:" + row["id"].String(), "filename": row["filename"].String(), "video_manifest": m}, e
}

func videoForVariant(ctx context.Context, p *governance.Principal, plan gdb.Record, platform string, data g.Map, approved, reinspect bool) (*MediaFile, error) {
	kind := gconv.String(data["media_type"])
	if kind == "" {
		kind = "images"
	}
	ref := gconv.String(data["video"])
	if kind == "images" {
		if ref != "" {
			return nil, errors.New("select video or Reels before attaching a video")
		}
		return nil, nil
	}
	if len(imageReferences(data["images"])) > 0 || ref == "" {
		return nil, errors.New("select one inspected video without images")
	}
	if platform == "facebook" && kind == "reels" && !gconv.Bool(data["public_reels_confirmed"]) {
		return nil, errors.New("Facebook Page Reels are public; confirm public sharing and audience disclosure before review")
	}
	file, e := readVideo(ctx, p, plan, ref, approved, reinspect)
	if e != nil {
		return nil, e
	}
	if e = validateVideo(platform, kind, file.Video); e != nil {
		return nil, e
	}
	return file, nil
}
