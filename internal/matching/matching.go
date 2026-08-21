// Package matching owns the waiting queue and the rule that forms
// 2-3 person rooms out of users waiting on the same topic.
//
// Boundary: it may hand room creation requests to other modules through
// interfaces, but never touches their persistence models directly.
package matching
