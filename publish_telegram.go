package main

import (
	"context"
	"errors"
	"net/url"
	"strings"
)

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
	if err := postMultipart(ctx, base+"/"+method, f, field, sp.Path, &res); err != nil {
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
