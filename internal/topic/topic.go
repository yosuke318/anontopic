// Package topic owns the catalogue of chat topics users can queue for,
// including their lifecycle (published, hidden, archived).
//
// Boundary: other modules refer to a topic by its ID, not by importing
// this package's persistence models.
package topic
