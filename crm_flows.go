package main

// ManyChat-style automations for Facebook and Instagram: comment-to-DM, keyword replies,
// welcome and default replies, questions that capture phone/email into the lead, and live chat.

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"

	"cloud.google.com/go/firestore"
)

type flowStep struct {
	Type     string // message | ask
	Text     string
	Field    string // ask: name | phone | email | note
	BtnTitle string
	BtnURL   string
}

func (s *server) flowsOf(ctx context.Context, wid string) []map[string]any {
	var out []map[string]any
	it := s.crmWS(wid).Collection("flows").Documents(ctx)
	defer it.Stop()
	for {
		d, err := it.Next()
		if err != nil {
			break
		}
		x := d.Data()
		x["id"] = d.Ref.ID
		out = append(out, x)
	}
	sort.Slice(out, func(i, j int) bool {
		a, _ := out[i]["createdAt"].(time.Time)
		b, _ := out[j]["createdAt"].(time.Time)
		return a.Before(b)
	})
	return out
}

func (s *server) crmFlows(w http.ResponseWriter, r *http.Request) {
	c := s.crmNeed(w, r)
	if c == nil {
		return
	}
	if !c.can("integrations", 'v') {
		c.deny(w, "view automations")
		return
	}
	fl := s.flowsOf(c.ctx, c.wid)
	if fl == nil {
		fl = []map[string]any{}
	}
	jsonOut(w, map[string]any{"flows": fl})
}

func (s *server) crmFlowSave(w http.ResponseWriter, r *http.Request) {
	c := s.crmNeed(w, r)
	if c == nil {
		return
	}
	var in struct {
		ID           string     `json:"id"`
		Name         string     `json:"name"`
		Active       bool       `json:"active"`
		Channel      string     `json:"channel"`
		Trigger      string     `json:"trigger"`
		Match        string     `json:"match"`
		Keywords     []string   `json:"keywords"`
		PostID       string     `json:"postId"`
		CommentReply string     `json:"commentReply"`
		Tag          string     `json:"tag"`
		Steps        []flowStepJ `json:"steps"`
	}
	if !readBody(w, r, &in, 64<<10) {
		return
	}
	need := byte('c')
	if in.ID != "" {
		need = 'e'
	}
	if !c.can("integrations", need) {
		c.deny(w, "change automations")
		return
	}
	in.Name = clip(strings.TrimSpace(in.Name), 80)
	if in.Name == "" {
		http.Error(w, "Give the automation a name.", 400)
		return
	}
	if !inList([]string{"facebook", "instagram", "both"}, in.Channel) {
		in.Channel = "both"
	}
	if !inList([]string{"comment", "dm_keyword", "welcome", "default"}, in.Trigger) {
		http.Error(w, "Choose a trigger.", 400)
		return
	}
	if in.Match != "exact" {
		in.Match = "contains"
	}
	var kws []string
	for _, k := range in.Keywords {
		if k = strings.ToLower(strings.TrimSpace(k)); k != "" && len(kws) < 30 {
			kws = append(kws, clip(k, 60))
		}
	}
	if in.Trigger == "dm_keyword" && len(kws) == 0 {
		http.Error(w, "Add at least one keyword for a keyword trigger.", 400)
		return
	}
	var steps []map[string]any
	for _, st := range in.Steps {
		if len(steps) >= 10 {
			break
		}
		txt := clip(strings.TrimSpace(st.Text), 900)
		if txt == "" {
			continue
		}
		t := ifs(st.Type == "ask", "ask", "message")
		f := st.Field
		if !inList([]string{"name", "phone", "email", "note"}, f) {
			f = "note"
		}
		steps = append(steps, map[string]any{"type": t, "text": txt, "field": f, "btnTitle": clip(strings.TrimSpace(st.BtnTitle), 20), "btnUrl": clip(strings.TrimSpace(st.BtnURL), 500)})
	}
	if len(steps) == 0 && strings.TrimSpace(in.CommentReply) == "" {
		http.Error(w, "Add at least one message to send.", 400)
		return
	}
	col := s.crmWS(c.wid).Collection("flows")
	data := map[string]any{"name": in.Name, "active": in.Active, "channel": in.Channel, "trigger": in.Trigger, "match": in.Match, "keywords": kws, "postId": clip(strings.TrimSpace(in.PostID), 80),
		"commentReply": clip(strings.TrimSpace(in.CommentReply), 500), "tag": clip(strings.TrimSpace(in.Tag), 40), "steps": steps, "updatedAt": time.Now()}
	if in.ID == "" {
		if len(s.flowsOf(c.ctx, c.wid)) >= 50 {
			http.Error(w, "You reached the limit of 50 automations.", 400)
			return
		}
		data["createdAt"] = time.Now()
		data["stats"] = map[string]any{"triggered": 0, "leads": 0, "completed": 0}
		ref := col.NewDoc()
		ref.Set(c.ctx, data)
		jsonOut(w, map[string]any{"ok": true, "id": ref.ID})
		return
	}
	ref := col.Doc(in.ID)
	if _, err := ref.Get(c.ctx); err != nil {
		http.Error(w, "Not found", 404)
		return
	}
	ref.Set(c.ctx, data, firestoreMerge())
	jsonOut(w, map[string]any{"ok": true, "id": in.ID})
}

