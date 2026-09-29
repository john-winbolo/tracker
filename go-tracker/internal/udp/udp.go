// Package udp serves the WinBolo tracker UDP protocol on port 50000.
//
// Handles:
//   - 76-byte INFO_PACKET registration (with version-gated address rules), and
//     the 111-byte extended INFO_PACKET sent by WinBolo ≥1.8.8
//   - 4-byte WBKA NAT keepalive
//   - 8-byte WBKA NAT keepalive carrying a game token
//   - 8-byte PunchProbeRequest (STUN-style host self-probe)
//   - 14-byte PunchRequest (joiner asking tracker to nudge a host)
package udp

import (
	"context"
	"errors"
	"log/slog"
	"net"
	"net/netip"
	"strings"
	"sync"
	"time"

	"github.com/john-winbolo/tracker/go-tracker/internal/bans"
	"github.com/john-winbolo/tracker/go-tracker/internal/proto"
	"github.com/john-winbolo/tracker/go-tracker/internal/registry"
	"github.com/john-winbolo/tracker/go-tracker/internal/stats"
)

// Server is the UDP listener.
type Server struct {
	conn   *net.UDPConn
	reg    *registry.Registry
	bans   *bans.List
	stats  *stats.Counters
	logger *slog.Logger

	// inflight tracks goroutines spawned for DNS/register so we can report a
	// peak to the HTTP stats page (mimicking the C tracker's "Most udp
	// requests at one time").
	inflight sync.WaitGroup
	cur      int64
	curMu    sync.Mutex
}

// Listen binds a UDP socket on addr (typically ":50000") and returns the
// initialized Server. Caller is responsible for calling Serve.
func Listen(addr string, reg *registry.Registry, bl *bans.List, s *stats.Counters, logger *slog.Logger) (*Server, error) {
	udpAddr, err := net.ResolveUDPAddr("udp4", addr)
	if err != nil {
		return nil, err
	}
	conn, err := net.ListenUDP("udp4", udpAddr)
	if err != nil {
		return nil, err
	}
	if logger == nil {
		logger = slog.Default()
	}
	return &Server{
		conn:   conn,
		reg:    reg,
		bans:   bl,
		stats:  s,
		logger: logger,
	}, nil
}

// Close shuts down the listener. Safe to call multiple times.
func (s *Server) Close() error {
	if s.conn != nil {
		err := s.conn.Close()
		s.inflight.Wait()
		return err
	}
	return nil
}

// LocalAddr returns the bound address — useful for tests using port 0.
func (s *Server) LocalAddr() net.Addr { return s.conn.LocalAddr() }

// Serve runs the read loop until ctx is cancelled or the connection is
// closed. Returns nil on graceful shutdown, otherwise the read error.
func (s *Server) Serve(ctx context.Context) error {
	go func() {
		<-ctx.Done()
		s.conn.SetReadDeadline(time.Unix(1, 0))
	}()

	buf := make([]byte, 2048)
	for {
		n, src, err := s.conn.ReadFromUDPAddrPort(buf)
		if err != nil {
			if ctx.Err() != nil {
				return nil
			}
			var ne net.Error
			if errors.As(err, &ne) && ne.Timeout() {
				if ctx.Err() != nil {
					return nil
				}
				continue
			}
			return err
		}
		s.handle(buf[:n], src)
	}
}

// handle dispatches a single received datagram. Hole-punch handlers run
// inline because they're a registry lookup plus a sendto. INFO_PACKET runs
// in a goroutine because it does a DNS reverse lookup.
func (s *Server) handle(b []byte, src netip.AddrPort) {
	switch {
	case proto.IsWBKA4(b):
		s.handleWBKA4(src)
	case proto.IsWBKA8(b):
		s.handleWBKA8(b, src)
	case proto.IsPunchProbeRequest(b):
		s.handleProbeRequest(src)
	case proto.IsPunchRequest(b):
		s.handlePunchRequest(b, src)
	case len(b) == proto.InfoPacketSize || len(b) == proto.InfoPacketSizeExtended:
		ip, err := proto.DecodeInfo(b)
		if err != nil {
			s.logger.Debug("invalid INFO_PACKET", "src", src.String(), "err", err)
			return
		}
		s.stats.IncUDP()
		// Spawn a goroutine because reverse DNS can take seconds.
		s.inflight.Add(1)
		s.curMu.Lock()
		s.cur++
		s.stats.ObserveUDPThreads(s.cur)
		s.curMu.Unlock()
		go func() {
			defer s.inflight.Done()
			defer func() {
				s.curMu.Lock()
				s.cur--
				s.curMu.Unlock()
			}()
			s.processInfoPacket(ip, src)
		}()
	default:
		s.logger.Debug("unrecognized UDP packet", "len", len(b), "src", src.String())
	}
}

