// SPDX-License-Identifier: GPL-3.0-or-later

// Package registry holds the live set of WinBolo games seen by the tracker.
//
// Replaces the C tracker's currentGames singly-linked list with a map keyed
// by (registered IP, port, starttime). Iteration order is preserved as
// insertion order (oldest first), matching the rendering order callers in
// the C tracker get from currentGamesGetItem(numGamesTotal-count+1).
package registry

import (
	"sort"
	"sync"
	"time"

	"github.com/john-winbolo/tracker/go-tracker/internal/proto"
)

// Game expiry — the C tracker had separate OLD/NEW timeouts but the values
// were the same (4*60+10 seconds), so we just use one constant.
const ExpireAfter = 4*time.Minute + 10*time.Second

// Key uniquely identifies a registered game.
//
// IP is the registered/advertised IP (from serveraddress for modern clients,
// from the UDP packet source otherwise). Port is the canonicalized port from
// the INFO_PACKET. StartTime is the BE-interpreted token from the wire (also
// what 8-byte WBKA keepalives carry).
type Key struct {
	IP        [4]byte
	Port      uint16
	StartTime uint32
}

// Game is the registry's stored form of a registered WinBolo game.
type Game struct {
	Key
	Address     string // hostname (or dotted-quad fallback) for display
	MapName     string
	Version     string // formatted "%d.%d%d" matching C tracker
	NumPlayers  uint8
	NumBases    uint8
	NumPills    uint8
	HiddenMines bool
	GameType    proto.GameType
	AI          proto.AIType
	Password    bool
	StartDelay  int32
	TimeLimit   int32

	// SourceIP/SourcePort track the live NAT mapping observed for this host.
	// Refreshed by INFO_PACKET registration and by WBKA keepalives. May
	// differ from Key.IP when the host advertises a UPnP-mapped external
	// address. Used to push PunchNotify through the host's live mapping.
	SourceIP   [4]byte
	SourcePort uint16

	LastPacket time.Time
	addedAt    time.Time // for stable insertion-order iteration
}

// Registry is the concurrent game store.
type Registry struct {
	mu    sync.RWMutex
	games map[Key]*Game
	// monotonically increasing sequence to break ties in addedAt for tests
	// that insert multiple games at the same instant.
	seq uint64
}

// New creates an empty Registry.
func New() *Registry {
	return &Registry{games: make(map[Key]*Game)}
}

// Update inserts a new game or refreshes an existing one. Every game setting
// is refreshed on re-registration — the lobby can change map, game type,
// mines, AI, and any other field mid-session without bumping StartTime, so we
// overwrite the whole record. Only addedAt is preserved, to keep the stable
// insertion-order used by Snapshot's rendering.
//
// (This diverges from the legacy C tracker, which updated only a fixed subset
// — counts, timelimit, source mapping — and so could never reflect a map or
// settings change without a new StartTime. The C tracker is frozen; the Go
// port is the source of truth.)
func (r *Registry) Update(g Game, now time.Time) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if existing, ok := r.games[g.Key]; ok {
		g.addedAt = existing.addedAt
		g.LastPacket = now
		*existing = g
		return
	}
	g.LastPacket = now
	g.addedAt = now
	r.seq++
	// Use seq to disambiguate ties; addedAt sub-microsecond ties are
	// otherwise possible on systems with coarse time resolution.
	g.addedAt = g.addedAt.Add(time.Duration(r.seq))
	gp := g
	r.games[g.Key] = &gp
}

// RefreshSource updates LastPacket and SourcePort on all games whose stored
// SourceIP matches sourceIP. Mirrors currentGamesRefreshSource — used when a
// 4-byte WBKA keepalive arrives without a game token.
func (r *Registry) RefreshSource(sourceIP [4]byte, sourcePort uint16, now time.Time) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, g := range r.games {
		if g.SourceIP == sourceIP {
			g.SourcePort = sourcePort
			g.LastPacket = now
		}
	}
}

// RefreshSourceExact updates LastPacket and SourcePort on the single game
// matching both sourceIP and startTime. Mirrors currentGamesRefreshSourceExact —
// used when an 8-byte WBKA keepalive carrying a game token arrives. Drops
// silently if no entry matches (e.g. stale token from a re-registered host).
func (r *Registry) RefreshSourceExact(sourceIP [4]byte, sourcePort uint16, startTime uint32, now time.Time) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, g := range r.games {
		if g.SourceIP == sourceIP && g.StartTime == startTime {
			g.SourcePort = sourcePort
			g.LastPacket = now
			return
		}
	}
}

// LookupByRegistered finds a game by its registered (advertised) IP and
// port. Used by hole-punch coordination to find the host a joiner asked for.
// The returned Game is a copy — callers can read it without holding the lock.
// ok is false if no such game is registered.
func (r *Registry) LookupByRegistered(ip [4]byte, port uint16) (Game, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	for _, g := range r.games {
		if g.IP == ip && g.Port == port {
			return *g, true
		}
	}
	return Game{}, false
}

// Purge removes games that haven't been seen in ExpireAfter.
func (r *Registry) Purge(now time.Time) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for k, g := range r.games {
		if now.Sub(g.LastPacket) > ExpireAfter {
			delete(r.games, k)
		}
	}
}

// Count returns the current number of registered games.
func (r *Registry) Count() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.games)
}

// Snapshot returns all games in insertion order (oldest first), matching
// the C tracker's TCP/HTTP rendering order. Callers can iterate the result
// without holding any registry lock.
func (r *Registry) Snapshot() []Game {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]Game, 0, len(r.games))
	for _, g := range r.games {
		out = append(out, *g)
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].addedAt.Before(out[j].addedAt)
	})
	return out
}
