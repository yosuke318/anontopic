// Package adminauth guards the administration endpoints of every module with
// the shared token the operators hold. The reasoning is in
// docs/adr/0007-guard-the-admin-api-with-a-static-bearer-token.md.
//
// It is not a module of its own: it owns no table and no state, and exists so
// that each module's handler checks the token the same way.
package adminauth

import (
	"crypto/sha256"
	"crypto/subtle"
	"net/http"
	"strings"
)

// Scheme prefixes the admin token in the Authorization header.
const Scheme = "Bearer "

// Guard rejects the requests that do not carry the admin token.
type Guard struct {
	// enabled says whether a token is configured, and sum is its digest.
	// Holding the digest rather than the token gives the check two values of
	// equal length to compare, whatever the caller presents.
	enabled bool
	sum     [sha256.Size]byte
}

// New builds a guard around token. An empty token builds a guard that is not
// enabled, which a handler takes as the sign not to serve its administration
// endpoints at all.
func New(token string) Guard {
	return Guard{enabled: token != "", sum: sha256.Sum256([]byte(token))}
}

// Enabled reports whether a token is configured.
func (g Guard) Enabled() bool {
	return g.enabled
}

// Require passes on the requests that carry the admin token and answers the
// rest with 401.
func (g Guard) Require(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		presented, hasScheme := strings.CutPrefix(r.Header.Get("Authorization"), Scheme)

		// Both sides are compared as digests, which are always the same
		// length: ConstantTimeCompare gives up as soon as the lengths differ,
		// so comparing the tokens themselves would time out faster for a
		// guess of the wrong length and leak how long the secret is.
		presentedSum := sha256.Sum256([]byte(presented))
		if !g.enabled || !hasScheme || subtle.ConstantTimeCompare(presentedSum[:], g.sum[:]) != 1 {
			w.Header().Set("WWW-Authenticate", "Bearer")
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}

		next.ServeHTTP(w, r)
	})
}
