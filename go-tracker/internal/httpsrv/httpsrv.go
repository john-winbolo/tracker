// Package httpsrv serves the WinBolo tracker's HTTP-ish web view on port
// 50005. Output is byte-for-byte the same as the C tracker — winbolo.net
// scrapes this — including the trailing NUL after the header (which the C
// tracker emits via sizeof on a string literal) and the unconditional
// </PRE></BODY></HTML> footer regardless of game count.
package httpsrv

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

// Header is the literal first chunk the C tracker sends. The trailing NUL
// reproduces the sizeof-on-string-literal byte the C emits — winbolo.net's
// scraper has tolerated this since at least 2003.
const headerLiteral = "HTTP/1.1 200 OK\nContent-Type: text/html\n\n" +
	"<HTML><BODY><meta http-equiv=\"pragma\" content=\"no-cache\"></meta>" +
	"This is an html output of the game list<BR><BR>\r\n"

// Serve listens on addr (typically ":50005") and returns when ctx is
// cancelled.
func Serve(ctx context.Context, addr string, reg *registry.Registry, st *stats.Counters, peakHTTP func() int64, peakUDP func() int64, logger *slog.Logger) error {
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
		st.IncHTTP()
		go handleConn(conn, reg, st, peakHTTP, peakUDP, logger)
	}
}

func handleConn(conn net.Conn, reg *registry.Registry, st *stats.Counters, peakHTTP func() int64, peakUDP func() int64, logger *slog.Logger) {
	defer conn.Close()
	conn.SetDeadline(time.Now().Add(30 * time.Second))

	// Read whatever the client sends until we see the end of headers or the
	// timeout fires. We don't actually parse anything — the C tracker only
	// sniffs for "GET"/"get" and otherwise ignores the request. We still
	// drain so the response we send isn't interleaved with leftover request
	// bytes some clients buffer separately.
	drainRequest(conn)

	if err := WriteResponse(conn, reg, st, peakHTTP(), peakUDP()); err != nil {
		logger.Debug("http write", "err", err, "remote", conn.RemoteAddr().String())
	}
}

func drainRequest(conn net.Conn) {
	buf := make([]byte, 4096)
	conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	for {
		n, err := conn.Read(buf)
		if err != nil || n == 0 {
			return
		}
		// HTTP request ends with \r\n\r\n (or \n\n).
		if bytesEndOfHeaders(buf[:n]) {
			return
		}
	}
}

func bytesEndOfHeaders(b []byte) bool {
	s := string(b)
	return strings.Contains(s, "\r\n\r\n") || strings.Contains(s, "\n\n")
}

// WriteResponse writes the full HTTP response body to w. Exposed so tests
// can capture the bytes.
func WriteResponse(w io.Writer, reg *registry.Registry, st *stats.Counters, peakHTTP, peakUDP int64) error {
	if _, err := io.WriteString(w, headerLiteral); err != nil {
		return err
	}
	// The C tracker sends sizeof(literal) which includes a trailing NUL.
	if _, err := w.Write([]byte{0}); err != nil {
		return err
	}

	var sb strings.Builder
	sb.WriteString("Tracker Statistics<BR>\r\n")
	fmt.Fprintf(&sb, "Number of Tracked Games: %d<BR>\r\n", st.Games.Load())
	fmt.Fprintf(&sb, "Number of Tracker Game List Requests: %d<BR>\r\n", st.TCP.Load())
	fmt.Fprintf(&sb, "Number of Interesting Game List Requests: %d<BR><BR>\r\n", st.Interesting.Load())
	fmt.Fprintf(&sb, "Number of Tracker Web Requests: %d<BR><BR>\r\n", st.HTTP.Load())
	fmt.Fprintf(&sb, "Most http requests at one time: %d<BR>\r\n", peakHTTP)
	fmt.Fprintf(&sb, "Most udp requests at one time: %d<BR><BR>\r\n", peakUDP)

	games := reg.Snapshot()
	switch len(games) {
	case 0:
		sb.WriteString("There are no games at the moment - maybe you could start one.\r\n")
	case 1:
		sb.WriteString("There is currently 1 game.<BR><BR><PRE>\r\n\n")
	default:
		fmt.Fprintf(&sb, "There are currently %d games.<BR><BR><PRE>\r\n\n", len(games))
	}

	for _, g := range games {
		fmt.Fprintf(&sb,
			"Host: <B><A HREF=winbolo://%s:%d>%s:%d</A></B>  Version: <B>%s</B>  Players: <B>%d</B>  "+
				"Bases: <B>%d</B>  Pills: <B>%d</B>\nMap: <B>%s</B>  Game: <B>%s</B>  "+
				"Hidden Mines: <I>%s</I>  Bots: <I>%s</I>  PW: <I>%s</I>\n\n",
			g.Address, g.Port, g.Address, g.Port, g.Version,
			g.NumPlayers, g.NumBases, g.NumPills,
			g.MapName, gameTypeString(g.GameType),
			yesNo(g.HiddenMines), aiString(g.AI), yesNo(g.Password))
	}

	// Footer is unconditional in the C tracker — emitted even when no games
	// (so no <PRE> was opened). Match exactly.
	sb.WriteString("</PRE></BODY></HTML>\r\n")

	_, err := io.WriteString(w, sb.String())
	return err
}

// Format helpers — duplicated from package tcp deliberately (the formatting
// is part of the wire-stable contract for each transport, so coupling them
// would mean a change to one risks the other).

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
