package conformance

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/adssib/StressLab/internal/api"
	"github.com/adssib/StressLab/internal/seed"
)

// goldenPosts are the posts whose bodies must never change between steps.
var goldenPosts = []int64{1, 2, 3, 250_000, 499_999, 500_000}

// missingPost is an ID no seeded or created post will reach.
const missingPost = 900_000_000

type suite struct {
	t      *testing.T
	url    string
	tokens *api.Tokens
	client *http.Client
}

type response struct {
	status int
	header http.Header
	body   []byte
}

type post struct {
	ID     int64 `json:"id"`
	Author struct {
		ID       int64  `json:"id"`
		Username string `json:"username"`
	} `json:"author"`
	Body      string `json:"body"`
	LikeCount int64  `json:"like_count"`
	CreatedAt string `json:"created_at"`
}

func TestConformance(t *testing.T) {
	url := os.Getenv("STRESSLAB_URL")
	if url == "" {
		t.Skip("STRESSLAB_URL not set: the conformance suite needs a live API on a freshly reset database")
	}
	key := os.Getenv("STRESSLAB_TOKEN_KEY")
	tokens, err := api.NewTokens([]byte(key), time.Hour)
	if err != nil {
		t.Fatalf("STRESSLAB_TOKEN_KEY must be the API's signing key: %v", err)
	}
	s := &suite{t: t, url: strings.TrimRight(url, "/"), tokens: tokens, client: &http.Client{Timeout: 10 * time.Second}}

	// Read-only checks first: the golden bodies assume nothing has been written.
	t.Run("golden", s.golden)
	t.Run("auth", s.auth)
	t.Run("feed", s.feed)
	t.Run("posts", s.posts)
	t.Run("likes", s.likes)
	t.Run("create", s.create)
	t.Run("service", s.service)
}

