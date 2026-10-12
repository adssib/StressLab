package seed

import (
	"crypto/sha256"
	"fmt"
	"hash"
	"testing"
	"unicode/utf8"

	"golang.org/x/crypto/bcrypt"
)

func TestSameSeedSameRows(t *testing.T) {
	a, b := digest(1), digest(1)
	if a != b {
		t.Fatalf("seed 1 produced different rows on two runs:\n%s\n%s", a, b)
	}
	if digest(2) == a {
		t.Fatal("seeds 1 and 2 produced identical rows")
	}
}

func TestShapeOfTheData(t *testing.T) {
	posts := GeneratePosts(1)
	if len(posts) != Posts {
		t.Fatalf("want %d posts, got %d", Posts, len(posts))
	}
	for i, p := range posts {
		if n := utf8.RuneCountInString(p.Body); n < 1 || n > 280 {
			t.Fatalf("post %d body is %d characters", p.ID, n)
		}
		if p.AuthorID < 1 || p.AuthorID > Users {
			t.Fatalf("post %d has author %d", p.ID, p.AuthorID)
		}
		if i > 0 && !p.CreatedAt.After(posts[i-1].CreatedAt) {
			t.Fatalf("post %d is not newer than post %d", p.ID, posts[i-1].ID)
		}
	}

	likes := 0
	EachLike(1, posts, func(Like) bool { likes++; return true })
	if lo, hi := expectedLikes*95/100, expectedLikes*105/100; likes < lo || likes > hi {
		t.Fatalf("want ~%d likes (±5%%), got %d", expectedLikes, likes)
	}
	if likes < 1_900_000 || likes > 2_100_000 {
		t.Fatalf("SPEC F4 says ~2,000,000 likes, got %d", likes)
	}
	t.Logf("users %d, posts %d, likes %d", Users, len(posts), likes)
}

func TestPasswordHashMatchesPassword(t *testing.T) {
	if err := bcrypt.CompareHashAndPassword([]byte(PasswordHash), []byte(Password)); err != nil {
		t.Fatalf("PasswordHash doesn't match Password: %v", err)
	}
	cost, err := bcrypt.Cost([]byte(PasswordHash))
	if err != nil || cost != bcrypt.DefaultCost {
		t.Fatalf("want cost %d (the API's dummy compare uses it), got %d, %v", bcrypt.DefaultCost, cost, err)
	}
}

func digest(seed uint64) string {
	h := sha256.New()
	EachUser(func(u User) bool { write(h, u); return true })
	posts := GeneratePosts(seed)
	for _, p := range posts {
		write(h, p)
	}
	EachLike(seed, posts, func(l Like) bool { write(h, l); return true })
	return fmt.Sprintf("%x", h.Sum(nil))
}

func write(h hash.Hash, v any) { fmt.Fprintf(h, "%v\n", v) }
