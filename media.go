package main

import (
	"encoding/binary"
	"errors"
	"fmt"
	"image"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"os"
)

type vidInfo struct {
	Dur      float64
	W, H     int
	Codec    string
	HasAudio bool
}

type mbox struct {
	typ  string
	data []byte
}

func parseBoxes(b []byte) []mbox {
	var out []mbox
	for len(b) >= 8 {
		size := int64(binary.BigEndian.Uint32(b[0:4]))
		typ := string(b[4:8])
		hdr := int64(8)
		if size == 1 {
			if len(b) < 16 {
				break
			}
			size = int64(binary.BigEndian.Uint64(b[8:16]))
			hdr = 16
		} else if size == 0 {
			size = int64(len(b))
		}
		if size < hdr || size > int64(len(b)) {
			break
		}
		out = append(out, mbox{typ, b[hdr:size]})
		b = b[size:]
	}
	return out
}

func findBox(bs []mbox, t string) *mbox {
	for i := range bs {
		if bs[i].typ == t {
			return &bs[i]
		}
	}
	return nil
}

// probeMP4 reads duration, size (rotation aware) and codec from an MP4/MOV file without ffmpeg.
func probeMP4(path string) (*vidInfo, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil {
		return nil, err
	}
	total := st.Size()
	var pos int64
	var moov []byte
	hdr := make([]byte, 16)
	for pos+8 <= total {
		if _, err := f.ReadAt(hdr[:8], pos); err != nil {
			break
		}
		size := int64(binary.BigEndian.Uint32(hdr[0:4]))
		typ := string(hdr[4:8])
		h := int64(8)
		if size == 1 {
			if _, err := f.ReadAt(hdr[8:16], pos+8); err != nil {
				break
			}
			size = int64(binary.BigEndian.Uint64(hdr[8:16]))
			h = 16
		} else if size == 0 {
			size = total - pos
		}
		if size < h {
			break
		}
		if typ == "moov" {
			if size-h > 64<<20 {
				return nil, errors.New("video header too large")
			}
			moov = make([]byte, size-h)
			if _, err := f.ReadAt(moov, pos+h); err != nil && err != io.EOF {
				return nil, err
			}
			break
		}
		pos += size
	}
	if moov == nil {
		return nil, errors.New("not a readable MP4/MOV file")
	}
	info := &vidInfo{}
	top := parseBoxes(moov)
	if mv := findBox(top, "mvhd"); mv != nil && len(mv.data) >= 20 {
		d := mv.data
		if d[0] == 1 && len(d) >= 32 {
			ts := binary.BigEndian.Uint32(d[20:24])
			du := binary.BigEndian.Uint64(d[24:32])
			if ts > 0 {
				info.Dur = float64(du) / float64(ts)
			}
		} else if d[0] == 0 {
			ts := binary.BigEndian.Uint32(d[12:16])
			du := binary.BigEndian.Uint32(d[16:20])
			if ts > 0 {
				info.Dur = float64(du) / float64(ts)
			}
		}
	}
	for _, b := range top {
		if b.typ != "trak" {
			continue
		}
		tr := parseBoxes(b.data)
		mdia := findBox(tr, "mdia")
		if mdia == nil {
			continue
		}
		md := parseBoxes(mdia.data)
		handler := ""
		if hd := findBox(md, "hdlr"); hd != nil && len(hd.data) >= 12 {
			handler = string(hd.data[8:12])
		}
		if handler == "soun" {
			info.HasAudio = true
			continue
		}
		if handler != "vide" || info.W > 0 {
			continue
		}
		if tk := findBox(tr, "tkhd"); tk != nil && len(tk.data) >= 84 {
			d := tk.data
			mOff, wOff := 40, 76
			if d[0] == 1 {
				mOff, wOff = 52, 88
			}
			if len(d) >= wOff+8 {
				a := int32(binary.BigEndian.Uint32(d[mOff : mOff+4]))
				dd := int32(binary.BigEndian.Uint32(d[mOff+16 : mOff+20]))
				w := int(binary.BigEndian.Uint32(d[wOff:wOff+4]) >> 16)
				h := int(binary.BigEndian.Uint32(d[wOff+4:wOff+8]) >> 16)
				if a == 0 && dd == 0 {
					w, h = h, w
				}
				info.W, info.H = w, h
			}
		}
		if minf := findBox(md, "minf"); minf != nil {
			if stbl := findBox(parseBoxes(minf.data), "stbl"); stbl != nil {
				if stsd := findBox(parseBoxes(stbl.data), "stsd"); stsd != nil && len(stsd.data) >= 16 {
					info.Codec = string(stsd.data[12:16])
				}
			}
		}
	}
	if info.W == 0 || info.H == 0 || info.Dur <= 0 {
		return nil, errors.New("could not read the video size or length")
	}
	return info, nil
}

