package main

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"os"
	"regexp"
	"sort"
	"strings"
	"time"

	"cloud.google.com/go/firestore"
	"google.golang.org/api/iterator"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// App-side scheduling for platforms whose APIs can't schedule (Instagram, TikTok).
// The post is stored in Firestore (collection scheduled_posts) and a background loop
// uploads it when the time comes. The browser does not need to stay open.
// The video file lives in MEDIA_DIR (default: the server's temp folder), so for
// reliable scheduling use a host with a persistent disk, or keep the server awake.

const schedColl = "scheduled_posts"

var reHex = regexp.MustCompile(`^[0-9a-f]{20}$`)

func appScheduled(p string) bool { return p == "instagram" || p == "tiktok" || p == "telegram" }

func (s *server) saveScheduled(ctx context.Context, uid string, sp pubSpec, plats []string) (string, error) {
	spec, _ := json.Marshal(sp)
	id := randHex(10)
	_, err := s.db.Collection(schedColl).Doc(id).Set(ctx, map[string]any{
		"uid": uid, "when": sp.Schedule.UTC(), "status": "pending", "spec": string(spec),
		"plats": plats, "title": cut(firstNonEmpty(sp.Title, sp.Caption), 80), "created": time.Now().UTC(), "results": "",
	})
	return id, err
}

func firstNonEmpty(a, b string) string {
	if strings.TrimSpace(a) != "" {
		return a
	}
	return b
}

func (s *server) scheduleLoop() {
	time.Sleep(20 * time.Second)
	s.failInterrupted()
	for {
		s.runDue()
		time.Sleep(30 * time.Second)
	}
}

// failInterrupted marks posts that were mid-upload when the server restarted, so they are never sent twice.
func (s *server) failInterrupted() {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	it := s.db.Collection(schedColl).Where("status", "==", "running").Documents(ctx)
	defer it.Stop()
	for {
		d, err := it.Next()
		if err != nil {
			return
		}
		d.Ref.Update(ctx, []firestore.Update{{Path: "status", Value: "failed"}, {Path: "results", Value: failResults(d.Data(), "The server restarted during the upload. Check the platform before retrying so it isn't posted twice.")}})
	}
}

func failResults(data map[string]any, msg string) string {
	var items []*pubStatus
	if pl, ok := data["plats"].([]any); ok {
		for _, p := range pl {
			if ps, ok := p.(string); ok {
				items = append(items, &pubStatus{Platform: ps, State: "failed", Message: msg})
			}
		}
	}
	b, _ := json.Marshal(items)
	return string(b)
}

func (s *server) runDue() {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	it := s.db.Collection(schedColl).Where("status", "==", "pending").Documents(ctx)
	defer it.Stop()
	for {
		d, err := it.Next()
		if err == iterator.Done {
			return
		}
		if err != nil {
			log.Println("schedule query:", err)
			return
		}
		data := d.Data()
		when, _ := data["when"].(time.Time)
		if when.IsZero() || when.After(time.Now()) {
			continue
		}
		// claim it so only one instance posts it
		ref := d.Ref
		err = s.db.RunTransaction(ctx, func(ctx context.Context, tx *firestore.Transaction) error {
			cur, err := tx.Get(ref)
			if err != nil {
				return err
			}
			if v, _ := cur.DataAt("status"); v != "pending" {
				return status.Error(codes.Aborted, "already claimed")
			}
			return tx.Update(ref, []firestore.Update{{Path: "status", Value: "running"}})
		})
		if err != nil {
			continue
		}
		go s.fireScheduled(ref, data, when)
	}
}

func (s *server) fireScheduled(ref *firestore.DocumentRef, data map[string]any, when time.Time) {
	ctx := context.Background()
	finish := func(state, results string) {
		ref.Update(ctx, []firestore.Update{{Path: "status", Value: state}, {Path: "results", Value: results}})
	}
	uid, _ := data["uid"].(string)
	raw, _ := data["spec"].(string)
	var sp pubSpec
	if json.Unmarshal([]byte(raw), &sp) != nil || uid == "" {
		finish("failed", failResults(data, "The saved post could not be read."))
		return
	}
	defer os.Remove(sp.Path)
	if time.Since(when) > 12*time.Hour {
		finish("failed", failResults(data, "Missed: the server was not running within 12 hours of the scheduled time."))
		return
	}
	if _, err := os.Stat(sp.Path); err != nil {
		finish("failed", failResults(data, "The video file is gone (the server restarted and its storage was cleared). Upload and schedule it again. A host with a persistent disk (MEDIA_DIR) prevents this."))
		return
	}
	var plats []string
	if pl, ok := data["plats"].([]any); ok {
		for _, p := range pl {
			if ps, ok := p.(string); ok && appScheduled(ps) {
				plats = append(plats, ps)
			}
		}
	}
	sp.Schedule, sp.Platforms, sp.Keep = nil, plats, true
	job := &pubJob{UID: uid, Created: time.Now()}
	for _, p := range plats {
		job.Items = append(job.Items, &pubStatus{Platform: p, State: "queued"})
	}
	s.runPublish(job, uid, sp)
	allOK := true
	for _, it := range job.Items {
		if it.State != "done" {
			allOK = false
		}
	}
	b, _ := json.Marshal(job.Items)
	finish(map[bool]string{true: "done", false: "failed"}[allOK], string(b))
}

