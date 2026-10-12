// Package api holds the HTTP handlers and the auth middleware. Handlers never
// change between ladder steps; a step plugs in by wrapping the store or adding
// middleware behind a config switch (SPEC F2).
package api
