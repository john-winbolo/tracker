// SPDX-License-Identifier: GPL-3.0-or-later

package registry

import (
	"testing"
	"time"

	"github.com/john-winbolo/tracker/go-tracker/internal/proto"
)

func mkGame(ip [4]byte, port uint16, st uint32) Game {
	return Game{
		Key:        Key{IP: ip, Port: port, StartTime: st},
		MapName:    "m",
		Version:    "1.18",
		SourceIP:   ip,
		SourcePort: port,
	}
}

func TestUpdateInsertAndRefresh(t *testing.T) {
	r := New()
	now := time.Unix(1700000000, 0)
	g := mkGame([4]byte{10, 0, 0, 1}, 5000, 100)
	g.NumPlayers = 3
	r.Update(g, now)

	if r.Count() != 1 {
		t.Fatalf("count = %d, want 1", r.Count())
	}

	// Re-register the same game (same key) with changed settings. The lobby
	// can change any field mid-session, so all of them must update in place.
	g.NumPlayers = 5
	g.MapName = "newmap"
	g.GameType = proto.GameTournament
	r.Update(g, now.Add(time.Minute))

	if r.Count() != 1 {
		t.Fatalf("count = %d, want 1 (no insert on update)", r.Count())
	}
	got := r.Snapshot()[0]
	if got.NumPlayers != 5 {
		t.Errorf("NumPlayers = %d, want 5", got.NumPlayers)
	}
	if got.MapName != "newmap" {
		t.Errorf("MapName = %q, want updated 'newmap'", got.MapName)
	}
	if got.GameType != proto.GameTournament {
		t.Errorf("GameType = %d, want updated GameTournament", got.GameType)
	}
	if !got.LastPacket.Equal(now.Add(time.Minute)) {
		t.Errorf("LastPacket not refreshed")
	}
}

func TestPurgeExpiresOldGames(t *testing.T) {
	r := New()
	t0 := time.Unix(1700000000, 0)
	r.Update(mkGame([4]byte{1, 1, 1, 1}, 1, 1), t0)
	r.Update(mkGame([4]byte{2, 2, 2, 2}, 2, 2), t0.Add(time.Hour))

	// At t0 + 1h + 1s, g1 is past ExpireAfter (~4m10s) but g2 is fresh.
	r.Purge(t0.Add(time.Hour + time.Second))
	if r.Count() != 1 {
		t.Errorf("count after purge = %d, want 1", r.Count())
	}
	if g, ok := r.LookupByRegistered([4]byte{2, 2, 2, 2}, 2); !ok || g.Port != 2 {
		t.Errorf("expected the newer game to survive purge")
	}
}

func TestSnapshotOrderOldestFirst(t *testing.T) {
	r := New()
	t0 := time.Unix(1700000000, 0)
	r.Update(mkGame([4]byte{1, 0, 0, 0}, 1, 1), t0)
	r.Update(mkGame([4]byte{2, 0, 0, 0}, 2, 2), t0.Add(time.Second))
	r.Update(mkGame([4]byte{3, 0, 0, 0}, 3, 3), t0.Add(2*time.Second))

	got := r.Snapshot()
	if len(got) != 3 {
		t.Fatalf("len = %d, want 3", len(got))
	}
	if got[0].IP != [4]byte{1, 0, 0, 0} || got[2].IP != [4]byte{3, 0, 0, 0} {
		t.Errorf("snapshot order wrong: got IPs %v, %v, %v",
			got[0].IP, got[1].IP, got[2].IP)
	}
}

func TestRefreshSourceMatchesByIPOnly(t *testing.T) {
	r := New()
	t0 := time.Unix(1700000000, 0)
	g1 := mkGame([4]byte{10, 0, 0, 5}, 5000, 100)
	g2 := mkGame([4]byte{10, 0, 0, 5}, 5001, 200)
	g2.SourceIP = [4]byte{10, 0, 0, 5} // same NAT IP, two games
	r.Update(g1, t0)
	r.Update(g2, t0)

	r.RefreshSource([4]byte{10, 0, 0, 5}, 9999, t0.Add(time.Second))
	for _, g := range r.Snapshot() {
		if g.SourcePort != 9999 {
			t.Errorf("game %v: SourcePort = %d, want 9999", g.Key, g.SourcePort)
		}
	}
}

func TestRefreshSourceExactMatchesOne(t *testing.T) {
	r := New()
	t0 := time.Unix(1700000000, 0)
	g1 := mkGame([4]byte{10, 0, 0, 5}, 5000, 100)
	g2 := mkGame([4]byte{10, 0, 0, 5}, 5001, 200)
	r.Update(g1, t0)
	r.Update(g2, t0)

	r.RefreshSourceExact([4]byte{10, 0, 0, 5}, 8888, 200, t0.Add(time.Second))

	got1, _ := r.LookupByRegistered([4]byte{10, 0, 0, 5}, 5000)
	got2, _ := r.LookupByRegistered([4]byte{10, 0, 0, 5}, 5001)
	if got1.SourcePort != 5000 {
		t.Errorf("g1 SourcePort = %d, want unchanged 5000", got1.SourcePort)
	}
	if got2.SourcePort != 8888 {
		t.Errorf("g2 SourcePort = %d, want 8888", got2.SourcePort)
	}
}

func TestRefreshSourceExactSilentDropOnMiss(t *testing.T) {
	r := New()
	t0 := time.Unix(1700000000, 0)
	r.Update(mkGame([4]byte{1, 1, 1, 1}, 1, 1), t0)
	// no panic, no insert
	r.RefreshSourceExact([4]byte{9, 9, 9, 9}, 1234, 5678, t0)
	if r.Count() != 1 {
		t.Errorf("count = %d, want 1 (no insert on miss)", r.Count())
	}
}
