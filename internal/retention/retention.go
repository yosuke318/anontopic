// Package retention owns scheduled deletion of expired data (rooms,
// messages, logs) according to the service's retention policy.
//
// Boundary: each module exposes its own purge operation; retention
// orchestrates them instead of deleting other modules' rows itself.
package retention
