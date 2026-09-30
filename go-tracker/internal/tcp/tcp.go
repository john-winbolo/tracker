// SPDX-License-Identifier: GPL-3.0-or-later

// Package tcp serves the WinBolo tracker game-list protocol on port 50000
// (all games) and is shared with the interesting-games listener on 50001.
//
// Output format is byte-exact with the C tracker — winbolo.net and any
// other client out there parses this. See FormatGameList for the spec.
package tcp

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"strings"
	"time"

	"github.com/john-winbolo/tracker/go-tracker/internal/proto"
	"github.com/john-winbolo/tracker/go-tracker/internal/registry"
	"github.com/john-winbolo/tracker/go-tracker/internal/stats"
)

// Filter selects which subset of games to send.
type Filter int

const (
	FilterAll         Filter = 0 // tcpSendGameData TCP_ALL
	FilterInteresting Filter = 1 // tcpSendGameData TCP_INTERESTING
)

// Serve listens on addr and dispatches each accepted connection to a
// goroutine that calls FormatGameList. tag is the request type to bump in
// stats — use stats.IncTCP for port 50000, IncInteresting for 50001.
func Serve(ctx context.Context, addr string, reg *registry.Registry, st *stats.Counters, filter Filter, logger *slog.Logger) error {
	lc := net.ListenConfig{}
	ln, err := lc.Listen(ctx, "tcp4", addr)
	if err != nil {
		return err
	}
	go func() {
		<-ctx.Done()
		ln.Close()
	}()
	if logger == nil {
		logger = slog.Default()
	}
	for {
		conn, err := ln.Accept()
		if err != nil {
			if ctx.Err() != nil {
				return nil
			}
			var ne net.Error
			if errors.As(err, &ne) && !ne.Timeout() {
				return err
			}
			continue
		}
		switch filter {
		case FilterInteresting:
			st.IncInteresting()
		default:
			st.IncTCP()
		}
		go handleConn(conn, reg, st, filter, logger)
	}
}

func handleConn(conn net.Conn, reg *registry.Registry, st *stats.Counters, filter Filter, logger *slog.Logger) {
	defer conn.Close()
	conn.SetWriteDeadline(time.Now().Add(30 * time.Second))
	if err := WriteGameList(conn, reg, st, filter); err != nil {
		logger.Debug("tcp write", "err", err, "remote", conn.RemoteAddr().String())
	}
}

// WriteGameList serializes the game list to w in the C tracker's exact
// format. Exposed so tests can capture output to a buffer.
func WriteGameList(w io.Writer, reg *registry.Registry, st *stats.Counters, filter Filter) error {
	var sb strings.Builder
	sb.WriteString("TVERSION=1\r\n")

	if filter == FilterInteresting {
		sb.WriteString("MOTDL=10\r\n")
	} else {
		sb.WriteString("MOTDL=6\r\n")
	}
	sb.WriteString("MOTD=This is a simple tracker\r\n")
	if filter == FilterInteresting {
		sb.WriteString("MOTD=\r\n")
		sb.WriteString("MOTD=This is the INTERESTING games list, where interesting means\r\n")
		sb.WriteString("MOTD=non-PW, < 6 players, and unclaimed bases (or open game).\r\n")
		sb.WriteString("MOTD=\r\n")
	}
	fmt.Fprintf(&sb, "MOTD=Tracker Statistics\r\n")
	fmt.Fprintf(&sb, "MOTD=Number of Tracked Games: %d\r\n", st.Games.Load())
	fmt.Fprintf(&sb, "MOTD=Number of Tracker Game List Requests: %d\r\n", st.TCP.Load())
	fmt.Fprintf(&sb, "MOTD=Number of Tracker Interesting Game List Requests: %d\r\n", st.Interesting.Load())
	fmt.Fprintf(&sb, "MOTD=Number of Tracker Web Requests: %d\r\n", st.HTTP.Load())

	games := reg.Snapshot()
	// Count how many pass the filter, since NGAMES is sent before the GAME
	// blocks. Mirrors the C tracker's two-pass loop.
	visible := games
	if filter == FilterInteresting {
		filtered := games[:0:0]
		for _, g := range games {
			if interesting(g) {
				filtered = append(filtered, g)
			}
		}
		visible = filtered
	}
	fmt.Fprintf(&sb, "NGAMES=%d\r\n", len(visible))

	if _, err := io.WriteString(w, sb.String()); err != nil {
		return err
	}

	// Emit each game block. Index restarts at 000 and increments per emitted
	// game (not per skipped game) — the C tracker's sendCount semantics.
	idx := 0
	for _, g := range visible {
		var block strings.Builder
		fmt.Fprintf(&block, "GAME%03d=%s:%d\r\n", idx, g.Address, g.Port)
		fmt.Fprintf(&block, "VERSION=%s\r\n", g.Version)
		fmt.Fprintf(&block, "MAP=%s\r\n", g.MapName)
		fmt.Fprintf(&block, "TYPE=%s\r\n", gameTypeString(g.GameType))
		fmt.Fprintf(&block, "PLAYERS=%d\r\n", g.NumPlayers)
		fmt.Fprintf(&block, "BASES=%d\r\n", g.NumBases)
		fmt.Fprintf(&block, "PILLS=%d\r\n", g.NumPills)
		fmt.Fprintf(&block, "HIDMINES=%s\r\n", yesNo(g.HiddenMines))
		fmt.Fprintf(&block, "PASSWORD=%s\r\n", yesNo(g.Password))
		fmt.Fprintf(&block, "BRAINS=%s\r\n", aiString(g.AI))
		fmt.Fprintf(&block, "DELAY=%d\r\n", g.StartDelay)
		fmt.Fprintf(&block, "LIMIT=%d\r\n", g.TimeLimit)
		// STARTTIME prints as signed long matching the C %ld; for the
		// canonical BE-derived uint32 this gives the same digits unless
		// the high bit is set, in which case the C tracker also prints a
		// negative number — int32 cast preserves that.
		fmt.Fprintf(&block, "STARTTIME=%d\r\n", int32(g.StartTime))
		if _, err := io.WriteString(w, block.String()); err != nil {
			return err
		}
		idx++
	}
	return nil
}

// interesting mirrors the C filter:
//
//	password == FALSE && ((numPlayers < 6 && numBases > 0) || game == gameOpen)
func interesting(g registry.Game) bool {
	if g.Password {
		return false
	}
	if g.NumPlayers < 6 && g.NumBases > 0 {
		return true
	}
	if g.GameType == proto.GameOpen {
		return true
	}
	return false
}

func gameTypeString(t proto.GameType) string {
	switch t {
	case proto.GameOpen:
		return "Open"
	case proto.GameTournament:
		return "Tournament"
	default:
		return "Strict"
	}
}

func aiString(a proto.AIType) string {
	switch a {
	case proto.AINone:
		return "no"
	case proto.AIYes:
		return "yes"
	case proto.AIYesAdvantage:
		return "yesAdv"
	default:
		return "yesFull"
	}
}

func yesNo(b bool) string {
	if b {
		return "Yes"
	}
	return "No"
}
