package main

import (
	"bytes"
	"context"
	"errors"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// tgUpload sends a multipart upload with a known Content-Length. Telegram answers 502 Bad Gateway to chunked (streamed) uploads,
// so the body is built completely first. Files are capped at 50 MB, so this fits in memory.
func tgUpload(ctx context.Context, u string, fields map[string]string, fileField, path string, out any) error {
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	for k, v := range fields {
		if err := mw.WriteField(k, v); err != nil {
			return err
		}
	}
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	part, err := mw.CreateFormFile(fileField, filepath.Base(path))
	if err != nil {
		return err
	}
	if _, err = io.Copy(part, f); err != nil {
		return err
	}
	if err = mw.Close(); err != nil {
		return err
	}
	body, ctype := buf.Bytes(), mw.FormDataContentType()
	var last error
	for try := 0; try < 3; try++ {
		if try > 0 {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(time.Duration(try) * 3 * time.Second):
			}
		}
		req, e := http.NewRequestWithContext(ctx, http.MethodPost, u, bytes.NewReader(body))
		if e != nil {
			return e
		}
		req.Header.Set("Content-Type", ctype)
		if last = doJSON(req, out); last == nil {
			return nil
		}
		if !strings.Contains(last.Error(), "502") && !strings.Contains(last.Error(), "504") && !strings.Contains(last.Error(), "429") {
			return last
		}
	}
	return last
}

// pubTelegram posts a video or photo (with caption) to the connected Telegram channel or group through the person's own bot.
func (s *server) pubTelegram(ctx context.Context, uid string, sp pubSpec) (string, string, error) {
	t, err := s.validToken(ctx, uid, provOf("telegram"))
	if err != nil {
		return "", "", errors.New("Telegram is not connected. Open Connected social and add your bot token and channel.")
	}
	base := "https://api.telegram.org/bot" + t.Access
	text := strings.TrimSpace(strings.TrimSpace(sp.Title) + "\n\n" + strings.TrimSpace(sp.Caption))
	caption, rest := text, ""
	if r := []rune(text); len(r) > 1024 {
		caption, rest = string(r[:1024]), string(r[1024:])
	}
	method, field := "sendVideo", "video"
	f := map[string]string{"chat_id": t.UserID, "caption": caption}
	if sp.Type == "image" {
		method, field = "sendPhoto", "photo"
	} else {
		f["supports_streaming"] = "true"
	}
	var res struct {
		OK          bool   `json:"ok"`
		Description string `json:"description"`
		Result      struct {
			MessageID int `json:"message_id"`
			Chat      struct {
				Username string `json:"username"`
			} `json:"chat"`
		} `json:"result"`
	}
	if err := tgUpload(ctx, base+"/"+method, f, field, sp.Path, &res); err != nil {
		return "", "", scrub(err, t.Access)
	}
	if !res.OK {
		return "", "", errors.New("Telegram refused the post: " + trunc(res.Description, 200) + ". Make sure the bot is an administrator of the channel.")
	}
	if rest != "" {
		var r2 struct{ OK bool }
		getJSON(ctx, base+"/sendMessage?"+url.Values{"chat_id": {t.UserID}, "text": {rest}}.Encode(), &r2)
	}
	link := ""
	if un := res.Result.Chat.Username; un != "" {
		link = "https://t.me/" + un + "/" + itoa(res.Result.MessageID)
	}
	return link, "", nil
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b []byte
	for n > 0 {
		b = append([]byte{byte('0' + n%10)}, b...)
		n /= 10
	}
	return string(b)
}
