// Package config loads the API's config.yaml: the listen address, the pool
// size, and every ladder switch (SPEC.md, "The ladder"). Every switch defaults
// to its baseline value, so an empty file is step 0. Secrets never live here —
// they come from environment variables.
package config

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"time"

	"go.yaml.in/yaml/v3"
)

// Config is the whole of config.yaml.
type Config struct {
	Listen string `yaml:"listen"`
	Schema string `yaml:"schema"` // step 1
	Cache  string `yaml:"cache"`  // step 4
	Chaos  string `yaml:"chaos"`  // step 10

	DB     DB     `yaml:"db"`
	API    API    `yaml:"api"`
	Writes Writes `yaml:"writes"`
	Feed   Feed   `yaml:"feed"`
	Auth   Auth   `yaml:"auth"`
}

type DB struct {
	PoolSize int    `yaml:"pool_size"` // step 2
	Pooler   string `yaml:"pooler"`    // step 2
	Host     string `yaml:"host"`      // step 5
	Reads    string `yaml:"reads"`     // step 6
}

type API struct {
	Instances int    `yaml:"instances"` // step 7
	Shedding  string `yaml:"shedding"`  // step 10
}

type Writes struct {
	Likes string `yaml:"likes"` // step 8
}

type Feed struct {
	Mode string `yaml:"mode"` // step 9
}

type Auth struct {
	TokenTTL time.Duration `yaml:"token_ttl"`
}

// Default is step 0: every switch at its baseline value.
func Default() Config {
	return Config{
		Listen: ":8080",
		Schema: "v0",
		Cache:  "none",
		Chaos:  "none",
		DB:     DB{PoolSize: 10, Pooler: "none", Host: "local", Reads: "primary"},
		API:    API{Instances: 1, Shedding: "off"},
		Writes: Writes{Likes: "sync"},
		Feed:   Feed{Mode: "global"},
		Auth:   Auth{TokenTTL: time.Hour},
	}
}

// Load reads path over the defaults. An empty path means pure defaults.
// Unknown keys are an error, so a typo can't silently leave a switch at its
// baseline.
func Load(path string) (Config, error) {
	cfg := Default()
	if path == "" {
		return cfg, cfg.Validate()
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return Config{}, fmt.Errorf("read config: %w", err)
	}
	dec := yaml.NewDecoder(bytes.NewReader(raw))
	dec.KnownFields(true)
	if err := dec.Decode(&cfg); err != nil && !errors.Is(err, io.EOF) {
		return Config{}, fmt.Errorf("parse %s: %w", path, err)
	}
	return cfg, cfg.Validate()
}

// Validate rejects values the code doesn't implement yet. Each switch names
// the ladder step that will add its other values.
func (c Config) Validate() error {
	var errs []error
	if c.Listen == "" {
		errs = append(errs, errors.New("listen: must be set"))
	}
	if c.DB.PoolSize < 1 {
		errs = append(errs, fmt.Errorf("db.pool_size: must be at least 1, got %d", c.DB.PoolSize))
	}
	if c.API.Instances != 1 {
		errs = append(errs, fmt.Errorf("api.instances: only 1 until step 7, got %d", c.API.Instances))
	}
	if c.Auth.TokenTTL <= 0 {
		errs = append(errs, fmt.Errorf("auth.token_ttl: must be positive, got %s", c.Auth.TokenTTL))
	}
	for _, s := range []struct{ key, got, want, step string }{
		{"schema", c.Schema, "v0", "1"},
		{"cache", c.Cache, "none", "4"},
		{"chaos", c.Chaos, "none", "10"},
		{"db.pooler", c.DB.Pooler, "none", "2"},
		{"db.host", c.DB.Host, "local", "5"},
		{"db.reads", c.DB.Reads, "primary", "6"},
		{"api.shedding", c.API.Shedding, "off", "10"},
		{"writes.likes", c.Writes.Likes, "sync", "8"},
		{"feed.mode", c.Feed.Mode, "global", "9"},
	} {
		if s.got != s.want {
			errs = append(errs, fmt.Errorf("%s: only %q until step %s, got %q", s.key, s.want, s.step, s.got))
		}
	}
	return errors.Join(errs...)
}