func (s *suite) golden(t *testing.T) {
	token := s.login(t, 1)
	update := os.Getenv("STRESSLAB_UPDATE_GOLDEN") == "1"
	check := func(name, path string) {
		r := s.do(t, "GET", path, token, nil)
		wantStatus(t, r, http.StatusOK)
		file := filepath.Join("testdata", "golden", name+".json")
		if update {
			if err := os.MkdirAll(filepath.Dir(file), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(file, r.body, 0o644); err != nil {
				t.Fatal(err)
			}
			return
		}
		want, err := os.ReadFile(file)
		if err != nil {
			t.Fatalf("missing golden file %s (create it from step 0 with STRESSLAB_UPDATE_GOLDEN=1): %v", file, err)
		}
		if !bytes.Equal(r.body, want) {
			t.Errorf("%s differs from the step-0 golden file %s\n got: %s\nwant: %s", path, file, r.body, want)
		}
	}
	check("feed", "/v1/feed")
	for _, id := range goldenPosts {
		check(fmt.Sprintf("post-%d", id), fmt.Sprintf("/v1/posts/%d", id))
	}
}

func (s *suite) auth(t *testing.T) {
	t.Run("login issues a working token", func(t *testing.T) {
		r := s.basic(t, seed.Username(1), seed.Password)
		wantStatus(t, r, http.StatusOK)
		var out struct {
			Token     string `json:"token"`
			ExpiresAt string `json:"expires_at"`
		}
		decode(t, r, &out)
		exp, err := time.Parse(time.RFC3339, out.ExpiresAt)
		if err != nil || out.Token == "" {
			t.Fatalf("want a token and an RFC 3339 expiry, got %s", r.body)
		}
		if d := time.Until(exp); d < 59*time.Minute || d > 61*time.Minute {
			t.Errorf("token should last 1 h, expires in %s", d)
		}
		wantStatus(t, s.do(t, "GET", "/v1/feed", out.Token, nil), http.StatusOK)
	})

	var wrongBody []byte
	t.Run("wrong password is 401", func(t *testing.T) {
		r := s.basic(t, seed.Username(1), "wrong-password")
		wantError(t, r, http.StatusUnauthorized)
		wrongBody = r.body
	})
	t.Run("unknown user is 401 with the same body", func(t *testing.T) {
		r := s.basic(t, "no_such_user", seed.Password)
		wantError(t, r, http.StatusUnauthorized)
		if !bytes.Equal(r.body, wrongBody) {
			t.Errorf("unknown user and wrong password must look identical\nunknown: %s\n  wrong: %s", r.body, wrongBody)
		}
	})
	t.Run("unknown user takes as long as a wrong password", func(t *testing.T) {
		var wrong, unknown []time.Duration
		for range 10 {
			wrong = append(wrong, s.timed(t, func() { s.basic(t, seed.Username(2), "wrong-password") }))
			unknown = append(unknown, s.timed(t, func() { s.basic(t, "no_such_user", "wrong-password") }))
		}
		w, u := median(wrong), median(unknown)
		if ratio := float64(u) / float64(w); ratio < 0.67 || ratio > 1.5 {
			t.Errorf("timing leaks which usernames exist: unknown %s vs wrong password %s (ratio %.2f)", u, w, ratio)
		}
		t.Logf("median login: wrong password %s, unknown user %s", w, u)
	})
	t.Run("no credentials is 401", func(t *testing.T) {
		wantError(t, s.do(t, "POST", "/v1/login", "", nil), http.StatusUnauthorized)
	})
	t.Run("malformed Basic header is 401", func(t *testing.T) {
		r := s.raw(t, "POST", "/v1/login", map[string]string{"Authorization": "Basic !!!not-base64"}, nil)
		wantError(t, r, http.StatusUnauthorized)
	})

	valid := s.login(t, 1)
	for name, header := range map[string]string{
		"no token":         "",
		"garbage token":    "Bearer garbage",
		"tampered token":   "Bearer " + valid[:len(valid)-2] + "xx",
		"expired token":    "Bearer " + s.tokens.Sign(1, time.Now().Add(-time.Minute)),
		"Basic on an API":  "Basic " + base64.StdEncoding.EncodeToString([]byte(seed.Username(1)+":"+seed.Password)),
		"empty Bearer":     "Bearer ",
		"lowercase scheme": "bearer " + valid,
	} {
		t.Run(name+" is 401", func(t *testing.T) {
			headers := map[string]string{}
			if header != "" {
				headers["Authorization"] = header
			}
			wantError(t, s.raw(t, "GET", "/v1/feed", headers, nil), http.StatusUnauthorized)
		})
	}
}

func (s *suite) feed(t *testing.T) {
	r := s.do(t, "GET", "/v1/feed", s.login(t, 1), nil)
	wantStatus(t, r, http.StatusOK)
	wantJSON(t, r)
	var out struct {
		Posts []post `json:"posts"`
	}
	decode(t, r, &out)
	if len(out.Posts) != api.FeedSize {
		t.Fatalf("want %d posts, got %d", api.FeedSize, len(out.Posts))
	}
	for i, p := range out.Posts {
		wantPostShape(t, p)
		if i == 0 {
			continue
		}
		prev, cur := mustTime(t, out.Posts[i-1].CreatedAt), mustTime(t, p.CreatedAt)
		if cur.After(prev) || (cur.Equal(prev) && p.ID > out.Posts[i-1].ID) {
			t.Errorf("feed is not newest first at position %d", i)
		}
	}
}

func (s *suite) posts(t *testing.T) {
	token := s.login(t, 1)
	t.Run("existing post is 200", func(t *testing.T) {
		r := s.do(t, "GET", "/v1/posts/1", token, nil)
		wantStatus(t, r, http.StatusOK)
		wantJSON(t, r)
		var p post
		decode(t, r, &p)
		wantPostShape(t, p)
		if p.ID != 1 {
			t.Errorf("asked for post 1, got %d", p.ID)
		}
	})
	for name, path := range map[string]string{
		"missing post":   fmt.Sprintf("/v1/posts/%d", missingPost),
		"non-numeric id": "/v1/posts/abc",
		"zero id":        "/v1/posts/0",
		"negative id":    "/v1/posts/-1",
	} {
		t.Run(name+" is 404", func(t *testing.T) {
			wantError(t, s.do(t, "GET", path, token, nil), http.StatusNotFound)
		})
	}
}

func (s *suite) likes(t *testing.T) {
	alice, bob := s.login(t, 1), s.login(t, 2)
	p := s.createPost(t, alice, "a post to like")
	likePath := fmt.Sprintf("/v1/posts/%d/like", p.ID)

	t.Run("like is 204 and visible on the next read", func(t *testing.T) {
		wantStatus(t, s.do(t, "POST", likePath, alice, nil), http.StatusNoContent)
		if got := s.likeCount(t, alice, p.ID); got != 1 {
			t.Errorf("want like_count 1 right after liking, got %d", got)
		}
	})
	t.Run("liking twice doesn't double-count", func(t *testing.T) {
		wantStatus(t, s.do(t, "POST", likePath, alice, nil), http.StatusNoContent)
		if got := s.likeCount(t, alice, p.ID); got != 1 {
			t.Errorf("a repeat like changed like_count to %d", got)
		}
	})
	t.Run("another user's like counts", func(t *testing.T) {
		wantStatus(t, s.do(t, "POST", likePath, bob, nil), http.StatusNoContent)
		if got := s.likeCount(t, alice, p.ID); got != 2 {
			t.Errorf("want like_count 2, got %d", got)
		}
	})
	t.Run("liking a missing post is 404", func(t *testing.T) {
		wantError(t, s.do(t, "POST", fmt.Sprintf("/v1/posts/%d/like", missingPost), alice, nil), http.StatusNotFound)
	})
}

func (s *suite) create(t *testing.T) {
	token := s.login(t, 3)
	t.Run("valid post is 201, authored by the caller", func(t *testing.T) {
		p := s.createPost(t, token, "hello from conformance")
		if p.Author.ID != 3 || p.Author.Username != seed.Username(3) || p.LikeCount != 0 || p.Body != "hello from conformance" {
			t.Errorf("unexpected post: %+v", p)
		}
	})
	t.Run("280 multibyte characters is allowed", func(t *testing.T) {
		s.createPost(t, token, strings.Repeat("é", 280))
	})
	t.Run("new post is first in the feed", func(t *testing.T) {
		p := s.createPost(t, token, "newest")
		r := s.do(t, "GET", "/v1/feed", token, nil)
		var out struct {
			Posts []post `json:"posts"`
		}
		decode(t, r, &out)
		if len(out.Posts) == 0 || out.Posts[0].ID != p.ID {
			t.Errorf("post %d should top the feed right after it's created", p.ID)
		}
	})
	for name, body := range map[string]string{
		"empty body":     `{"body": ""}`,
		"281 characters": `{"body": "` + strings.Repeat("a", 281) + `"}`,
		"invalid JSON":   `{"body": `,
		"missing field":  `{}`,
		"unknown field":  `{"body": "hi", "admin": true}`,
		"wrong type":     `{"body": 42}`,
	} {
		t.Run(name+" is 400", func(t *testing.T) {
			wantError(t, s.do(t, "POST", "/v1/posts", token, []byte(body)), http.StatusBadRequest)
		})
	}
}

func (s *suite) service(t *testing.T) {
	t.Run("healthz is 200 without auth", func(t *testing.T) {
		wantStatus(t, s.do(t, "GET", "/healthz", "", nil), http.StatusOK)
	})
	t.Run("metrics are exposed", func(t *testing.T) {
		r := s.do(t, "GET", "/metrics", "", nil)
		wantStatus(t, r, http.StatusOK)
		for _, name := range []string{"stresslab_http_requests_total", "stresslab_db_pool_empty_acquire_wait_seconds_total", "go_goroutines"} {
			if !bytes.Contains(r.body, []byte(name)) {
				t.Errorf("/metrics is missing %s", name)
			}
		}
	})
	t.Run("unknown route is a JSON 404", func(t *testing.T) {
		wantError(t, s.do(t, "GET", "/v1/nope", "", nil), http.StatusNotFound)
	})
}

// --- helpers

func (s *suite) login(t *testing.T, userID int64) string {
	t.Helper()
	r := s.basic(t, seed.Username(userID), seed.Password)
	wantStatus(t, r, http.StatusOK)
	var out struct {
		Token string `json:"token"`
	}
	decode(t, r, &out)
	return out.Token
}

func (s *suite) basic(t *testing.T, user, pass string) response {
	t.Helper()
	auth := base64.StdEncoding.EncodeToString([]byte(user + ":" + pass))
	return s.raw(t, "POST", "/v1/login", map[string]string{"Authorization": "Basic " + auth}, nil)
}

func (s *suite) createPost(t *testing.T, token, body string) post {
	t.Helper()
	payload, _ := json.Marshal(map[string]string{"body": body})
	r := s.do(t, "POST", "/v1/posts", token, payload)
	wantStatus(t, r, http.StatusCreated)
	var p post
	decode(t, r, &p)
	wantPostShape(t, p)
	return p
}

func (s *suite) likeCount(t *testing.T, token string, id int64) int64 {
	t.Helper()
	r := s.do(t, "GET", fmt.Sprintf("/v1/posts/%d", id), token, nil)
	wantStatus(t, r, http.StatusOK)
	var p post
	decode(t, r, &p)
	return p.LikeCount
}

func (s *suite) do(t *testing.T, method, path, token string, body []byte) response {
	t.Helper()
	headers := map[string]string{}
	if token != "" {
		headers["Authorization"] = "Bearer " + token
	}
	if body != nil {
		headers["Content-Type"] = "application/json"
	}
	return s.raw(t, method, path, headers, body)
}

func (s *suite) raw(t *testing.T, method, path string, headers map[string]string, body []byte) response {
	t.Helper()
	req, err := http.NewRequest(method, s.url+path, bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	res, err := s.client.Do(req)
	if err != nil {
		t.Fatalf("%s %s: %v", method, path, err)
	}
	defer res.Body.Close()
	b, err := io.ReadAll(res.Body)
	if err != nil {
		t.Fatal(err)
	}
	return response{status: res.StatusCode, header: res.Header, body: b}
}

func (s *suite) timed(t *testing.T, f func()) time.Duration {
	t.Helper()
	start := time.Now()
	f()
	return time.Since(start)
}

func median(ds []time.Duration) time.Duration {
	s := slices.Clone(ds)
	slices.Sort(s)
	return s[len(s)/2]
}

func wantStatus(t *testing.T, r response, want int) {
	t.Helper()
	if r.status != want {
		t.Fatalf("want status %d, got %d: %s", want, r.status, r.body)
	}
}

func wantJSON(t *testing.T, r response) {
	t.Helper()
	if ct := r.header.Get("Content-Type"); ct != "application/json" {
		t.Errorf("want Content-Type application/json, got %q", ct)
	}
}

func wantError(t *testing.T, r response, status int) {
	t.Helper()
	wantStatus(t, r, status)
	wantJSON(t, r)
	var out struct {
		Error string `json:"error"`
	}
	decode(t, r, &out)
	if out.Error == "" {
		t.Errorf(`want {"error": "..."}, got %s`, r.body)
	}
}

func wantPostShape(t *testing.T, p post) {
	t.Helper()
	if p.ID < 1 || p.Author.ID < 1 || !strings.HasPrefix(p.Author.Username, "user_") ||
		p.Body == "" || p.LikeCount < 0 {
		t.Errorf("malformed post: %+v", p)
	}
	mustTime(t, p.CreatedAt)
}

func mustTime(t *testing.T, s string) time.Time {
	t.Helper()
	ts, err := time.Parse(time.RFC3339Nano, s)
	if err != nil {
		t.Fatalf("created_at %q isn't RFC 3339: %v", s, err)
	}
	return ts
}

func decode(t *testing.T, r response, v any) {
	t.Helper()
	if err := json.Unmarshal(r.body, v); err != nil {
		t.Fatalf("response isn't the expected JSON: %v\n%s", err, r.body)
	}
}
