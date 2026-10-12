package store

import (
	"context"
	"embed"
	"errors"
	"time"
)

// ErrNotFound means the user or post doesn't exist.
var ErrNotFound = errors.New("not found")

//go:embed schema/*.sql
var schemas embed.FS

// Schema returns the SQL for a schema version, e.g. "v0".
func Schema(version string) (string, error) {
	b, err := schemas.ReadFile("schema/" + version + ".sql")
	if err != nil {
		return "", err
	}
	return string(b), nil
}

// Author is a post's author as the API returns it.
type Author struct {
	ID       int64
	Username string
}

// Post is one post with its author and like count.
type Post struct {
	ID        int64
	Author    Author
	Body      string
	LikeCount int64
	CreatedAt time.Time
}

// Credentials is what login needs to check a password.
type Credentials struct {
	UserID       int64
	PasswordHash []byte
}

// Store is everything the handlers need. Every method is exactly one store
// operation (SPEC F3). Later steps wrap a Store rather than editing handlers
// (SPEC F2).
type Store interface {
	// Credentials looks a user up by username for login; ErrNotFound if absent.
	Credentials(ctx context.Context, username string) (Credentials, error)
	// Feed returns the newest posts, newest first.
	Feed(ctx context.Context, limit int) ([]Post, error)
	// Post returns one post; ErrNotFound if absent.
	Post(ctx context.Context, id int64) (Post, error)
	// Like records that userID likes postID. Idempotent; ErrNotFound if the
	// post doesn't exist.
	Like(ctx context.Context, userID, postID int64) error
	// CreatePost stores a new post and returns it as the API shows it.
	CreatePost(ctx context.Context, authorID int64, body string) (Post, error)
	// Ping checks the store is reachable, for /healthz.
	Ping(ctx context.Context) error
}
