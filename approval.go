package main

// Sign-up approval: new accounts can sign up but cannot use anything until an admin approves them.
// Switch off with APPROVAL_REQUIRED=0. Admin emails (ADMIN_EMAILS) are always allowed.

import (
	"context"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"

	"firebase.google.com/go/v4/auth"
	"google.golang.org/api/iterator"
)

type apprCache struct {
	ok  bool
	exp time.Time
}

var (
	apprMu sync.Mutex
	apprC  = map[string]apprCache{}
)

func approvalOn() bool { return env("APPROVAL_REQUIRED") != "0" }

func isAdminEmail(email string, verified bool) bool {
	if !verified || email == "" {
		return false
	}
	for _, a := range strings.Split(env("ADMIN_EMAILS"), ",") {
		if strings.EqualFold(strings.TrimSpace(a), email) {
			return true
		}
	}
	return false
}

func tokVerified(t *auth.Token) (string, bool) {
	email, _ := t.Claims["email"].(string)
	v, _ := t.Claims["email_verified"].(bool)
	if fb, ok := t.Claims["firebase"].(map[string]any); ok {
		if p, _ := fb["sign_in_provider"].(string); p == "google.com" {
			v = true
		}
	}
	return strings.ToLower(strings.TrimSpace(email)), v
}

// approved reports whether the signed-in person may use the app. Pending accounts are recorded and the admin is notified once.
func (s *server) approved(ctx context.Context, t *auth.Token) bool {
	if !approvalOn() {
		return true
	}
	email, ver := tokVerified(t)
	if isAdminEmail(email, ver) {
		return true
	}
	apprMu.Lock()
	if c, ok := apprC[t.UID]; ok && time.Now().Before(c.exp) {
		apprMu.Unlock()
		return c.ok
	}
	apprMu.Unlock()
	ok := false
	ref := s.db.Collection("approved_users").Doc(t.UID)
	d, err := ref.Get(ctx)
	if err == nil {
		ok, _ = d.Data()["approved"].(bool)
	} else {
		ref.Set(ctx, map[string]any{"approved": false, "pending": true, "email": email, "createdAt": time.Now()})
		go s.notifyAdminSignup(email)
	}
	ttl := 10 * time.Second
	if ok {
		ttl = 60 * time.Second
	}
	apprMu.Lock()
	apprC[t.UID] = apprCache{ok, time.Now().Add(ttl)}
	apprMu.Unlock()
	return ok
}

func (s *server) notifyAdminSignup(email string) {
	if !mailConfigured() {
		return
	}
	to := strings.TrimSpace(strings.Split(env("ADMIN_EMAILS"), ",")[0])
	if to == "" {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	base := strings.TrimRight(env("BASE_URL"), "/")
	sendMail(ctx, to, "New sign-up waiting for your approval", "<p>"+email+" signed up and is waiting for approval.</p><p><a href=\""+base+"/admin\">Open the admin console</a> to approve or block.</p>", email+" signed up and is waiting for approval: "+base+"/admin")
}

func (s *server) apprForget(uid string) {
	apprMu.Lock()
	delete(apprC, uid)
	apprMu.Unlock()
}

// GET /api/approval: used by the front end after a 401, and by the waiting page.
func (s *server) approvalState(w http.ResponseWriter, r *http.Request) {
	tok := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
	t, err := s.auth.VerifyIDToken(r.Context(), tok)
	if err != nil {
		http.Error(w, "unauthorized", 401)
		return
	}
	email, _ := tokVerified(t)
	s.apprForget(t.UID)
	jsonOut(w, map[string]any{"approved": s.approved(r.Context(), t), "email": email})
}

// ---------------------------------------------------------------- admin: sign-ups

func (s *server) adminUsers(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.adminOK(r); !ok {
		http.Error(w, "forbidden", 403)
		return
	}
	status := map[string]map[string]any{}
	it := s.db.Collection("approved_users").Documents(r.Context())
	for {
		d, err := it.Next()
		if err != nil {
			break
		}
		status[d.Ref.ID] = d.Data()
	}
	it.Stop()
	out := []map[string]any{}
	ui := s.auth.Users(r.Context(), "")
	for len(out) < 1000 {
		u, err := ui.Next()
		if err == iterator.Done || err != nil {
			break
		}
		st := "pending"
		if x, ok := status[u.UID]; ok {
			if x["blocked"] == true {
				st = "blocked"
			} else if x["approved"] == true {
				st = "approved"
			}
		}
		if isAdminEmail(strings.ToLower(u.Email), true) {
			st = "admin"
		}
		prov := ""
		if len(u.ProviderUserInfo) > 0 {
			prov = u.ProviderUserInfo[0].ProviderID
		}
		out = append(out, map[string]any{"uid": u.UID, "email": u.Email, "name": u.DisplayName, "provider": prov, "verified": u.EmailVerified, "disabled": u.Disabled,
			"created": u.UserMetadata.CreationTimestamp, "lastLogin": u.UserMetadata.LastLogInTimestamp, "status": st})
	}
	sort.Slice(out, func(i, j int) bool { return out[i]["created"].(int64) > out[j]["created"].(int64) })
	jsonOut(w, map[string]any{"users": out, "approvalOn": approvalOn()})
}

func (s *server) adminUserSet(w http.ResponseWriter, r *http.Request) {
	adm, ok := s.adminOK(r)
	if !ok {
		http.Error(w, "forbidden", 403)
		return
	}
	var in struct{ UID, Action string }
	if !readBody(w, r, &in, 2<<10) || in.UID == "" {
		return
	}
	ctx := r.Context()
	u, err := s.auth.GetUser(ctx, in.UID)
	if err != nil {
		http.Error(w, "User not found", 404)
		return
	}
	if strings.EqualFold(u.Email, adm) && in.Action != "approve" {
		http.Error(w, "You can't block or delete your own admin account.", 400)
		return
	}
	ref := s.db.Collection("approved_users").Doc(in.UID)
	now := time.Now()
	switch in.Action {
	case "approve":
		ref.Set(ctx, map[string]any{"approved": true, "pending": false, "blocked": false, "email": u.Email, "approvedAt": now, "approvedBy": adm})
		if u.Disabled {
			s.auth.UpdateUser(ctx, in.UID, (&auth.UserToUpdate{}).Disabled(false))
		}
	case "revoke":
		ref.Set(ctx, map[string]any{"approved": false, "pending": true, "blocked": false, "email": u.Email})
	case "block":
		ref.Set(ctx, map[string]any{"approved": false, "pending": false, "blocked": true, "email": u.Email, "blockedAt": now})
		s.auth.UpdateUser(ctx, in.UID, (&auth.UserToUpdate{}).Disabled(true))
		s.auth.RevokeRefreshTokens(ctx, in.UID)
	case "delete":
		ref.Delete(ctx)
		if err := s.auth.DeleteUser(ctx, in.UID); err != nil {
			http.Error(w, "Could not delete: "+trunc(err.Error(), 120), 500)
			return
		}
	default:
		http.Error(w, "bad action", 400)
		return
	}
	s.apprForget(in.UID)
	jsonOut(w, map[string]bool{"ok": true})
}