type flowStepJ struct {
	Type     string `json:"type"`
	Text     string `json:"text"`
	Field    string `json:"field"`
	BtnTitle string `json:"btnTitle"`
	BtnURL   string `json:"btnUrl"`
}

func (s *server) crmFlowDelete(w http.ResponseWriter, r *http.Request) {
	c := s.crmNeed(w, r)
	if c == nil {
		return
	}
	var in map[string]string
	if !readBody(w, r, &in, 4<<10) {
		return
	}
	if !c.can("integrations", 'd') || in["id"] == "" {
		c.deny(w, "delete automations")
		return
	}
	s.crmWS(c.wid).Collection("flows").Doc(in["id"]).Delete(c.ctx)
	jsonOut(w, map[string]bool{"ok": true})
}

// ---------------------------------------------------------------- Graph sending

func graphPost(ctx context.Context, u string, tok string, body map[string]any) error {
	b, _ := json.Marshal(body)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, u+"?"+url.Values{"access_token": {tok}}.Encode(), bytes.NewReader(b))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	var out map[string]any
	return doJSON(req, &out)
}

// sendDM sends a message to a person. For the first reply to a comment pass commentID.
func sendDM(ctx context.Context, tok, pageID string, ig bool, to, commentID, text, btnTitle, btnURL string) error {
	g := "https://graph.facebook.com/v21.0/"
	if commentID != "" {
		if btnURL != "" {
			text += "\n" + btnURL
		}
		if ig {
			return graphPost(ctx, g+pageID+"/messages", tok, map[string]any{"recipient": map[string]any{"comment_id": commentID}, "message": map[string]any{"text": text}})
		}
		return graphPost(ctx, g+commentID+"/private_replies", tok, map[string]any{"message": text})
	}
	msg := map[string]any{"text": text}
	if btnURL != "" && btnTitle != "" {
		msg = map[string]any{"attachment": map[string]any{"type": "template", "payload": map[string]any{"template_type": "button", "text": text,
			"buttons": []map[string]any{{"type": "web_url", "url": btnURL, "title": btnTitle}}}}}
	}
	body := map[string]any{"recipient": map[string]any{"id": to}, "message": msg}
	if !ig {
		body["messaging_type"] = "RESPONSE"
	}
	return graphPost(ctx, g+pageID+"/messages", tok, body)
}

func publicReply(ctx context.Context, tok string, ig bool, commentID, text string) error {
	g := "https://graph.facebook.com/v21.0/" + commentID
	return graphPost(ctx, g+ifs(ig, "/replies", "/comments"), tok, map[string]any{"message": text})
}

