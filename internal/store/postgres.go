package store

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Postgres is the step-0 store: one SQL statement per method, over a pgx pool.
type Postgres struct {
	pool *pgxpool.Pool
}

// NewPostgres opens a pool of at most poolSize connections.
func NewPostgres(ctx context.Context, url string, poolSize int) (*Postgres, error) {
	cfg, err := pgxpool.ParseConfig(url)
	if err != nil {
		return nil, fmt.Errorf("parse database url: %w", err)
	}
	cfg.MaxConns = int32(poolSize)
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("open pool: %w", err)
	}
	return &Postgres{pool: pool}, nil
}

// Pool exposes the pool for metrics (pool wait time, connections in use).
func (p *Postgres) Pool() *pgxpool.Pool { return p.pool }

// Close releases every connection.
func (p *Postgres) Close() { p.pool.Close() }

const postColumns = `
  p.id, p.body, p.created_at, u.id, u.username,
  (SELECT count(*) FROM likes l WHERE l.post_id = p.id) AS like_count`

const feedSQL = `SELECT` + postColumns + `
FROM posts p JOIN users u ON u.id = p.author_id
ORDER BY p.created_at DESC, p.id DESC
LIMIT $1`

const postSQL = `SELECT` + postColumns + `
FROM posts p JOIN users u ON u.id = p.author_id
WHERE p.id = $1`

// One statement: the CTE finds the post, inserts the like only if the post
// exists, ignores a repeat like, and reports whether the post was there.
const likeSQL = `
WITH post AS (SELECT id FROM posts WHERE id = $2),
     ins  AS (INSERT INTO likes (user_id, post_id)
              SELECT $1, id FROM post
              ON CONFLICT DO NOTHING)
SELECT EXISTS (SELECT 1 FROM post)`

const createPostSQL = `
WITH p AS (INSERT INTO posts (author_id, body) VALUES ($1, $2)
           RETURNING id, author_id, body, created_at)
SELECT p.id, p.body, p.created_at, u.id, u.username, 0::bigint
FROM p JOIN users u ON u.id = p.author_id`

const credentialsSQL = `SELECT id, password_hash FROM users WHERE username = $1`

func (p *Postgres) Credentials(ctx context.Context, username string) (Credentials, error) {
	var c Credentials
	var hash string
	err := p.pool.QueryRow(ctx, credentialsSQL, username).Scan(&c.UserID, &hash)
	if errors.Is(err, pgx.ErrNoRows) {
		return Credentials{}, ErrNotFound
	}
	if err != nil {
		return Credentials{}, fmt.Errorf("credentials: %w", err)
	}
	c.PasswordHash = []byte(hash)
	return c, nil
}

func (p *Postgres) Feed(ctx context.Context, limit int) ([]Post, error) {
	rows, err := p.pool.Query(ctx, feedSQL, limit)
	if err != nil {
		return nil, fmt.Errorf("feed: %w", err)
	}
	posts, err := pgx.CollectRows(rows, scanPost)
	if err != nil {
		return nil, fmt.Errorf("feed: %w", err)
	}
	return posts, nil
}

func (p *Postgres) Post(ctx context.Context, id int64) (Post, error) {
	rows, err := p.pool.Query(ctx, postSQL, id)
	if err != nil {
		return Post{}, fmt.Errorf("post %d: %w", id, err)
	}
	post, err := pgx.CollectExactlyOneRow(rows, scanPost)
	if errors.Is(err, pgx.ErrNoRows) {
		return Post{}, ErrNotFound
	}
	if err != nil {
		return Post{}, fmt.Errorf("post %d: %w", id, err)
	}
	return post, nil
}

func (p *Postgres) Like(ctx context.Context, userID, postID int64) error {
	var exists bool
	if err := p.pool.QueryRow(ctx, likeSQL, userID, postID).Scan(&exists); err != nil {
		return fmt.Errorf("like %d: %w", postID, err)
	}
	if !exists {
		return ErrNotFound
	}
	return nil
}

func (p *Postgres) CreatePost(ctx context.Context, authorID int64, body string) (Post, error) {
	rows, err := p.pool.Query(ctx, createPostSQL, authorID, body)
	if err != nil {
		return Post{}, fmt.Errorf("create post: %w", err)
	}
	post, err := pgx.CollectExactlyOneRow(rows, scanPost)
	if err != nil {
		return Post{}, fmt.Errorf("create post: %w", err)
	}
	return post, nil
}

func (p *Postgres) Ping(ctx context.Context) error {
	return p.pool.Ping(ctx)
}

func scanPost(row pgx.CollectableRow) (Post, error) {
	var post Post
	err := row.Scan(&post.ID, &post.Body, &post.CreatedAt,
		&post.Author.ID, &post.Author.Username, &post.LikeCount)
	return post, err
}
