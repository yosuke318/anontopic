// Package moderation owns automated content checks applied to messages
// and the resulting enforcement actions.
//
// Boundary: it consumes message payloads passed in by the caller and does
// not read another module's storage on its own.
package moderation
