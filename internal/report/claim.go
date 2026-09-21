package report

import (
	"context"
	"errors"
	"fmt"
	"net/mail"
	"slices"
	"strings"
	"time"
	"unicode/utf8"
)

// The rights a claim says were infringed.
const (
	// RightPrivacy is a claim that private information was disclosed.
	RightPrivacy = "privacy"
	// RightDefamation is a claim that someone's reputation was damaged.
	RightDefamation = "defamation"
	// RightCopyright is a claim that a work was copied without permission.
	RightCopyright = "copyright"
	// RightOther is every other right.
	RightOther = "other"
)

var rights = []string{
	RightPrivacy,
	RightDefamation,
	RightCopyright,
	RightOther,
}

// The lengths the fields of a claim are held to, counted in runes. The name
// and the email fit the columns they are stored in; 254 is the longest
// address a mail server accepts.
const (
	maxClaimNameRunes    = 100
	maxClaimEmailRunes   = 254
	maxClaimDetailsRunes = 4000
)

// ErrInvalidClaim is returned for a claim missing a field it needs, or holding
// a field that is too long or malformed.
var ErrInvalidClaim = errors.New("report: invalid claim")

// Claim is a request, from anyone and not only a participant, to remove what
// infringes their rights. The claimant is contacted about the outcome at the
// address they gave. The reasoning is in
// docs/adr/0022-take-rights-infringement-claims-from-anyone.md.
type Claim struct {
	ID    int64
	Name  string
	Email string
	Right string
	// Details is what the claimant says the infringing information is, where
	// they saw it and why it infringes their rights.
	Details   string
	Status    string
	CreatedAt time.Time
}

// SubmitClaim records a claim once its fields are within their bounds.
func (s *Service) SubmitClaim(ctx context.Context, c Claim) (Claim, error) {
	c.Name = strings.TrimSpace(c.Name)
	c.Email = strings.TrimSpace(c.Email)
	c.Details = strings.TrimSpace(c.Details)

	if err := validateClaim(c); err != nil {
		return Claim{}, err
	}

	c.Status = StatusOpen
	created, err := s.repo.AddClaim(ctx, c)
	if err != nil {
		return Claim{}, fmt.Errorf("add claim: %w", err)
	}
	return created, nil
}

// ListClaims returns the claims f keeps.
func (s *Service) ListClaims(ctx context.Context, f Filter) ([]Claim, error) {
	f, err := normalizeFilter(f)
	if err != nil {
		return nil, err
	}
	return s.repo.ListClaims(ctx, f)
}

// UpdateClaimStatus moves the claim id names to status.
func (s *Service) UpdateClaimStatus(ctx context.Context, id int64, status string) (Claim, error) {
	if !slices.Contains(statuses, status) {
		return Claim{}, ErrUnknownStatus
	}
	return s.repo.SetClaimStatus(ctx, id, status)
}

func validateClaim(c Claim) error {
	switch {
	case c.Name == "" || utf8.RuneCountInString(c.Name) > maxClaimNameRunes:
		return fmt.Errorf("%w: name", ErrInvalidClaim)
	case !validEmail(c.Email):
		return fmt.Errorf("%w: email", ErrInvalidClaim)
	case !slices.Contains(rights, c.Right):
		return fmt.Errorf("%w: right", ErrInvalidClaim)
	case c.Details == "" || utf8.RuneCountInString(c.Details) > maxClaimDetailsRunes:
		return fmt.Errorf("%w: details", ErrInvalidClaim)
	}
	return nil
}

// validEmail accepts a bare address, the only form an operator can reply to
// without reading it apart first.
func validEmail(email string) bool {
	if email == "" || utf8.RuneCountInString(email) > maxClaimEmailRunes {
		return false
	}
	addr, err := mail.ParseAddress(email)
	return err == nil && addr.Name == "" && addr.Address == email
}
