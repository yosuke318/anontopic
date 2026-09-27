package report

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"time"
	"unicode/utf8"
)

// The identifier types a sanction applies to, as banned_identifiers holds
// them.
const (
	// IdentifierIPHash is the keyed hash of the address a client connects
	// from. Everyone behind one shared address has the same one.
	IdentifierIPHash = "ip_hash"
	// IdentifierDevice is the device ID the server hands a browser in a
	// cookie of its own.
	IdentifierDevice = "device_fingerprint"
)

var identifierTypes = []string{IdentifierIPHash, IdentifierDevice}

// The stages of a sanction, from the lightest.
const (
	// SanctionWarning is shown in the chat screen and bars nothing.
	SanctionWarning = "warning"
	// SanctionSuspension bars the identifier until BannedUntil.
	SanctionSuspension = "suspension"
	// SanctionPermanent bars the identifier with no end.
	SanctionPermanent = "permanent"
)

var sanctions = []string{SanctionWarning, SanctionSuspension, SanctionPermanent}

// The sources a sanction comes from.
const (
	// SourceNGWord is a sanction for sending messages the filter blocked.
	SourceNGWord = "ng_word"
	// SourceReport is a sanction for being reported in several conversations.
	SourceReport = "report"
	// SourceOperator is a sanction an operator imposed.
	SourceOperator = "operator"
)

const (
	// DefaultBanCacheTTL is how long whether an identifier is banned is kept
	// in Redis. Imposing and lifting a ban clear what is kept, so the TTL only
	// bounds how long a read racing a change can keep the old answer.
	DefaultBanCacheTTL = time.Minute

	// DefaultBlockedThreshold is how many blocked messages within
	// DefaultBlockedWindow lead to the next sanction.
	DefaultBlockedThreshold = 5
	// DefaultBlockedWindow is the window blocked messages are counted in.
	DefaultBlockedWindow = time.Hour

	// DefaultReportedThreshold is how many reported conversations within
	// DefaultReportedWindow lead to the next sanction. Reports are counted
	// per conversation, so that the participants of one room cannot add up
	// to a sanction on their own.
	DefaultReportedThreshold = 3
	// DefaultReportedWindow is the window reported conversations are counted
	// in.
	DefaultReportedWindow = 7 * 24 * time.Hour

	// DefaultSuspension is how long a suspension imposed automatically lasts.
	DefaultSuspension = 24 * time.Hour

	// DefaultWarningTTL is how long a warning waits to be shown in the chat
	// screen.
	DefaultWarningTTL = 7 * 24 * time.Hour

	// MaxSuspension is the longest suspension an operator can impose. Longer
	// than that is a permanent ban.
	MaxSuspension = 365 * 24 * time.Hour

	// maxBanReasonRunes is what banned_identifiers.reason holds.
	maxBanReasonRunes = 100
)

var (
	// ErrUnknownIdentifierType is returned for an identifier type outside the
	// fixed set.
	ErrUnknownIdentifierType = errors.New("report: unknown identifier type")

	// ErrUnknownSanction is returned for a sanction outside the fixed set.
	ErrUnknownSanction = errors.New("report: unknown sanction")

	// ErrInvalidSuspension is returned for a suspension without a length, or
	// one longer than MaxSuspension, and for a length given to a sanction
	// other than a suspension.
	ErrInvalidSuspension = errors.New("report: invalid suspension length")

	// ErrReasonTooLong is returned for a reason banned_identifiers cannot hold.
	ErrReasonTooLong = errors.New("report: reason too long")

	// ErrUnknownParticipant is returned for a participant number the
	// conversation does not have.
	ErrUnknownParticipant = errors.New("report: unknown participant")

	// ErrNoIdentifier is returned when the participant has no identifier of
	// the type asked for recorded.
	ErrNoIdentifier = errors.New("report: no identifier recorded for the participant")

	// ErrAlreadyLifted is returned for lifting a ban that was lifted before.
	ErrAlreadyLifted = errors.New("report: already lifted")
)

// Identity is what one client is told apart by. Either field is empty when
// it is not known.
type Identity struct {
	IPHash string
	Device string
}

// identifier returns the value of one identifier type.
func (id Identity) identifier(identifierType string) string {
	switch identifierType {
	case IdentifierIPHash:
		return id.IPHash
	case IdentifierDevice:
		return id.Device
	default:
		return ""
	}
}