type verdict struct{ Block, Note string }

func imageSize(path string) (int, int) {
	f, err := os.Open(path)
	if err != nil {
		return 0, 0
	}
	defer f.Close()
	cfg, _, err := image.DecodeConfig(f)
	if err != nil {
		return 0, 0
	}
	return cfg.Width, cfg.Height
}

// preflight decides, per platform, whether the file can be posted in the right format.
func preflight(sp pubSpec, info *vidInfo, iw, ih int) map[string]verdict {
	out := map[string]verdict{}
	for _, p := range sp.Platforms {
		out[p] = platformVerdict(p, sp, info, iw, ih)
	}
	return out
}

func platformVerdict(p string, sp pubSpec, info *vidInfo, iw, ih int) verdict {
	if p == "telegram" {
		if sp.Type == "image" && sp.Size > 10<<20 {
			return verdict{Block: "Telegram bots accept photos up to 10 MB. Compress the image."}
		}
		if sp.Type != "image" && sp.Size > 50<<20 {
			return verdict{Block: "Telegram bots accept videos up to 50 MB. Compress the video."}
		}
		return verdict{}
	}
	if sp.Type == "image" {
		if p == "instagram" {
			if sp.Ext == ".png" {
				return verdict{Block: "Instagram only accepts JPG images. Convert the image to JPG."}
			}
			if iw > 0 && ih > 0 {
				r := float64(iw) / float64(ih)
				if r < 0.8 || r > 1.91 {
					return verdict{Block: fmt.Sprintf("Instagram needs an aspect ratio between 4:5 and 1.91:1 (this image is %dx%d). Crop it first.", iw, ih)}
				}
			}
		}
		return verdict{}
	}
	if info == nil {
		return verdict{Block: "Could not read the video. Export it as MP4 (H.264) or MOV and try again."}
	}
	ratio := float64(info.W) / float64(info.H)
	landscape := info.W > info.H
	vcodecOK := info.Codec == "avc1" || info.Codec == "hvc1" || info.Codec == "hev1"
	switch p {
	case "youtube":
		if sp.Type == "short" {
			if info.Dur > 180 {
				return verdict{Block: fmt.Sprintf("YouTube Shorts must be 3 minutes or shorter (this video is %.0f s). Post it as a Normal video.", info.Dur)}
			}
			if landscape {
				return verdict{Block: "YouTube Shorts must be vertical or square (this video is landscape). Post it as a Normal video."}
			}
			return verdict{Note: "Posted as a YouTube Short (#Shorts added)."}
		}
	case "instagram":
		if info.Dur < 3 {
			return verdict{Block: "Instagram Reels must be at least 3 seconds long."}
		}
		if !vcodecOK {
			return verdict{Block: "Instagram needs H.264 or HEVC video (this file uses " + info.Codec + ")."}
		}
		if sp.Type == "short" && landscape {
			return verdict{Block: "Instagram Reels must be vertical (9:16). Post it as a Normal video, or export a vertical version."}
		}
		if landscape {
			return verdict{Note: "Instagram posts all videos as Reels; a landscape video will show with bars."}
		}
		if ratio < 0.5 || ratio > 0.63 {
			return verdict{Note: "Not exactly 9:16; Instagram may crop or add bars."}
		}
	case "tiktok":
		if info.Dur < 3 {
			return verdict{Block: "TikTok videos must be at least 3 seconds long."}
		}
		if info.Dur > 600 {
			return verdict{Block: "TikTok videos must be 10 minutes or shorter."}
		}
		if landscape {
			return verdict{Block: "TikTok videos must be vertical (9:16)."}
		}
		if !(vcodecOK || info.Codec == "vp09" || info.Codec == "vp08") {
			return verdict{Block: "TikTok needs H.264, H.265 or VP9 video (this file uses " + info.Codec + ")."}
		}
	case "facebook":
		if sp.Type == "short" {
			if !landscape && info.Dur >= 3 && info.Dur <= 90 {
				return verdict{Note: "Posted as a Facebook Reel."}
			}
			return verdict{Note: "Posted as a normal Facebook video (Reels need vertical 9:16 and 3-90 seconds)."}
		}
	}
	return verdict{}
}
