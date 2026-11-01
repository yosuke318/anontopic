package session

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"slices"
)

// The gender and the age band a user may give are optional and self-declared.
// They are recorded with the participants of a conversation for statistics
// and moderation, and are never used to pick a partner or shown to one. The
// reasoning is in
// docs/adr/0044-record-the-gender-and-age-band-a-user-chose-to-give-without-matching-on-them.md.

// ErrInvalidAttributes is returned for a gender or an age band that is not
// one of the values below. Callers translate it into 400 for the client.
var ErrInvalidAttributes = errors.New("session: invalid attributes")

// genders and ageBands are the values the client may send. They match the
// CHECK constraints on conversation_participants.
var (
	genders  = []string{"female", "male", "other"}
	ageBands = []string{"under_18", "18_19", "20s", "30s", "40s", "50s", "60_plus"}
)

// Attributes is what a user chose to tell about themselves. A field is empty
// when the user did not answer it.
type Attributes struct {
	Gender  string
	AgeBand string
}

// validate reports ErrInvalidAttributes for a value the client may not send.
func (a Attributes) validate() error {
	if a.Gender != "" && !slices.Contains(genders, a.Gender) {
		return fmt.Errorf("%w: gender %q", ErrInvalidAttributes, a.Gender)
	}
	if a.AgeBand != "" && !slices.Contains(ageBands, a.AgeBand) {
		return fmt.Errorf("%w: age band %q", ErrInvalidAttributes, a.AgeBand)
	}
	return nil
}

// SetAttributes replaces the attributes of the session behind token. Empty
// fields clear what the user answered before.
func (s *Service) SetAttributes(ctx context.Context, token string, attrs Attributes) error {
	if !wellFormed(token) {
		return ErrInvalidSession
	}
	if err := attrs.validate(); err != nil {
		return err
	}

	err := s.store.SetAttributes(ctx, token, attrs)
	if errors.Is(err, ErrNotStored) {
		return ErrInvalidSession
	}
	if err != nil {
		return fmt.Errorf("store attributes: %w", err)
	}
	return nil
}

// attributesRequest is the optional body of POST /api/session. A field that
// is missing, null or empty means the user did not answer it.
type attributesRequest struct {
	Gender  *string `json:"gender"`
	AgeBand *string `json:"age_band"`
}

// maxAttributesBytes bounds the body of POST /api/session.
const maxAttributesBytes = 1 << 10

// readAttributes reads the attributes the request body carries. It reports
// false when the request has no body, which leaves the attributes of an
// existing session as they are.
func readAttributes(w http.ResponseWriter, r *http.Request) (Attributes, bool, error) {
	if r.Body == nil {
		return Attributes{}, false, nil
	}

	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxAttributesBytes))
	dec.DisallowUnknownFields()

	var req attributesRequest
	if err := dec.Decode(&req); err != nil {
		if errors.Is(err, io.EOF) {
			return Attributes{}, false, nil
		}
		return Attributes{}, false, fmt.Errorf("%w: %w", ErrInvalidAttributes, err)
	}
	if dec.More() {
		return Attributes{}, false, fmt.Errorf("%w: more than one value in the body", ErrInvalidAttributes)
	}

	attrs := Attributes{}
	if req.Gender != nil {
		attrs.Gender = *req.Gender
	}
	if req.AgeBand != nil {
		attrs.AgeBand = *req.AgeBand
	}
	if err := attrs.validate(); err != nil {
		return Attributes{}, false, err
	}
	return attrs, true, nil
}
