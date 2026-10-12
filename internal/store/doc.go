// Package store defines the Store interface and the step-0 Postgres store.
// Later steps add wrappers (cache, read router, events) as sub-packages, each
// taking the next Store in the chain (ARCHITECTURE.md, "how a step plugs in").
package store
