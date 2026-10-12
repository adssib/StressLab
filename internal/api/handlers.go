package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/adssib/StressLab/internal/store"
	"golang.org/x/crypto/bcrypt"
)

type userKey struct{}

// authed rejects a request without a valid bearer token, and otherwise passes
// the caller's user ID along. It never touches the store.
func (s *Server) authed(next http.HandlerFunc) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		token, ok := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
		if !ok {
			writeError(w, http.StatusUnauthorized, "unauthorized")
			return
		}
		userID, err := s.tokens.Verify(token)
		if err != nil {
			writeError(w, http.StatusUnauthorized, "unauthorized")
			return
		}
		if info, ok := r.Context().Value(infoKey{}).(*requestInfo); ok {
			info.userID = userID
		}
		next(w, r.WithContext(context.WithValue(r.Context(), userKey{}, userID)))
	})
}

func callerID(r *http.Request) int64 { return r.Context().Value(userKey{}).(int64) }

// login checks Basic credentials and issues a token. An unknown username runs
// a bcrypt compare against a throwaway hash, so it takes as long as a wrong
// password and the response time can't reveal which usernames exist.
func (s *Server) login(w http.ResponseWriter, r *http.Request) {
	username, password, ok := r.BasicAuth()
	if !ok {
		writeError(w, http.StatusUnauthorized, "invalid credentials")
		return
	}
	creds, err := s.store.Credentials(r.Context(), username)
	switch {
	case errors.Is(err, store.ErrNotFound):
		_ = bcrypt.CompareHashAndPassword(s.dummyHash, []byte(password))
		writeError(w, http.StatusUnauthorized, "invalid credentials")
		return
	case err != nil:
		s.internalError(w, r, err)
		return
	}
	if bcrypt.CompareHashAndPassword(creds.PasswordHash, []byte(password)) != nil {
		writeError(w, http.StatusUnauthorized, "invalid credentials")
		return
	}
	token, exp := s.tokens.Issue(creds.UserID)
	writeJSON(w, http.StatusOK, map[string]string{
		"token":      token,
		"expires_at": exp.UTC().Format(time.RFC3339),
	})
}

func (s *Server) feed(w http.ResponseWriter, r *http.Request) {
	posts, err := s.store.Feed(r.Context(), FeedSize)
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	out := make([]postJSON, len(posts))
	for i, p := range posts {
		out[i] = toJSON(p)
	}
	writeJSON(w, http.StatusOK, map[string][]postJSON{"posts": out})
}

func (s *Server) getPost(w http.ResponseWriter, r *http.Request) {
	id, ok := postID(r)
	if !ok {
		writeError(w, http.StatusNotFound, "post not found")
		return
	}
	post, err := s.store.Post(r.Context(), id)
	switch {
	case errors.Is(err, store.ErrNotFound):
		writeError(w, http.StatusNotFound, "post not found")
	case err != nil:
		s.internalError(w, r, err)
	default:
		writeJSON(w, http.StatusOK, toJSON(post))
	}
}

func (s *Server) like(w http.ResponseWriter, r *http.Request) {
	id, ok := postID(r)
	if !ok {
		writeError(w, http.StatusNotFound, "post not found")
		return
	}
	err := s.store.Like(r.Context(), callerID(r), id)
	switch {
	case errors.Is(err, store.ErrNotFound):
		writeError(w, http.StatusNotFound, "post not found")
	case err != nil:
		s.internalError(w, r, err)
	default:
		w.WriteHeader(http.StatusNoContent)
	}
}

// MaxBodyChars is the longest post body (SPEC, E4).
const MaxBodyChars = 280

func (s *Server) createPost(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Body *string `json:"body"`
	}
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&in); err != nil || in.Body == nil {
		writeError(w, http.StatusBadRequest, `body must be JSON: {"body": "..."}`)
		return
	}
	if n := utf8.RuneCountInString(*in.Body); n < 1 || n > MaxBodyChars {
		writeError(w, http.StatusBadRequest, "body must be 1-280 characters")
		return
	}
	post, err := s.store.CreatePost(r.Context(), callerID(r), *in.Body)
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, toJSON(post))
}

func (s *Server) healthz(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), time.Second)
	defer cancel()
	if err := s.store.Ping(ctx); err != nil {
		writeError(w, http.StatusServiceUnavailable, "store unavailable")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// postID parses {id}; anything that isn't a positive integer is "not found".
func postID(r *http.Request) (int64, bool) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	return id, err == nil && id > 0
}

// postJSON is the one Post shape (SPEC, "Endpoints").
type postJSON struct {
	ID        int64      `json:"id"`
	Author    authorJSON `json:"author"`
	Body      string     `json:"body"`
	LikeCount int64      `json:"like_count"`
	CreatedAt string     `json:"created_at"`
}

type authorJSON struct {
	ID       int64  `json:"id"`
	Username string `json:"username"`
}

func toJSON(p store.Post) postJSON {
	return postJSON{
		ID:        p.ID,
		Author:    authorJSON{ID: p.Author.ID, Username: p.Author.Username},
		Body:      p.Body,
		LikeCount: p.LikeCount,
		CreatedAt: p.CreatedAt.UTC().Format(time.RFC3339Nano),
	}
}