// scheduledList returns the user's scheduled posts (newest schedule first).
func (s *server) scheduledList(w http.ResponseWriter, r *http.Request) {
	uid, err := s.uid(r)
	if err != nil {
		http.Error(w, "unauthorized", 401)
		return
	}
	it := s.db.Collection(schedColl).Where("uid", "==", uid).Documents(r.Context())
	defer it.Stop()
	type row struct {
		ID      string       `json:"id"`
		When    time.Time    `json:"when"`
		Status  string       `json:"status"`
		Title   string       `json:"title"`
		Plats   []string     `json:"plats"`
		Results []*pubStatus `json:"results"`
	}
	out := []row{}
	for {
		d, err := it.Next()
		if err != nil {
			break
		}
		m := d.Data()
		rw := row{ID: d.Ref.ID}
		rw.When, _ = m["when"].(time.Time)
		rw.Status, _ = m["status"].(string)
		rw.Title, _ = m["title"].(string)
		if pl, ok := m["plats"].([]any); ok {
			for _, p := range pl {
				if ps, ok := p.(string); ok {
					rw.Plats = append(rw.Plats, ps)
				}
			}
		}
		if rs, _ := m["results"].(string); rs != "" {
			json.Unmarshal([]byte(rs), &rw.Results)
		}
		if rw.Status == "cancelled" {
			continue
		}
		out = append(out, rw)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].When.After(out[j].When) })
	if len(out) > 30 {
		out = out[:30]
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(out)
}

func (s *server) scheduledCancel(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", 405)
		return
	}
	uid, err := s.uid(r)
	if err != nil {
		http.Error(w, "unauthorized", 401)
		return
	}
	var in struct {
		ID string `json:"id"`
	}
	if json.NewDecoder(http.MaxBytesReader(w, r.Body, 4<<10)).Decode(&in) != nil || !reHex.MatchString(in.ID) {
		http.Error(w, "bad request", 400)
		return
	}
	ref := s.db.Collection(schedColl).Doc(in.ID)
	var spec string
	err = s.db.RunTransaction(r.Context(), func(ctx context.Context, tx *firestore.Transaction) error {
		cur, err := tx.Get(ref)
		if err != nil {
			return err
		}
		m := cur.Data()
		if m["uid"] != uid || m["status"] != "pending" {
			return status.Error(codes.FailedPrecondition, "not cancellable")
		}
		spec, _ = m["spec"].(string)
		return tx.Update(ref, []firestore.Update{{Path: "status", Value: "cancelled"}})
	})
	if err != nil {
		http.Error(w, "That post can't be cancelled (it may already be uploading).", 400)
		return
	}
	var sp pubSpec
	if json.Unmarshal([]byte(spec), &sp) == nil && sp.Path != "" {
		os.Remove(sp.Path)
	}
	w.Write([]byte(`{"ok":true}`))
}

// scheduledNow moves a waiting post's time to now so the scheduler sends it within seconds.
func (s *server) scheduledNow(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", 405)
		return
	}
	uid, err := s.uid(r)
	if err != nil {
		http.Error(w, "unauthorized", 401)
		return
	}
	var in struct {
		ID string `json:"id"`
	}
	if json.NewDecoder(http.MaxBytesReader(w, r.Body, 4<<10)).Decode(&in) != nil || !reHex.MatchString(in.ID) {
		http.Error(w, "bad request", 400)
		return
	}
	ref := s.db.Collection(schedColl).Doc(in.ID)
	err = s.db.RunTransaction(r.Context(), func(ctx context.Context, tx *firestore.Transaction) error {
		cur, err := tx.Get(ref)
		if err != nil {
			return err
		}
		m := cur.Data()
		if m["uid"] != uid || m["status"] != "pending" {
			return status.Error(codes.FailedPrecondition, "not waiting")
		}
		return tx.Update(ref, []firestore.Update{{Path: "when", Value: time.Now().UTC().Add(-time.Second)}})
	})
	if err != nil {
		http.Error(w, "That post is not waiting any more.", 400)
		return
	}
	go s.runDue()
	w.Write([]byte(`{"ok":true}`))
}
