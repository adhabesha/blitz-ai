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
	"sync"
	"time"
)

// tgServe holds files that Telegram may fetch by link for a few minutes (fallback when a direct upload is refused).
var tgServe sync.Map

func (s *server) tgFile(w http.ResponseWriter, r *http.Request) {
	parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/tg/"), "/")
	if len(parts) < 1 || len(parts[0]) < 16 {
		http.NotFound(w, r)
		return
	}
	v, ok := tgServe.Load(parts[0])
	if !ok {
		http.NotFound(w, r)
		return
	}
	http.ServeFile(w, r, v.(string))
}

// tgByLink asks Telegram to download the file from this server instead of receiving an upload.
func tgByLink(ctx context.Context, base, method, field string, f map[string]string, path string) (string, error) {
	pub := strings.TrimRight(env("BASE_URL"), "/")
	if pub == "" {
		return "", errors.New("BASE_URL is not set in Admin, so Telegram cannot fetch the file by link")
	}
	tok := randHex(12)
	tgServe.Store(tok, path)
	defer func() { time.AfterFunc(2*time.Minute, func() { tgServe.Delete(tok) }) }()
	q := url.Values{}
	for k, v := range f {
		q.Set(k, v)
	}
	q.Set(field, pub+"/tg/"+tok+"/"+url.PathEscape(filepath.Base(path)))
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
	if err := getJSON(ctx, base+"/"+method+"?"+q.Encode(), &res); err != nil {
		return "", err
	}
	if !res.OK {
		return "", errors.New(trunc(res.Description, 200))
	}
	if un := res.Result.Chat.Username; un != "" {
		return "https://t.me/" + un + "/" + itoa(res.Result.MessageID), nil
	}
	return "", nil
}

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
	upErr := tgUpload(ctx, base+"/"+method, f, field, sp.Path, &res)
	if upErr == nil && !res.OK {
		upErr = errors.New("Telegram refused the post: " + trunc(res.Description, 200))
	}
	if upErr != nil {
		// 1) Same upload without the caption (the caption is then sent as a normal message).
		f2 := map[string]string{"chat_id": t.UserID}
		if f["supports_streaming"] != "" {
			f2["supports_streaming"] = "true"
		}
		var r3 struct {
			OK     bool `json:"ok"`
			Result struct {
				MessageID int `json:"message_id"`
				Chat      struct {
					Username string `json:"username"`
				} `json:"chat"`
			} `json:"result"`
		}
		if e3 := tgUpload(ctx, base+"/"+method, f2, field, sp.Path, &r3); e3 == nil && r3.OK {
			var r4 struct{ OK bool }
			getJSON(ctx, base+"/sendMessage?"+url.Values{"chat_id": {t.UserID}, "text": {text}}.Encode(), &r4)
			link := ""
			if un := r3.Result.Chat.Username; un != "" {
				link = "https://t.me/" + un + "/" + itoa(r3.Result.MessageID)
			}
			return link, "Posted without a caption attached (Telegram refused the caption); the text followed as a separate message.", nil
		}
		// 2) Let Telegram download the file from this server.
		link, lerr := tgByLink(ctx, base, method, field, f, sp.Path)
		if lerr == nil {
			return link, "Posted by link after the direct upload failed.", nil
		}
		// 3) Say exactly what works: can the bot send plain text to this chat at all?
		var r5 struct {
			OK bool `json:"ok"`
		}
		txt := "plain text to the chat works, so only media is refused"
		if e5 := getJSON(ctx, base+"/sendMessage?"+url.Values{"chat_id": {t.UserID}, "text": {"ficx connection test"}}.Encode(), &r5); e5 != nil {
			txt = "even a plain text message is refused: " + scrub(e5, t.Access).Error()
		}
		return "", "", errors.New("Upload: " + scrub(upErr, t.Access).Error() + " | By link: " + scrub(lerr, t.Access).Error() + " | Test: " + txt + " | chat_id used: " + t.UserID)
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
