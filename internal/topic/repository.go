package topic

import "context"

// Repository stores the topic catalogue.
//
// Implementations report a missing topic as ErrNotFound, and a topic that
// conversations still reference as ErrInUse.
type Repository interface {
	// ListActive returns the active topics in catalogue order.
	ListActive(ctx context.Context) ([]Topic, error)
	// List returns every topic in catalogue order.
	List(ctx context.Context) ([]Topic, error)
	// Create stores an active topic under name.
	Create(ctx context.Context, name string) (Topic, error)
	// Update applies the fields upd sets and returns the stored topic.
	Update(ctx context.Context, id int, upd Update) (Topic, error)
	// Delete removes the topic with the given ID.
	Delete(ctx context.Context, id int) error
}
