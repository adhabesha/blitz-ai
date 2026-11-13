package main

// WhatsApp Business (Cloud API): messages arrive through the same Meta webhook and land in CRM Chats and Automations.

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"
)


func waSend(ctx context.Context, tok, phoneID, to, text string) error {
	return graphPost(ctx, "https://graph.facebook.com/v21.0/"+phoneID+"/messages", tok, map[string]any{
		"messaging_product": "whatsapp", "to": reDigits.ReplaceAllString(to, ""), "type": "text", "text": map[string]any{"body": text}})
}

func (s *server) crmWhatsApp(w http.ResponseWriter, r *http.Request) {
	c := s.crmNeed(w, r)
	if c == nil {
		return
	}
	if !c.can("integrations", 'v') {
		c.deny(w, "view WhatsApp")
		return
	}
	base := strings.TrimRight(env("BASE_URL"), "/")
	if base == "" {
		base = "https://" + r.Host
	}
	it := s.db.Collection("crm_pages").Where("wid", "==", c.wid).Documents(c.ctx)
	defer it.Stop()
	out := []map[string]any{}
	for {
		d, err := it.Next()
		if err != nil {
			break
		}
		if x := d.Data(); sv(x, "kind") == "whatsapp" {
			out = append(out, map[string]any{"id": d.Ref.ID, "name": sv(x, "name"), "display": sv(x, "display")})
		}
	}
	jsonOut(w, map[string]any{"numbers": out, "webhookUrl": base + "/api/crm/meta/webhook", "verifyToken": metaVerifyToken(), "appSecretSet": env("FACEBOOK_APP_SECRET") != ""})
}

func (s *server) crmWhatsAppConnect(w http.ResponseWriter, r *http.Request) {
	c := s.crmNeed(w, r)
	if c == nil {
		return
	}
	if !c.can("integrations", 'e') {
		c.deny(w, "connect WhatsApp")
		return
	}
	var in struct{ PhoneID, Token string }
	if !readBody(w, r, &in, 4<<10) {
		return
	}
	in.PhoneID, in.Token = strings.TrimSpace(in.PhoneID), strings.TrimSpace(in.Token)
	if !regexp.MustCompile(`^\d{8,20}$`).MatchString(in.PhoneID) || len(in.Token) < 20 {
		http.Error(w, "Enter the Phone number ID (digits only) and the access token from the Meta WhatsApp setup page.", 400)
		return
	}
	if d, err := s.db.Collection("crm_pages").Doc(in.PhoneID).Get(c.ctx); err == nil && sv(d.Data(), "wid") != c.wid {
		http.Error(w, "That number is already connected to another workspace.", 400)
		return
	}
	var info struct {
		Display string `json:"display_phone_number"`
		Name    string `json:"verified_name"`
	}
	ctx, cancel := context.WithTimeout(c.ctx, 25*time.Second)
	defer cancel()
	if err := getJSON(ctx, "https://graph.facebook.com/v21.0/"+in.PhoneID+"?"+url.Values{"fields": {"display_phone_number,verified_name"}, "access_token": {in.Token}}.Encode(), &info); err != nil {
		http.Error(w, "Meta did not accept that Phone number ID and token: "+trunc(strings.ReplaceAll(err.Error(), in.Token, "***"), 160), 400)
		return
	}
	_, err := s.db.Collection("crm_pages").Doc(in.PhoneID).Set(c.ctx, map[string]any{"wid": c.wid, "ownerUid": c.uid, "kind": "whatsapp", "token": in.Token, "display": info.Display,
		"name": "WhatsApp " + firstNonEmptyStr(info.Display, info.Name)})
	if err != nil {
		http.Error(w, "Could not save.", 500)
		return
	}
	jsonOut(w, map[string]string{"display": info.Display})
}

func (s *server) crmWhatsAppRemove(w http.ResponseWriter, r *http.Request) {
	c := s.crmNeed(w, r)
	if c == nil {
		return
	}
	if !c.can("integrations", 'd') {
		c.deny(w, "remove WhatsApp")
		return
	}
	var in struct{ ID string }
	if !readBody(w, r, &in, 1<<10) || in.ID == "" || strings.Contains(in.ID, "/") {
		return
	}
	ref := s.db.Collection("crm_pages").Doc(in.ID)
	if d, err := ref.Get(c.ctx); err == nil && sv(d.Data(), "wid") == c.wid && sv(d.Data(), "kind") == "whatsapp" {
		ref.Delete(c.ctx)
	}
	jsonOut(w, map[string]bool{"ok": true})
}

// waEvents handles webhook events of the whatsapp_business_account object.
func (s *server) waEvents(body []byte) {
	var ev struct {
		Entry []struct {
			Changes []struct {
				Field string `json:"field"`
				Value struct {
					Metadata struct {
						PhoneID string `json:"phone_number_id"`
					} `json:"metadata"`
					Contacts []struct {
						WaID    string `json:"wa_id"`
						Profile struct {
							Name string `json:"name"`
						} `json:"profile"`
					} `json:"contacts"`
					Messages []struct {
						From, ID, Type string
						Text           struct {
							Body string `json:"body"`
						} `json:"text"`
					} `json:"messages"`
				} `json:"value"`
			} `json:"changes"`
		} `json:"entry"`
	}
	if json.Unmarshal(body, &ev) != nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Second)
	defer cancel()
	for _, en := range ev.Entry {
		for _, ch := range en.Changes {
			if ch.Field != "messages" || len(ch.Value.Messages) == 0 {
				continue
			}
			pid := ch.Value.Metadata.PhoneID
			d, err := s.db.Collection("crm_pages").Doc(pid).Get(ctx)
			if err != nil || sv(d.Data(), "kind") != "whatsapp" {
				s.hookLog(ctx, "", "whatsapp event", "Received for phone number ID "+pid+" but no workspace has it connected. Add it under Lead sources > WhatsApp.")
				continue
			}
			wid, tok := sv(d.Data(), "wid"), "wa:"+sv(d.Data(), "token")
			for _, m := range ch.Value.Messages {
				name := ""
				for _, c := range ch.Value.Contacts {
					if c.WaID == m.From {
						name = c.Profile.Name
					}
				}
				name = firstNonEmptyStr(name, "+"+m.From)
				text := strings.TrimSpace(m.Text.Body)
				s.hookLog(ctx, wid, "whatsapp event", "Message from "+name+" ("+m.Type+")")
				if text == "" {
					text = "[" + firstNonEmptyStr(m.Type, "message") + "]"
				}
				f := map[string]any{"name": clip(name, 120), "phone": "+" + m.From, "source": "wa_dm", "sourceRef": "wa_dm:" + m.From}
				lid := s.crmInboundID(ctx, wid, f, "WhatsApp: "+ctext(text, 500))
				s.crmFlow(ctx, flowEvent{wid: wid, tok: tok, pageID: pid, wa: true, sender: m.From, name: name, text: text, mid: m.ID, leadID: lid})
			}
		}
	}
}
