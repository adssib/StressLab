-- Schema v0 — step 0: the logical model from SPEC.md with only the obvious
-- indexes. Step 1 ("Queries") changes this as a named experiment.

CREATE TABLE users (
  id            bigint      PRIMARY KEY,
  username      text        NOT NULL UNIQUE,
  password_hash text        NOT NULL,
  created_at    timestamptz NOT NULL
);

CREATE TABLE posts (
  id         bigserial   PRIMARY KEY,
  author_id  bigint      NOT NULL REFERENCES users (id),
  body       text        NOT NULL CHECK (char_length(body) BETWEEN 1 AND 280),
  created_at timestamptz NOT NULL DEFAULT now()
);

-- The feed: newest first, id breaks ties.
CREATE INDEX posts_created_at_id_idx ON posts (created_at DESC, id DESC);

CREATE TABLE likes (
  user_id    bigint      NOT NULL REFERENCES users (id),
  post_id    bigint      NOT NULL REFERENCES posts (id),
  created_at timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (user_id, post_id)
);

-- Counting a post's likes.
CREATE INDEX likes_post_id_idx ON likes (post_id);
