// Package conformance is the suite every step must pass before it's
// load-tested (SPEC F6): every endpoint, status code, auth rule and error case,
// the consistency contract, and byte-identical feed and post bodies against
// the step-0 golden files.
//
// It runs against a live API on a freshly reset database:
//
//	seed reset            # fresh clone of the template
//	(restart the API)
//	STRESSLAB_URL=http://localhost:8080 STRESSLAB_TOKEN_KEY=... go test ./conformance -count=1 -v
//
// STRESSLAB_UPDATE_GOLDEN=1 rewrites the golden files — only ever from step 0.
// Without STRESSLAB_URL the suite skips, so `go test ./...` stays offline.
package conformance