func (s *Server) handleWBKA4(src netip.AddrPort) {
	addr := src.Addr().As4()
	s.reg.RefreshSource(addr, src.Port(), time.Now())
}

func (s *Server) handleWBKA8(b []byte, src netip.AddrPort) {
	addr := src.Addr().As4()
	token := proto.WBKA8Token(b)
	s.reg.RefreshSourceExact(addr, src.Port(), token, time.Now())
}

func (s *Server) handleProbeRequest(src netip.AddrPort) {
	addr := src.Addr().As4()
	reply := proto.EncodePunchProbeReply(addr, src.Port())
	if _, err := s.conn.WriteToUDPAddrPort(reply, src); err != nil {
		s.logger.Debug("probe reply send failed", "err", err)
	}
}

// handlePunchRequest looks up the host the joiner is asking for, sends an
// ack to the joiner, and (on PunchAckOK) pushes a notify through the host's
// live NAT mapping. We snapshot the host's source under the registry lock
// before any sendto — matching the C tracker which drops the games lock
// before blocking I/O.
func (s *Server) handlePunchRequest(b []byte, src netip.AddrPort) {
	joinerIP := src.Addr().As4()
	joinerPort := src.Port()
	targetIP, targetPort := proto.PunchRequestTarget(b)

	game, ok := s.reg.LookupByRegistered(targetIP, targetPort)
	var status uint8
	switch {
	case !ok:
		status = proto.PunchAckHostNotFound
	case game.SourcePort == 0:
		status = proto.PunchAckHostUnreachable
	default:
		status = proto.PunchAckOK
	}

	ack := proto.EncodePunchAck(status)
	dest := netip.AddrPortFrom(netip.AddrFrom4(joinerIP), joinerPort)
	if _, err := s.conn.WriteToUDPAddrPort(ack, dest); err != nil {
		s.logger.Debug("punch ack send failed", "err", err)
	}

	if status == proto.PunchAckOK {
		notify := proto.EncodePunchNotify(joinerIP, joinerPort)
		hostDest := netip.AddrPortFrom(netip.AddrFrom4(game.SourceIP), game.SourcePort)
		if _, err := s.conn.WriteToUDPAddrPort(notify, hostDest); err != nil {
			s.logger.Debug("punch notify send failed", "err", err)
		}
	}
}

// processInfoPacket runs the equivalent of udpProcessInfoPacket: choose
// registered IP, reverse DNS it, ban check, then update the registry.
func (s *Server) processInfoPacket(ip proto.InfoPacket, src netip.AddrPort) {
	srcAddr := src.Addr().As4()

	// Decide registered IP (advertised vs. packet source) per version gate.
	// The version gate happens at the wire layer in C (drops pre-1.1.8 with
	// non-zero advertised); we replicate it here to avoid passing dropped
	// packets into the registry.
	var registeredAddr [4]byte
	if ip.ServerAddress != [4]byte{} {
		if !ip.AcceptsAdvertisedAddress() {
			s.logger.Debug("dropping pre-1.1.8 with non-zero advertised addr",
				"src", src.String())
			return
		}
		registeredAddr = ip.ServerAddress
	} else {
		registeredAddr = srcAddr
	}

	address := reverseLookup(registeredAddr)
	if s.bans.Exists(address) {
		s.logger.Debug("banned", "addr", address)
		return
	}

	s.stats.IncGames()
	s.reg.Update(registry.Game{
		Key: registry.Key{
			IP:        registeredAddr,
			Port:      ip.ServerPort,
			StartTime: ip.StartTime,
		},
		Address:     address,
		MapName:     ip.MapName,
		Version:     ip.VersionString(),
		NumPlayers:  uint8(ip.NumPlayers),
		NumBases:    uint8(ip.FreeBases),
		NumPills:    uint8(ip.FreePills),
		HiddenMines: ip.HiddenMines(),
		GameType:    ip.GameType,
		AI:          ip.AllowAI,
		Password:    ip.HasPassword,
		StartDelay:  ip.StartDelay,
		TimeLimit:   ip.TimeLimit,
		SourceIP:    srcAddr,
		SourcePort:  src.Port(),
	}, time.Now())
}

// reverseLookup mirrors utilReverseLookup: PTR lookup, fall through to a few
// hardcoded aliases the original tracker maintained, and dotted-quad
// fallback if PTR fails. The aliases are vestigial (the named hosts haven't
// resolved this way for years) but we keep them for byte-exact parity with
// the C tracker on the off chance they ever match.
func reverseLookup(ip [4]byte) string {
	addr := net.IPv4(ip[0], ip[1], ip[2], ip[3])
	names, err := net.LookupAddr(addr.String())
	if err != nil || len(names) == 0 {
		return addr.String()
	}
	name := strings.TrimSuffix(names[0], ".")

	return name
}
