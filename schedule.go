package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
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

// Scheduled videos are also kept in Firestore (in 800 KB pieces) so a restart or a redeploy of the server, which
// clears its disk, no longer loses them. The copy is put back on disk when the post is due and deleted afterwards.
const mediaColl = "scheduled_media"
const mediaChunk = 800 << 10
const maxScheduledMedia = 150 << 20

func (s *server) persistMedia(ctx context.Context, path string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil {
		return err
	}
	if st.Size() > maxScheduledMedia {
		return fmt.Errorf("this file is larger than 150 MB, the limit for scheduled posts")
	}
	base := s.db.Collection(mediaColl).Doc(filepath.Base(path))
	buf := make([]byte, mediaChunk)
	for i := 0; ; i++ {
		n, rerr := io.ReadFull(f, buf)
		if n > 0 {
			if _, e := base.Collection("c").Doc(fmt.Sprintf("%05d", i)).Set(ctx, map[string]any{"b": append([]byte(nil), buf[:n]...)}); e != nil {
				return e
			}
		}
		if rerr == io.EOF || rerr == io.ErrUnexpectedEOF {
			break
		}
		if rerr != nil {
			return rerr
		}
	}
	_, err = base.Set(ctx, map[string]any{"size": st.Size(), "created": time.Now().UTC()})
	return err
}

// restoreMedia puts the stored copy back on disk when the file is missing.
func (s *server) restoreMedia(ctx context.Context, path string) error {
	if _, err := os.Stat(path); err == nil {
		return nil
	}
	base := s.db.Collection(mediaColl).Doc(filepath.Base(path))
	meta, err := base.Get(ctx)
	if err != nil {
		return err
	}
	want, _ := meta.Data()["size"].(int64)
	os.MkdirAll(filepath.Dir(path), 0o755)
	out, err := os.Create(path)
	if err != nil {
		return err
	}
	it := base.Collection("c").OrderBy(firestore.DocumentID, firestore.Asc).Documents(ctx)
	defer it.Stop()
	var got int64
	for {
		d, e := it.Next()
		if e == iterator.Done {
			break
		}
		if e != nil {
			out.Close()
			os.Remove(path)
			return e
		}
		b, _ := d.Data()["b"].([]byte)
		n, _ := out.Write(b)
		got += int64(n)
	}
	out.Close()
	if want > 0 && got != want {
		os.Remove(path)
		return fmt.Errorf("stored copy incomplete (%d of %d bytes)", got, want)
	}
	return nil
}

func (s *server) dropMedia(ctx context.Context, path string) {
	base := s.db.Collection(mediaColl).Doc(filepath.Base(path))
	it := base.Collection("c").Documents(ctx)
	for {
		d, e := it.Next()
		if e != nil {
			break
		}
		d.Ref.Delete(ctx)
	}
	it.Stop()
	base.Delete(ctx)
}

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
	defer s.dropMedia(ctx, sp.Path)
	if time.Since(when) > 12*time.Hour {
		finish("failed", failResults(data, "Missed: the server was not running within 12 hours of the scheduled time."))
		return
	}
	if err := s.restoreMedia(ctx, sp.Path); err != nil {
		finish("failed", failResults(data, "The video file could not be found (the server restarted and the saved copy could not be restored: "+trunc(err.Error(), 120)+"). Upload and schedule it again."))
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
		s.dropMedia(context.Background(), sp.Path)
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