// Ban is one row of banned_identifiers: a sanction on one identifier.
type Ban struct {
	ID             int64
	IdentifierType string
	Identifier     string
	Sanction       string
	Source         string
	Reason         string
	// ConversationID is the conversation the sanction came from, or empty.
	ConversationID string
	// BannedUntil is the zero time for a warning and a permanent ban.
	BannedUntil time.Time
	CreatedAt   time.Time
	// LiftedAt is the zero time for a sanction an operator has not lifted.
	LiftedAt time.Time
}

// BanFilter narrows a list of bans. Lists are ordered newest first.
type BanFilter struct {
	// Active keeps the suspensions and permanent bans still in force.
	Active bool
	// BeforeID keeps the bans older than the one it names.
	BeforeID int64
	// Limit is how many bans the list holds at most. The service always sets
	// it.
	Limit int
}

// BanRepository stores the sanctions in banned_identifiers.
type BanRepository interface {
	// AddBan records b and returns it with its ID and the time it was
	// recorded.
	AddBan(ctx context.Context, b Ban) (Ban, error)

	// BannedUntil reports whether a suspension or a permanent ban that is
	// not lifted is in force on the identifier at now, and when the last of
	// them ends. The end is the zero time when one of them has none.
	BannedUntil(ctx context.Context, identifierType, identifier string, now time.Time) (bool, time.Time, error)

	// CountSanctions counts the sanctions on the identifier that are not
	// lifted, warnings included.
	CountSanctions(ctx context.Context, identifierType, identifier string) (int, error)

	// ListBans returns the bans f keeps. Active is judged at now.
	ListBans(ctx context.Context, f BanFilter, now time.Time) ([]Ban, error)

	// GetBan returns the ban id names, or ErrNotFound.
	GetBan(ctx context.Context, id int64) (Ban, error)

	// LiftBan records that the ban id names was lifted at, and returns it as
	// it is afterwards, or ErrNotFound.
	LiftBan(ctx context.Context, id int64, at time.Time) (Ban, error)
}

// SanctionStore keeps in Redis what is read on every connection and what is
// counted towards the next sanction.
type SanctionStore interface {
	// CachedBan returns whether the identifier was found banned, and ok false
	// when nothing is kept for it.
	CachedBan(ctx context.Context, identifierType, identifier string) (banned, ok bool, err error)
	// CacheBan keeps whether the identifier is banned for ttl.
	CacheBan(ctx context.Context, identifierType, identifier string, banned bool, ttl time.Duration) error
	// ForgetBan drops what CacheBan kept for the identifier.
	ForgetBan(ctx context.Context, identifierType, identifier string) error

	// AddBlocked counts one blocked message of the identifier and returns
	// how many were counted within window.
	AddBlocked(ctx context.Context, identifierType, identifier string, window time.Duration) (int, error)
	// AddReported counts the conversation as one the identifier was reported
	// in and returns how many different conversations were counted within
	// window.
	AddReported(ctx context.Context, identifierType, identifier, conversationID string, window time.Duration) (int, error)
	// ResetOffences drops both counts of the identifier.
	ResetOffences(ctx context.Context, identifierType, identifier string) error

	// MarkWarning keeps a warning for the identifier to be shown for ttl.
	MarkWarning(ctx context.Context, identifierType, identifier string, ttl time.Duration) error
	// TakeWarning reports whether a warning waits for the identifier, and
	// drops it.
	TakeWarning(ctx context.Context, identifierType, identifier string) (bool, error)
}

// BanOptions configures Bans. The zero value of each field selects the
// default described on the field.
type BanOptions struct {
	// CacheTTL defaults to DefaultBanCacheTTL.
	CacheTTL time.Duration
	// BlockedThreshold defaults to DefaultBlockedThreshold.
	BlockedThreshold int
	// BlockedWindow defaults to DefaultBlockedWindow.
	BlockedWindow time.Duration
	// ReportedThreshold defaults to DefaultReportedThreshold.
	ReportedThreshold int
	// ReportedWindow defaults to DefaultReportedWindow.
	ReportedWindow time.Duration
	// Suspension defaults to DefaultSuspension.
	Suspension time.Duration
	// WarningTTL defaults to DefaultWarningTTL.
	WarningTTL time.Duration
}

// Bans keeps the ban list: it answers whether a client is barred, moves an
// identifier up the stages of sanction as offences are counted against it,
// and carries the sanctions operators impose and lift.
//
// Sanctions imposed automatically apply to the device ID only, because an
// address can be shared by everyone on one mobile carrier gateway or one
// school network. A ban on an address is imposed by an operator who has read
// the conversation. The reasoning is in
// docs/adr/0025-escalate-sanctions-on-the-device-id-and-leave-address-bans-to-operators.md.
type Bans struct {
	repo  BanRepository
	store SanctionStore
	opts  BanOptions
	now   func() time.Time
}

