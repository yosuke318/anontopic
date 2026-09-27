package report

import (
	"context"
	"errors"
	"fmt"
	"math/rand/v2"
	"testing"
	"time"
)

// postgresTestBanRepository builds a ban repository on the PostgreSQL of the
// local stack, and skips like postgresTestRepository when the columns the
// sanctions need are not there.
func postgresTestBanRepository(t *testing.T) *PostgresBanRepository {
	t.Helper()

	pool := postgresTestRepository(t).pool

	var ready bool
	if err := pool.QueryRow(t.Context(),
		"SELECT EXISTS (SELECT 1 FROM information_schema.columns "+
			"WHERE table_name = 'banned_identifiers' AND column_name = 'lifted_at')").Scan(&ready); err != nil {
		t.Skipf("cannot read the schema: %v", err)
	}
	if !ready {
		t.Skip("banned_identifiers has no sanctions yet, run `make migrate` to test against it")
	}

	return &PostgresBanRepository{pool: pool}
}

// testIdentifier is an identifier no other test run writes, whose rows are
// removed afterwards.
func testIdentifier(t *testing.T, repo *PostgresBanRepository) string {
	t.Helper()

	identifier := fmt.Sprintf("test-%s-%d", t.Name(), rand.Int64())
	t.Cleanup(func() {
		if _, err := repo.pool.Exec(context.Background(),
			"DELETE FROM banned_identifiers WHERE identifier = $1", identifier); err != nil {
			t.Errorf("clean up: %v", err)
		}
	})
	return identifier
}

func TestPostgresBanRepositoryJudgesWhichSanctionsBar(t *testing.T) {
	repo := postgresTestBanRepository(t)
	ctx := t.Context()
	identifier := testIdentifier(t, repo)
	now := time.Now().UTC().Truncate(time.Microsecond)

	add := func(b Ban) Ban {
		t.Helper()
		b.IdentifierType = IdentifierDevice
		b.Identifier = identifier
		b.Source = SourceOperator
		added, err := repo.AddBan(ctx, b)
		if err != nil {
			t.Fatalf("AddBan: %v", err)
		}
		return added
	}

	add(Ban{Sanction: SanctionWarning})
	add(Ban{Sanction: SanctionSuspension, BannedUntil: now.Add(-time.Minute)})
	if banned, _, err := repo.BannedUntil(ctx, IdentifierDevice, identifier, now); err != nil || banned {
		t.Fatalf("BannedUntil = %v, %v, want a warning and a past suspension to bar nothing", banned, err)
	}

	suspension := add(Ban{Sanction: SanctionSuspension, BannedUntil: now.Add(time.Hour), ConversationID: "5f3a1c2e-7e9d-4b0a-9f3e-1c2e7e9d4b0a"})
	if suspension.ConversationID != "5f3a1c2e-7e9d-4b0a-9f3e-1c2e7e9d4b0a" || !suspension.BannedUntil.Equal(now.Add(time.Hour)) {
		t.Fatalf("suspension = %+v, want its conversation and its end", suspension)
	}
	banned, until, err := repo.BannedUntil(ctx, IdentifierDevice, identifier, now)
	if err != nil || !banned || !until.Equal(now.Add(time.Hour)) {
		t.Fatalf("BannedUntil = %v, %v, %v, want barred until %v", banned, until, err, now.Add(time.Hour))
	}

	permanent := add(Ban{Sanction: SanctionPermanent})
	if banned, until, err := repo.BannedUntil(ctx, IdentifierDevice, identifier, now); err != nil || !banned || !until.IsZero() {
		t.Fatalf("BannedUntil = %v, %v, %v, want barred with no end", banned, until, err)
	}

	if n, err := repo.CountSanctions(ctx, IdentifierDevice, identifier); err != nil || n != 4 {
		t.Fatalf("CountSanctions = %d, %v, want 4", n, err)
	}

	for _, id := range []int64{suspension.ID, permanent.ID} {
		if _, err := repo.LiftBan(ctx, id, now); err != nil {
			t.Fatalf("LiftBan: %v", err)
		}
	}
	if banned, _, err := repo.BannedUntil(ctx, IdentifierDevice, identifier, now); err != nil || banned {
		t.Fatalf("BannedUntil = %v, %v, want lifted bans to bar nothing", banned, err)
	}
	if n, err := repo.CountSanctions(ctx, IdentifierDevice, identifier); err != nil || n != 2 {
		t.Fatalf("CountSanctions = %d, %v, want the 2 that were not lifted", n, err)
	}

	got, err := repo.GetBan(ctx, permanent.ID)
	if err != nil || !got.LiftedAt.Equal(now) {
		t.Fatalf("GetBan = %+v, %v, want lifted at %v", got, err, now)
	}
}

func TestPostgresBanRepositoryListsTheBansInForce(t *testing.T) {
	repo := postgresTestBanRepository(t)
	ctx := t.Context()
	identifier := testIdentifier(t, repo)
	now := time.Now().UTC()

	var ids []int64
	for _, sanction := range []string{SanctionWarning, SanctionPermanent} {
		b, err := repo.AddBan(ctx, Ban{
			IdentifierType: IdentifierIPHash, Identifier: identifier, Sanction: sanction, Source: SourceOperator,
		})
		if err != nil {
			t.Fatalf("AddBan: %v", err)
		}
		ids = append(ids, b.ID)
	}

	// The rows of this test are the newest, so a page ending after the last
	// of them starts with them.
	all, err := repo.ListBans(ctx, BanFilter{BeforeID: ids[1] + 1, Limit: 2}, now)
	if err != nil || len(all) != 2 || all[0].ID != ids[1] || all[1].ID != ids[0] {
		t.Fatalf("ListBans = %+v, %v, want both, newest first", all, err)
	}

	active, err := repo.ListBans(ctx, BanFilter{Active: true, BeforeID: ids[1] + 1, Limit: 1}, now)
	if err != nil || len(active) != 1 || active[0].ID != ids[1] {
		t.Fatalf("active = %+v, %v, want the permanent ban", active, err)
	}
}

func TestPostgresBanRepositoryAnswersABanThatDoesNotExist(t *testing.T) {
	repo := postgresTestBanRepository(t)

	if _, err := repo.GetBan(t.Context(), -1); !errors.Is(err, ErrNotFound) {
		t.Fatalf("GetBan = %v, want %v", err, ErrNotFound)
	}
	if _, err := repo.LiftBan(t.Context(), -1, time.Now()); !errors.Is(err, ErrNotFound) {
		t.Fatalf("LiftBan = %v, want %v", err, ErrNotFound)
	}
}