// ---------------------------------------------------------------- engine

type flowEvent struct {
	wid, tok, pageID, sender, name, text, commentID, postID, mid, leadID string
	ig                                                              bool
}

func fillVars(t, name string) string {
	first := name
	if i := strings.IndexByte(name, ' '); i > 0 {
		first = name[:i]
	}
	return strings.NewReplacer("{{first_name}}", first, "{{name}}", name, "{first_name}", first, "{name}", name).Replace(t)
}

func flowMatches(f map[string]any, text string) bool {
	kws, _ := f["keywords"].([]any)
	if len(kws) == 0 {
		return true
	}
	t := strings.ToLower(strings.TrimSpace(text))
	for _, k := range kws {
		ks, _ := k.(string)
		if ks == "" {
			continue
		}
		if sv(f, "match") == "exact" {
			if t == ks {
				return true
			}
		} else if strings.Contains(t, ks) {
			return true
		}
	}
	return false
}

func logMsg(cv map[string]any, dir, text string) {
	m, _ := cv["msgs"].([]any)
	m = append(m, map[string]any{"d": dir, "t": clip(text, 600), "at": time.Now()})
	if len(m) > 40 {
		m = m[len(m)-40:]
	}
	cv["msgs"] = m
}

func (s *server) flowStat(ctx context.Context, wid, fid, key string) {
	s.crmWS(wid).Collection("flows").Doc(fid).Update(ctx, []firestore.Update{{Path: "stats." + key, Value: firestore.Increment(1)}})
}

// crmFlow handles one incoming DM or comment. It returns nothing; leads are created by the caller.
func (s *server) crmFlow(ctx context.Context, e flowEvent) {
	chn := ifs(e.ig, "instagram", "facebook")
	cid := chn + ":" + e.sender
	ref := s.crmWS(e.wid).Collection("convos").Doc(cid)
	cv := map[string]any{}
	existed := false
	if d, err := ref.Get(ctx); err == nil {
		cv, existed = d.Data(), true
	}
	if e.mid != "" && sv(cv, "lastMid") == e.mid {
		return
	}
	if e.commentID != "" && sv(cv, "lastCid") == e.commentID {
		return
	}
	cv["wid"], cv["channel"], cv["pageId"], cv["senderId"], cv["updatedAt"] = e.wid, chn, e.pageID, e.sender, time.Now()
	if e.name != "" {
		cv["name"] = e.name
	}
	if e.leadID != "" {
		cv["leadId"] = e.leadID
	}
	name := sv(cv, "name")
	if e.mid != "" {
		cv["lastMid"] = e.mid
	}
	if e.commentID != "" {
		cv["lastCid"] = e.commentID
	}
	cv["lastInAt"] = time.Now()
	label := e.text
	if e.commentID != "" {
		label = "(comment) " + e.text
	}
	logMsg(cv, "in", label)
	defer func() { ref.Set(ctx, cv) }()

	paused := sv(cv, "status") == "human" && time.Since(timeOf(cv["humanAt"])) < 24*time.Hour
	if paused {
		return
	}
	// answer to a question we asked
	if e.commentID == "" && sv(cv, "awaiting") != "" {
		s.flowAnswer(ctx, e, cv, name)
		return
	}
	flows := s.flowsOf(ctx, e.wid)
	var hit map[string]any
	pick := func(trig string, needText bool) map[string]any {
		for _, f := range flows {
			if f["active"] != true || sv(f, "trigger") != trig {
				continue
			}
			if ch := sv(f, "channel"); ch != "both" && ch != chn {
				continue
			}
			if needText && !flowMatches(f, e.text) {
				continue
			}
			if trig == "comment" && sv(f, "postId") != "" && !strings.Contains(e.postID, sv(f, "postId")) {
				continue
			}
			return f
		}
		return nil
	}
	if e.commentID != "" {
		hit = pick("comment", true)
	} else {
		hit = pick("dm_keyword", true)
		if hit == nil && !existed {
			hit = pick("welcome", false)
		}
		if hit == nil && time.Since(timeOf(cv["lastOutAt"])) > 30*time.Second {
			hit = pick("default", false)
		}
	}
	if hit == nil {
		return
	}
	fid := sv(hit, "id")
	s.flowStat(ctx, e.wid, fid, "triggered")
	cv["flowId"], cv["flowName"], cv["status"], cv["collected"] = fid, sv(hit, "name"), "open", map[string]any{}
	if t := sv(hit, "tag"); t != "" && e.leadID != "" {
		s.crmWS(e.wid).Collection("leads").Doc(e.leadID).Update(ctx, []firestore.Update{{Path: "tags", Value: firestore.ArrayUnion(t)}})
	}
	if e.commentID != "" && sv(hit, "commentReply") != "" {
		if err := publicReply(ctx, e.tok, e.ig, e.commentID, fillVars(sv(hit, "commentReply"), name)); err != nil {
			cv["lastError"] = "Public reply failed: " + ctext(err.Error(), 200)
		}
	}
	steps, _ := hit["steps"].([]any)
	s.flowRun(ctx, e, cv, name, steps, 0, e.commentID)
}