// NewBans builds the ban list on top of repo and store.
func NewBans(repo BanRepository, store SanctionStore, opts BanOptions) *Bans {
	if opts.CacheTTL <= 0 {
		opts.CacheTTL = DefaultBanCacheTTL
	}
	if opts.BlockedThreshold <= 0 {
		opts.BlockedThreshold = DefaultBlockedThreshold
	}
	if opts.BlockedWindow <= 0 {
		opts.BlockedWindow = DefaultBlockedWindow
	}
	if opts.ReportedThreshold <= 0 {
		opts.ReportedThreshold = DefaultReportedThreshold
	}
	if opts.ReportedWindow <= 0 {
		opts.ReportedWindow = DefaultReportedWindow
	}
	if opts.Suspension <= 0 {
		opts.Suspension = DefaultSuspension
	}
	if opts.WarningTTL <= 0 {
		opts.WarningTTL = DefaultWarningTTL
	}

	return &Bans{repo: repo, store: store, opts: opts, now: time.Now}
}

// IsBanned reports whether a ban is in force on any identifier of id.
func (b *Bans) IsBanned(ctx context.Context, id Identity) (bool, error) {
	for _, identifierType := range identifierTypes {
		identifier := id.identifier(identifierType)
		if identifier == "" {
			continue
		}

		banned, err := b.isBanned(ctx, identifierType, identifier)
		if err != nil || banned {
			return banned, err
		}
	}
	return false, nil
}

// isBanned answers for one identifier out of Redis, and reads the database
// when Redis holds nothing for it. A Redis that cannot be read sends every
// check to the database rather than letting a banned client through.
func (b *Bans) isBanned(ctx context.Context, identifierType, identifier string) (bool, error) {
	banned, ok, err := b.store.CachedBan(ctx, identifierType, identifier)
	if err != nil {
		slog.Warn("read the cached ban", slog.Any("error", err))
	}
	if err == nil && ok {
		return banned, nil
	}

	now := b.now().UTC()
	banned, until, err := b.repo.BannedUntil(ctx, identifierType, identifier, now)
	if err != nil {
		return false, fmt.Errorf("read ban list: %w", err)
	}

	// A suspension is kept for at most what is left of it, so that it lifts
	// on time without anything clearing it.
	ttl := b.opts.CacheTTL
	if banned && !until.IsZero() {
		ttl = max(min(ttl, until.Sub(now)), time.Second)
	}
	if err := b.store.CacheBan(ctx, identifierType, identifier, banned, ttl); err != nil {
		slog.Warn("cache the ban", slog.Any("error", err))
	}
	return banned, nil
}

// RecordBlocked counts one message the filter blocked against the device of
// id, and returns the sanction that led to, or an empty string when it led to
// none.
func (b *Bans) RecordBlocked(ctx context.Context, id Identity) (string, error) {
	if id.Device == "" {
		return "", nil
	}

	n, err := b.store.AddBlocked(ctx, IdentifierDevice, id.Device, b.opts.BlockedWindow)
	if err != nil {
		return "", fmt.Errorf("count blocked message: %w", err)
	}
	if n < b.opts.BlockedThreshold {
		return "", nil
	}

	return b.escalate(ctx, id.Device, SourceNGWord, "")
}

// RecordReported counts conversationID as a conversation the device of id was
// reported in, and returns the sanction that led to, or an empty string when
// it led to none.
func (b *Bans) RecordReported(ctx context.Context, id Identity, conversationID string) (string, error) {
	if id.Device == "" {
		return "", nil
	}

	n, err := b.store.AddReported(ctx, IdentifierDevice, id.Device, conversationID, b.opts.ReportedWindow)
	if err != nil {
		return "", fmt.Errorf("count reported conversation: %w", err)
	}
	if n < b.opts.ReportedThreshold {
		return "", nil
	}

	return b.escalate(ctx, id.Device, SourceReport, conversationID)
}

// escalate imposes the next stage of sanction on a device: a warning first,
// then a suspension, then a permanent ban. The offences that led to it are
// dropped, so that the next stage takes as many again.
func (b *Bans) escalate(ctx context.Context, device, source, conversationID string) (string, error) {
	n, err := b.repo.CountSanctions(ctx, IdentifierDevice, device)
	if err != nil {
		return "", fmt.Errorf("count sanctions: %w", err)
	}

	ban := Ban{
		IdentifierType: IdentifierDevice,
		Identifier:     device,
		Source:         source,
		ConversationID: conversationID,
	}
	switch n {
	case 0:
		ban.Sanction = SanctionWarning
	case 1:
		ban.Sanction = SanctionSuspension
		ban.BannedUntil = b.now().UTC().Add(b.opts.Suspension)
	default:
		ban.Sanction = SanctionPermanent
	}

	if _, err := b.impose(ctx, ban); err != nil {
		return "", err
	}

	if err := b.store.ResetOffences(ctx, IdentifierDevice, device); err != nil {
		return "", fmt.Errorf("reset offences: %w", err)
	}
	return ban.Sanction, nil
}

