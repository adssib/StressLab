package seed

import (
	"context"
	"crypto/sha256"
	"fmt"
	"iter"
	"log/slog"
	"time"

	"github.com/adssib/StressLab/internal/store"
	"github.com/jackc/pgx/v5"
)

const (
	// TemplateDB holds the seeded dataset. Nothing ever writes to it after
	// Build; runs write to a fresh clone (METHODOLOGY, "One run").
	TemplateDB = "stresslab_seed"
	// RunDB is the clone the API runs against.
	RunDB = "stresslab"
)

// Build creates TemplateDB from scratch: schema, every row via COPY, the post
// ID sequence moved past the seeded IDs, and fresh planner statistics.
func Build(ctx context.Context, adminURL, schema string, seed uint64, log *slog.Logger) error {
	ddl, err := store.Schema(schema)
	if err != nil {
		return fmt.Errorf("schema %s: %w", schema, err)
	}
	if err := adminExec(ctx, adminURL,
		"DROP DATABASE IF EXISTS "+TemplateDB+" WITH (FORCE)",
		"CREATE DATABASE "+TemplateDB,
	); err != nil {
		return err
	}

	conn, err := connect(ctx, adminURL, TemplateDB)
	if err != nil {
		return err
	}
	defer conn.Close(ctx)

	if _, err := conn.Exec(ctx, ddl); err != nil {
		return fmt.Errorf("apply schema: %w", err)
	}

	users := 0
	if err := copyRows(ctx, conn, "users", []string{"id", "username", "password_hash", "created_at"},
		func(yield func([]any) bool) {
			EachUser(func(u User) bool {
				users++
				return yield([]any{u.ID, u.Username, PasswordHash, u.CreatedAt})
			})
		}, log); err != nil {
		return err
	}

	posts := GeneratePosts(seed)
	if err := copyRows(ctx, conn, "posts", []string{"id", "author_id", "body", "created_at"},
		func(yield func([]any) bool) {
			for _, p := range posts {
				if !yield([]any{p.ID, p.AuthorID, p.Body, p.CreatedAt}) {
					return
				}
			}
		}, log); err != nil {
		return err
	}

	if err := copyRows(ctx, conn, "likes", []string{"user_id", "post_id", "created_at"},
		func(yield func([]any) bool) {
			EachLike(seed, posts, func(l Like) bool {
				return yield([]any{l.UserID, l.PostID, l.CreatedAt})
			})
		}, log); err != nil {
		return err
	}

	for _, sql := range []string{
		"SELECT setval('posts_id_seq', (SELECT max(id) FROM posts))",
		"VACUUM ANALYZE",
	} {
		if _, err := conn.Exec(ctx, sql); err != nil {
			return fmt.Errorf("%s: %w", sql, err)
		}
	}
	return nil
}

// copyRows streams rows into table with COPY.
func copyRows(ctx context.Context, conn *pgx.Conn, table string, cols []string,
	rows func(yield func([]any) bool), log *slog.Logger) error {
	start := time.Now()
	next, stop := iter.Pull(iter.Seq[[]any](rows))
	defer stop()
	n, err := conn.CopyFrom(ctx, pgx.Identifier{table}, cols, pgx.CopyFromFunc(func() ([]any, error) {
		row, ok := next()
		if !ok {
			return nil, nil
		}
		return row, nil
	}))
	if err != nil {
		return fmt.Errorf("copy %s: %w", table, err)
	}
	log.Info("copied", slog.String("table", table), slog.Int64("rows", n),
		slog.String("took", time.Since(start).Round(time.Millisecond).String()))
	return nil
}

// Strategy is how Postgres clones the template: FILE_COPY copies the files
// (meant for large databases); WAL_LOG writes everything through the WAL.
type Strategy string

const (
	FileCopy Strategy = "FILE_COPY"
	WALLog   Strategy = "WAL_LOG"
)

// Reset replaces RunDB with a fresh clone of TemplateDB and reports how long
// it took — the reset before every run (METHODOLOGY, "One run").
func Reset(ctx context.Context, adminURL string, strategy Strategy) (time.Duration, error) {
	if strategy != FileCopy && strategy != WALLog {
		return 0, fmt.Errorf("unknown strategy %q", strategy)
	}
	start := time.Now()
	err := adminExec(ctx, adminURL,
		"DROP DATABASE IF EXISTS "+RunDB+" WITH (FORCE)",
		"CREATE DATABASE "+RunDB+" TEMPLATE "+TemplateDB+" STRATEGY "+string(strategy),
	)
	return time.Since(start), err
}

// Checksum hashes every table of db in primary-key order. Two databases with
// equal checksums hold identical rows.
func Checksum(ctx context.Context, adminURL, db string) (map[string]string, error) {
	conn, err := connect(ctx, adminURL, db)
	if err != nil {
		return nil, err
	}
	defer conn.Close(ctx)

	sums := map[string]string{}
	for table, order := range map[string]string{
		"users": "id",
		"posts": "id",
		"likes": "user_id, post_id",
	} {
		h := sha256.New()
		sql := fmt.Sprintf("COPY (SELECT * FROM %s ORDER BY %s) TO STDOUT", table, order)
		if _, err := conn.PgConn().CopyTo(ctx, h, sql); err != nil {
			return nil, fmt.Errorf("checksum %s: %w", table, err)
		}
		sums[table] = fmt.Sprintf("%x", h.Sum(nil))
	}
	return sums, nil
}

func connect(ctx context.Context, adminURL, db string) (*pgx.Conn, error) {
	cfg, err := pgx.ParseConfig(adminURL)
	if err != nil {
		return nil, fmt.Errorf("parse admin url: %w", err)
	}
	cfg.Database = db
	conn, err := pgx.ConnectConfig(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("connect to %s: %w", db, err)
	}
	return conn, nil
}

// adminExec runs statements against the admin database, one at a time
// (CREATE/DROP DATABASE can't run inside a transaction).
func adminExec(ctx context.Context, adminURL string, sqls ...string) error {
	cfg, err := pgx.ParseConfig(adminURL)
	if err != nil {
		return fmt.Errorf("parse admin url: %w", err)
	}
	conn, err := pgx.ConnectConfig(ctx, cfg)
	if err != nil {
		return fmt.Errorf("connect to admin db: %w", err)
	}
	defer conn.Close(ctx)
	for _, sql := range sqls {
		if _, err := conn.Exec(ctx, sql); err != nil {
			return fmt.Errorf("%s: %w", sql, err)
		}
	}
	return nil
}