func timeOf(v any) time.Time { t, _ := v.(time.Time); return t }

func (s *server) flowRun(ctx context.Context, e flowEvent, cv map[string]any, name string, steps []any, from int, commentID string) {
	delete(cv, "awaiting")
	for i := from; i < len(steps); i++ {
		st, _ := steps[i].(map[string]any)
		txt := fillVars(sv(st, "text"), name)
		if err := sendDM(ctx, e.tok, e.pageID, e.ig, e.sender, commentID, txt, sv(st, "btnTitle"), sv(st, "btnUrl")); err != nil {
			cv["lastError"] = "Message failed: " + ctext(err.Error(), 250)
			cv["status"] = "error"
			return
		}
		delete(cv, "lastError")
		commentID = ""
		logMsg(cv, "out", txt)
		cv["lastOutAt"] = time.Now()
		if sv(st, "type") == "ask" {
			cv["awaiting"], cv["step"] = sv(st, "field"), i+1
			return
		}
	}
	cv["step"], cv["status"] = len(steps), "done"
	if fid := sv(cv, "flowId"); fid != "" {
		s.flowStat(ctx, e.wid, fid, "completed")
	}
}

func (s *server) flowAnswer(ctx context.Context, e flowEvent, cv map[string]any, name string) {
	field := sv(cv, "awaiting")
	ans := strings.TrimSpace(e.text)
	bad := ""
	switch field {
	case "email":
		if !reEmailOK.MatchString(ans) {
			bad = "That doesn't look like an email address. Could you type it again?"
		}
	case "phone":
		if len(normPhone(ans)) < 7 {
			bad = "That doesn't look like a phone number. Could you type it again with the country code?"
		}
	}
	if bad != "" {
		sendDM(ctx, e.tok, e.pageID, e.ig, e.sender, "", bad, "", "")
		logMsg(cv, "out", bad)
		return
	}
	col, _ := cv["collected"].(map[string]any)
	if col == nil {
		col = map[string]any{}
	}
	col[field] = clip(ans, 300)
	cv["collected"] = col
	if lid := sv(cv, "leadId"); lid != "" {
		ref := s.crmWS(e.wid).Collection("leads").Doc(lid)
		var up []firestore.Update
		switch field {
		case "email":
			up = append(up, firestore.Update{Path: "email", Value: strings.ToLower(ans)})
		case "phone":
			up = append(up, firestore.Update{Path: "phone", Value: clip(ans, 40)}, firestore.Update{Path: "phoneKey", Value: normPhone(ans)})
		case "name":
			up = append(up, firestore.Update{Path: "name", Value: clip(ans, 120)})
			cv["name"] = ans
			name = ans
		}
		if field == "note" {
			if d, err := ref.Get(ctx); err == nil {
				n := sv(d.Data(), "notes")
				up = append(up, firestore.Update{Path: "notes", Value: clip(strings.TrimSpace(n+"\n"+ans), 2000)})
			}
		}
		if len(up) > 0 {
			up = append(up, firestore.Update{Path: "updatedAt", Value: time.Now()})
			ref.Update(ctx, up)
			s.crmLog(ctx, e.wid, "leads", lid, name, "note", "Chat answer ("+field+"): "+ctext(ans, 200), "", "")
		}
		if field == "email" || field == "phone" {
			s.flowStat(ctx, e.wid, sv(cv, "flowId"), "leads")
		}
	}
	fd, err := s.crmWS(e.wid).Collection("flows").Doc(sv(cv, "flowId")).Get(ctx)
	if err != nil {
		delete(cv, "awaiting")
		return
	}
	steps, _ := fd.Data()["steps"].([]any)
	s.flowRun(ctx, e, cv, name, steps, int(nv(cv, "step")), "")
}

