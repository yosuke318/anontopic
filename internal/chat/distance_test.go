package chat

import (
	"testing"
)

func distanceOf(n int) *int { return &n }

func TestAWaitingDistanceReachesTheRoomAndALaterArrival(t *testing.T) {
	repo := newFakeRepository(tokenAlice, tokenBob)
	store := newFakeStore()
	srv := newTestServer(t, repo, store, nil, nil, testOptions(), tokenAlice, tokenBob)

	alice := dial(t, srv, repo.conv.ID, tokenAlice)
	if ev, _ := await(t, alice, eventJoined); len(ev.Distances) != 0 {
		t.Fatalf("distances = %+v, want none before anyone reported one", ev.Distances)
	}

	send(t, alice, clientFrame{Type: frameWaitingDistance, Distance: distanceOf(1234)})

	ev, _ := await(t, alice, eventWaitingDistance)
	if ev.Participant != 1 || ev.Distance == nil || *ev.Distance != 1234 {
		t.Fatalf("event = %+v, want participant 1 at 1234", ev)
	}

	// The participant who arrives after the report reads it on entering.
	bob := dial(t, srv, repo.conv.ID, tokenBob)
	joined, _ := await(t, bob, eventJoined)
	if len(joined.Distances) != 1 || joined.Distances[0] != (participantDistance{Participant: 1, Distance: 1234}) {
		t.Fatalf("distances = %+v, want participant 1 at 1234", joined.Distances)
	}
}

func TestAZeroWaitingDistanceIsSentAsAValue(t *testing.T) {
	repo := newFakeRepository(tokenAlice, tokenBob)
	srv := newTestServer(t, repo, newFakeStore(), nil, nil, testOptions(), tokenAlice)

	alice := dial(t, srv, repo.conv.ID, tokenAlice)
	await(t, alice, eventJoined)

	send(t, alice, clientFrame{Type: frameWaitingDistance, Distance: distanceOf(0)})

	if ev, _ := await(t, alice, eventWaitingDistance); ev.Distance == nil || *ev.Distance != 0 {
		t.Fatalf("event = %+v, want a distance of 0", ev)
	}
}

func TestOnlyTheFirstWaitingDistanceOfAParticipantIsKept(t *testing.T) {
	repo := newFakeRepository(tokenAlice, tokenBob)
	store := newFakeStore()
	srv := newTestServer(t, repo, store, nil, nil, testOptions(), tokenAlice, tokenBob)

	alice := dial(t, srv, repo.conv.ID, tokenAlice)
	bob := dial(t, srv, repo.conv.ID, tokenBob)
	await(t, alice, eventJoined)
	await(t, bob, eventJoined)
	send(t, alice, clientFrame{Type: frameWaitingDistance, Distance: distanceOf(100)})
	await(t, bob, eventWaitingDistance)

	// Connecting again is a new connection, and the report it sends finds
	// one already kept for the participant. Bob stays, so the conversation
	// goes on while alice is away.
	_ = alice.Close()
	again := dial(t, srv, repo.conv.ID, tokenAlice)
	await(t, again, eventJoined)
	send(t, again, clientFrame{Type: frameWaitingDistance, Distance: distanceOf(999_999)})

	// The frames of one connection are read in order, so the message
	// reaching the room means the report before it was read.
	send(t, again, clientFrame{Type: frameMessage, Body: "ただいま"})
	await(t, bob, eventMessage)

	kept, err := store.Distances(t.Context(), repo.conv.ID)
	if err != nil {
		t.Fatalf("Distances: %v", err)
	}
	if kept[1] != 100 {
		t.Fatalf("kept = %v, want the first report of participant 1", kept)
	}
}

func TestAWaitingDistanceTheRoomCannotTakeIsDroppedWithoutAnAnswer(t *testing.T) {
	tests := []struct {
		name  string
		frame clientFrame
	}{
		{"no distance", clientFrame{Type: frameWaitingDistance}},
		{"a negative distance", clientFrame{Type: frameWaitingDistance, Distance: distanceOf(-1)}},
		{"a distance over the limit", clientFrame{Type: frameWaitingDistance, Distance: distanceOf(maxWaitingDistance + 1)}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			repo := newFakeRepository(tokenAlice, tokenBob)
			store := newFakeStore()
			srv := newTestServer(t, repo, store, nil, nil, testOptions(), tokenAlice)

			alice := dial(t, srv, repo.conv.ID, tokenAlice)
			await(t, alice, eventJoined)

			send(t, alice, tc.frame)
			send(t, alice, clientFrame{Type: frameMessage, Body: "こんにちは"})

			// The frames are read in order, so the message arriving with
			// nothing before it means the report was neither answered nor
			// told to the room.
			for {
				ev, payload := readEvent(t, alice)
				if ev.Type == eventMessage {
					break
				}
				if ev.Type == eventError || ev.Type == eventWaitingDistance {
					t.Fatalf("the report was answered with %s", payload)
				}
			}

			if kept, _ := store.Distances(t.Context(), repo.conv.ID); len(kept) != 0 {
				t.Fatalf("kept = %v, want nothing", kept)
			}
		})
	}
}