// impose records one sanction and makes it take effect: a ban reaches every
// server the next time it checks the identifier, and a warning waits to be
// shown.
func (b *Bans) impose(ctx context.Context, ban Ban) (Ban, error) {
	ban, err := b.repo.AddBan(ctx, ban)
	if err != nil {
		return Ban{}, fmt.Errorf("add ban: %w", err)
	}

	if ban.Sanction == SanctionWarning {
		if err := b.store.MarkWarning(ctx, ban.IdentifierType, ban.Identifier, b.opts.WarningTTL); err != nil {
			return Ban{}, fmt.Errorf("mark warning: %w", err)
		}
		return ban, nil
	}

	if err := b.store.ForgetBan(ctx, ban.IdentifierType, ban.Identifier); err != nil {
		return Ban{}, fmt.Errorf("clear cached ban: %w", err)
	}
	return ban, nil
}

// TakeWarning reports whether a warning waits to be shown to id, and drops
// it, so that each warning is shown once.
func (b *Bans) TakeWarning(ctx context.Context, id Identity) (bool, error) {
	warned := false
	for _, identifierType := range identifierTypes {
		identifier := id.identifier(identifierType)
		if identifier == "" {
			continue
		}

		took, err := b.store.TakeWarning(ctx, identifierType, identifier)
		if err != nil {
			return false, fmt.Errorf("take warning: %w", err)
		}
		warned = warned || took
	}
	return warned, nil
}

// Imposition is a sanction an operator imposes on one identifier.
type Imposition struct {
	IdentifierType string
	Sanction       string
	// Duration is how long a suspension lasts. The other sanctions take none.
	Duration       time.Duration
	Reason         string
	ConversationID string
}

// Impose records a sanction an operator decided on identifier.
func (b *Bans) Impose(ctx context.Context, identifier string, imp Imposition) (Ban, error) {
	if err := validateImposition(imp); err != nil {
		return Ban{}, err
	}

	ban := Ban{
		IdentifierType: imp.IdentifierType,
		Identifier:     identifier,
		Sanction:       imp.Sanction,
		Source:         SourceOperator,
		Reason:         imp.Reason,
		ConversationID: imp.ConversationID,
	}
	if imp.Sanction == SanctionSuspension {
		ban.BannedUntil = b.now().UTC().Add(imp.Duration)
	}

	return b.impose(ctx, ban)
}

// validateImposition refuses what banned_identifiers cannot hold or an
// operator cannot have meant.
func validateImposition(imp Imposition) error {
	if !slices.Contains(identifierTypes, imp.IdentifierType) {
		return ErrUnknownIdentifierType
	}
	if !slices.Contains(sanctions, imp.Sanction) {
		return ErrUnknownSanction
	}
	if imp.Sanction == SanctionSuspension {
		if imp.Duration <= 0 || imp.Duration > MaxSuspension {
			return ErrInvalidSuspension
		}
	} else if imp.Duration != 0 {
		return ErrInvalidSuspension
	}
	if utf8.RuneCountInString(imp.Reason) > maxBanReasonRunes {
		return ErrReasonTooLong
	}
	return nil
}

// Lift ends the sanction id names. A lifted sanction is kept for the record,
// bars nothing and does not count towards the next stage.
func (b *Bans) Lift(ctx context.Context, id int64) (Ban, error) {
	ban, err := b.repo.GetBan(ctx, id)
	if err != nil {
		return Ban{}, err
	}
	if !ban.LiftedAt.IsZero() {
		return Ban{}, ErrAlreadyLifted
	}

	ban, err = b.repo.LiftBan(ctx, id, b.now().UTC())
	if err != nil {
		return Ban{}, err
	}

	if err := b.store.ForgetBan(ctx, ban.IdentifierType, ban.Identifier); err != nil {
		return Ban{}, fmt.Errorf("clear cached ban: %w", err)
	}
	return ban, nil
}

// List returns the bans f keeps.
func (b *Bans) List(ctx context.Context, f BanFilter) ([]Ban, error) {
	if f.Limit <= 0 {
		f.Limit = DefaultListLimit
	}
	f.Limit = min(f.Limit, MaxListLimit)
	return b.repo.ListBans(ctx, f, b.now().UTC())
}