// ---------------------------------------------------------------- live chat

func (s *server) crmChats(w http.ResponseWriter, r *http.Request) {
	c := s.crmNeed(w, r)
	if c == nil {
		return
	}
	if !c.can("leads", 'v') {
		c.deny(w, "view conversations")
		return
	}
	out := []map[string]any{}
	it := s.crmWS(c.wid).Collection("convos").OrderBy("updatedAt", firestore.Desc).Limit(60).Documents(c.ctx)
	defer it.Stop()
	for {
		d, err := it.Next()
		if err != nil {
			break
		}
		x := d.Data()
		x["id"] = d.Ref.ID
		delete(x, "lastMid")
		delete(x, "lastCid")
		out = append(out, x)
	}
	jsonOut(w, map[string]any{"chats": out})
}

func (s *server) crmChatSend(w http.ResponseWriter, r *http.Request) {
	c := s.crmNeed(w, r)
	if c == nil {
		return
	}
	var in map[string]string
	if !readBody(w, r, &in, 8<<10) {
		return
	}
	text := strings.TrimSpace(in["text"])
	if !c.can("leads", 'e') || text == "" || in["id"] == "" {
		c.deny(w, "reply to conversations")
		return
	}
	ref := s.crmWS(c.wid).Collection("convos").Doc(in["id"])
	d, err := ref.Get(c.ctx)
	if err != nil {
		http.Error(w, "Not found", 404)
		return
	}
	cv := d.Data()
	_, tok, err := s.crmPageToken(c.ctx, sv(cv, "pageId"))
	if err != nil || tok == "" {
		http.Error(w, "Could not reach the connected page. Reconnect it under Lead sources.", 502)
		return
	}
	if err := sendDM(c.ctx, tok, sv(cv, "pageId"), sv(cv, "channel") == "instagram", sv(cv, "senderId"), "", clip(text, 900), "", ""); err != nil {
		http.Error(w, "Not sent: "+ctext(err.Error(), 250)+" (Meta only allows replies within 24 hours of their last message.)", 502)
		return
	}
	logMsg(cv, "out", text)
	cv["status"], cv["humanAt"], cv["updatedAt"], cv["lastOutAt"] = "human", time.Now(), time.Now(), time.Now()
	delete(cv, "awaiting")
	ref.Set(c.ctx, cv)
	jsonOut(w, map[string]bool{"ok": true})
}

func (s *server) crmChatResume(w http.ResponseWriter, r *http.Request) {
	c := s.crmNeed(w, r)
	if c == nil {
		return
	}
	var in map[string]string
	if !readBody(w, r, &in, 4<<10) {
		return
	}
	if !c.can("leads", 'e') || in["id"] == "" {
		c.deny(w, "change conversations")
		return
	}
	s.crmWS(c.wid).Collection("convos").Doc(in["id"]).Update(c.ctx, []firestore.Update{{Path: "status", Value: "done"}})
	jsonOut(w, map[string]bool{"ok": true})
}
