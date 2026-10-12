// Package seed builds the deterministic dataset (SPEC F4) — 50,000 users,
// 500,000 posts, ~2,000,000 likes — and the template database every run is
// cloned from. The generator is pure: the same seed always yields the same
// rows, in the same order, with no database involved.
package seed

import (
	"math"
	"math/rand/v2"
	"strconv"
	"strings"
	"time"
)

const (
	Users = 50_000
	Posts = 500_000

	// Password is every seeded user's password. These are synthetic test
	// accounts, not secrets (SPEC, Authentication).
	Password = "stresslab-password"

	// PasswordHash is bcrypt(Password) at the default cost (10), computed once.
	// A constant, because bcrypt salts are random and a fresh hash per seeding
	// would break "same seed, same rows". Logins still pay a full compare.
	PasswordHash = "$2a$10$8NmZu0Q5s36ddBuFQPtGoOlZN0HJbvyeZFaWnyYIPD2fWScjQ3jTK"
)

// Epoch is when the seeded world begins; posts start a week later.
var Epoch = time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)

// Each table draws from its own random stream, so changing how one table is
// generated never shifts another's rows.
const (
	streamAuthors = 1
	streamPosts   = 2
	streamLikes   = 3
)

func stream(seed uint64, id uint64) *rand.Rand {
	return rand.New(rand.NewPCG(seed, id))
}

// User is one users row.
type User struct {
	ID        int64
	Username  string
	CreatedAt time.Time
}

// Username is the seeded username for an ID, e.g. "user_42".
func Username(id int64) string { return "user_" + strconv.FormatInt(id, 10) }

// EachUser yields every user in ID order.
func EachUser(yield func(User) bool) {
	for id := int64(1); id <= Users; id++ {
		if !yield(User{ID: id, Username: Username(id), CreatedAt: Epoch.Add(time.Duration(id) * time.Minute)}) {
			return
		}
	}
}

// Post is one posts row.
type Post struct {
	ID        int64
	AuthorID  int64
	Body      string
	CreatedAt time.Time
}

// GeneratePosts returns every post in ID order. Time only moves forward, so
// newer IDs are newer posts, as in a real system. Authorship is skewed (Zipf):
// a few users post a lot, most post rarely.
func GeneratePosts(seed uint64) []Post {
	authors := stream(seed, streamAuthors).Perm(Users) // who is prolific, shuffled
	rng := stream(seed, streamPosts)
	zipf := rand.NewZipf(rng, 1.2, 1, Users-1)

	posts := make([]Post, Posts)
	t := Epoch.Add(7 * 24 * time.Hour)
	for i := range posts {
		t = t.Add(time.Duration(1+int(rng.ExpFloat64()*30)) * time.Second)
		posts[i] = Post{
			ID:        int64(i + 1),
			AuthorID:  int64(authors[zipf.Uint64()] + 1),
			Body:      body(rng),
			CreatedAt: t,
		}
	}
	return posts
}

// Like is one likes row.
type Like struct {
	UserID    int64
	PostID    int64
	CreatedAt time.Time
}

// likeScale sets the exponential behind ordinary posts' like counts. Rounding
// down lowers the mean, so the scale is above the ~2.9 likes a post gets.
const likeScale = 3.4

// EachLike yields every like. Most posts get a few (exponential); about 1 in
// 1,000 goes viral with 200-2,000 — the hot rows later steps hit. ~2M in total.
func EachLike(seed uint64, posts []Post, yield func(Like) bool) {
	rng := stream(seed, streamLikes)
	seen := make(map[int64]struct{})
	for _, p := range posts {
		k := int(rng.ExpFloat64() * likeScale)
		if rng.Float64() < 0.001 {
			k = 200 + rng.IntN(1801)
		}
		clear(seen)
		for len(seen) < k {
			u := int64(1 + rng.IntN(Users))
			if _, dup := seen[u]; dup {
				continue
			}
			seen[u] = struct{}{}
			at := p.CreatedAt.Add(time.Duration(1+rng.IntN(3*24*3600)) * time.Second)
			if !yield(Like{UserID: u, PostID: p.ID, CreatedAt: at}) {
				return
			}
		}
	}
}

var words = strings.Fields(`
	the a to and of in is it you that was for on are with as at be this have
	from or one had by word but not what all were we when your can said there
	use an each which she do how their if will up other about out many then
	them these so some her would make like him into time has look two more
	write go see number no way could people my than first water been call who
	oil its now find long down day did get come made may part server latency
	cache query index pool shard replica queue kafka fanout feed post load
	limit budget throughput goodput tail p95 p99 coffee deploy rollback`)

// body is 3-30 words, never longer than 280 characters.
func body(rng *rand.Rand) string {
	n := 3 + rng.IntN(28)
	var b strings.Builder
	for i := 0; i < n; i++ {
		w := words[rng.IntN(len(words))]
		if b.Len()+1+len(w) > 280 {
			break
		}
		if i > 0 {
			b.WriteByte(' ')
		}
		b.WriteString(w)
	}
	return b.String()
}

// expectedLikes is the generator's mean total: E[floor(X)] for an exponential
// X with mean likeScale is 1/(e^(1/likeScale) - 1), plus the viral posts.
var expectedLikes = int(math.Round(
	Posts*0.999/(math.Exp(1/likeScale)-1) + Posts*0.001*1100))
