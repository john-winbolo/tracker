package udp

import (
	"context"
	"encoding/binary"
	"log/slog"
	"net"
	"net/netip"
	"testing"
	"time"

	"github.com/john-winbolo/tracker/go-tracker/internal/bans"
	"github.com/john-winbolo/tracker/go-tracker/internal/proto"
	"github.com/john-winbolo/tracker/go-tracker/internal/registry"
	"github.com/john-winbolo/tracker/go-tracker/internal/stats"
)

// startServer spins up a UDP server on a random port and returns it plus a
// connected client socket and the server's port (for the client to send to).
func startServer(t *testing.T) (*Server, *net.UDPConn, uint16) {
	t.Helper()
	reg := registry.New()
	bl := bans.New()
	st := stats.New()
	logger := slog.New(slog.NewTextHandler(testWriter{t}, nil))

	srv, err := Listen("127.0.0.1:0", reg, bl, st, logger)
	if err != nil {
		t.Fatal(err)
	}

	srvAddr := srv.LocalAddr().(*net.UDPAddr)

	// Start serving in a goroutine
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	t.Cleanup(func() { srv.Close() })
	go srv.Serve(ctx)

	// Open a client socket so we can both send to the server AND receive
	// replies on the same NAT mapping (necessary for probe/punch tests).
	cliConn, err := net.DialUDP("udp4", nil, srvAddr)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { cliConn.Close() })

	return srv, cliConn, uint16(srvAddr.Port)
}

func TestServerRegistersInfoPacket(t *testing.T) {
	srv, cli, _ := startServer(t)

	// Build an INFO_PACKET with serveraddress=0 (registry uses packet source)
	pkt := proto.EncodeInfo(proto.InfoPacket{
		VersionMajor: 1, VersionMinor: 1, VersionRevision: 8,
		MapName:    "test.map",
		ServerPort: 7777,
		StartTime:  0xCAFEBABE,
		GameType:   proto.GameOpen,
		AllowMines: 0x80,
		NumPlayers: 2,
		FreeBases:  3,
		FreePills:  4,
	})
	if _, err := cli.Write(pkt); err != nil {
		t.Fatal(err)
	}

	// Wait for goroutine + DNS to register the game.
	if !waitFor(t, time.Second, func() bool { return srv.reg.Count() == 1 }) {
		t.Fatalf("registry never reached count=1, got %d", srv.reg.Count())
	}
	games := srv.reg.Snapshot()
	g := games[0]
	if g.Port != 7777 {
		t.Errorf("port = %d, want 7777", g.Port)
	}
	if g.StartTime != 0xCAFEBABE {
		t.Errorf("starttime = %x, want CAFEBABE", g.StartTime)
	}
	if g.MapName != "test.map" {
		t.Errorf("mapname = %q", g.MapName)
	}
	if g.NumPlayers != 2 || g.NumBases != 3 || g.NumPills != 4 {
		t.Errorf("counts wrong: players=%d bases=%d pills=%d", g.NumPlayers, g.NumBases, g.NumPills)
	}
	if !g.HiddenMines {
		t.Errorf("HiddenMines should be true (0x80)")
	}
}

func TestServerWBKA4Refreshes(t *testing.T) {
	srv, cli, _ := startServer(t)

	// Register a game first
	pkt := proto.EncodeInfo(proto.InfoPacket{
		VersionMajor: 1, VersionMinor: 1, VersionRevision: 8,
		MapName: "m", ServerPort: 1234, StartTime: 1,
	})
	cli.Write(pkt)
	waitFor(t, time.Second, func() bool { return srv.reg.Count() == 1 })

	before := srv.reg.Snapshot()[0].LastPacket
	time.Sleep(10 * time.Millisecond)

	// 4-byte WBKA — server should refresh LastPacket on our game (since
	// the source IP matches what we registered with).
	cli.Write([]byte{'W', 'B', 'K', 'A'})

	if !waitFor(t, time.Second, func() bool {
		return srv.reg.Snapshot()[0].LastPacket.After(before)
	}) {
		t.Errorf("LastPacket not refreshed by WBKA4")
	}
}

func TestServerWBKA8RefreshesByToken(t *testing.T) {
	srv, cli, _ := startServer(t)

	// Register two games at the same NAT IP, different starttimes
	pkt1 := proto.EncodeInfo(proto.InfoPacket{
		VersionMajor: 1, VersionMinor: 1, VersionRevision: 8,
		MapName: "m1", ServerPort: 1, StartTime: 0x01020304,
	})
	pkt2 := proto.EncodeInfo(proto.InfoPacket{
		VersionMajor: 1, VersionMinor: 1, VersionRevision: 8,
		MapName: "m2", ServerPort: 2, StartTime: 0x05060708,
	})
	cli.Write(pkt1)
	cli.Write(pkt2)
	waitFor(t, time.Second, func() bool { return srv.reg.Count() == 2 })

	// Send an 8-byte WBKA carrying token 0x05060708 — only game 2 should
	// have its LastPacket bumped.
	wbka := []byte{'W', 'B', 'K', 'A', 0x05, 0x06, 0x07, 0x08}
	time.Sleep(10 * time.Millisecond)
	t1 := timeOfStartTime(srv, 0x01020304)
	t2 := timeOfStartTime(srv, 0x05060708)
	cli.Write(wbka)

	if !waitFor(t, time.Second, func() bool {
		return timeOfStartTime(srv, 0x05060708).After(t2)
	}) {
		t.Errorf("WBKA8 didn't refresh game with matching token")
	}
	if !timeOfStartTime(srv, 0x01020304).Equal(t1) {
		t.Errorf("WBKA8 refreshed wrong game (token didn't disambiguate)")
	}
}

func TestServerProbeReply(t *testing.T) {
	_, cli, _ := startServer(t)

	probe := []byte{'W', 'B', proto.PunchProbeRequest, 0, 0, 0, 0, 0}
	cli.Write(probe)

	cli.SetReadDeadline(time.Now().Add(time.Second))
	buf := make([]byte, 64)
	n, err := cli.Read(buf)
	if err != nil {
		t.Fatal(err)
	}
	if n != 14 {
		t.Fatalf("reply len = %d, want 14", n)
	}
	if buf[0] != 'W' || buf[1] != 'B' || buf[2] != proto.PunchProbeReply {
		t.Errorf("bad header: %x", buf[0:3])
	}

	// The reply's reflexive address should be our local UDP socket address.
	cliAddr := cli.LocalAddr().(*net.UDPAddr)
	wantPort := uint16(cliAddr.Port)
	gotPort := binary.BigEndian.Uint16(buf[12:14])
	if gotPort != wantPort {
		t.Errorf("reflexive port = %d, want %d", gotPort, wantPort)
	}
	// IP comes from recvfrom — should be 127.0.0.1 since both are on lo.
	if [4]byte{buf[8], buf[9], buf[10], buf[11]} != [4]byte{127, 0, 0, 1} {
		t.Errorf("reflexive IP = %v, want 127.0.0.1", buf[8:12])
	}
}

func TestServerPunchHostNotFound(t *testing.T) {
	_, cli, _ := startServer(t)

	// PunchRequest for an unregistered host
	pr := make([]byte, 14)
	pr[0] = 'W'
	pr[1] = 'B'
	pr[2] = proto.PunchRequest
	copy(pr[8:12], []byte{198, 51, 100, 1}) // TEST-NET
	binary.BigEndian.PutUint16(pr[12:14], 9999)

	cli.Write(pr)
	cli.SetReadDeadline(time.Now().Add(time.Second))
	buf := make([]byte, 64)
	n, err := cli.Read(buf)
	if err != nil {
		t.Fatal(err)
	}
	if n != 9 {
		t.Fatalf("ack len = %d, want 9", n)
	}
	if buf[2] != proto.PunchRequestAck {
		t.Errorf("opcode = %d, want %d", buf[2], proto.PunchRequestAck)
	}
	if buf[8] != proto.PunchAckHostNotFound {
		t.Errorf("status = %d, want PunchAckHostNotFound (%d)", buf[8], proto.PunchAckHostNotFound)
	}
}

func TestServerPunchHostFoundSendsNotify(t *testing.T) {
	srv, joiner, srvPort := startServer(t)

	// "Host" socket — needs to be able to receive the PunchNotify.
	host, err := net.DialUDP("udp4", nil, &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: int(srvPort)})
	if err != nil {
		t.Fatal(err)
	}
	defer host.Close()

	// Host registers a game. ServerAddress=0 → registry uses packet source.
	hostPkt := proto.EncodeInfo(proto.InfoPacket{
		VersionMajor: 1, VersionMinor: 1, VersionRevision: 8,
		MapName: "h", ServerPort: 4242, StartTime: 1,
	})
	host.Write(hostPkt)
	if !waitFor(t, time.Second, func() bool { return srv.reg.Count() == 1 }) {
		t.Fatal("host never registered")
	}
	hostGame := srv.reg.Snapshot()[0]
	registeredIP := hostGame.IP

	// Joiner sends PunchRequest for the host.
	pr := make([]byte, 14)
	pr[0] = 'W'
	pr[1] = 'B'
	pr[2] = proto.PunchRequest
	copy(pr[8:12], registeredIP[:])
	binary.BigEndian.PutUint16(pr[12:14], 4242)
	joiner.Write(pr)

	// Joiner reads the ack.
	joiner.SetReadDeadline(time.Now().Add(time.Second))
	ack := make([]byte, 64)
	n, err := joiner.Read(ack)
	if err != nil {
		t.Fatal(err)
	}
	if n != 9 || ack[8] != proto.PunchAckOK {
		t.Fatalf("expected PunchAckOK, got len=%d status=%d", n, ack[8])
	}

	// Host reads the notify (which carries the joiner's reflexive addr).
	host.SetReadDeadline(time.Now().Add(time.Second))
	notify := make([]byte, 64)
	n, err = host.Read(notify)
	if err != nil {
		t.Fatal(err)
	}
	if n != 14 || notify[2] != proto.PunchNotify {
		t.Fatalf("expected PunchNotify, got len=%d opcode=%d", n, notify[2])
	}
	joinerAddr := joiner.LocalAddr().(*net.UDPAddr)
	wantPort := uint16(joinerAddr.Port)
	gotPort := binary.BigEndian.Uint16(notify[12:14])
	if gotPort != wantPort {
		t.Errorf("notify port = %d, want %d (joiner)", gotPort, wantPort)
	}
}

func TestServerDropsMalformedInfoPacket(t *testing.T) {
	srv, cli, _ := startServer(t)

	// 76 bytes but bad signature
	bad := make([]byte, proto.InfoPacketSize)
	copy(bad[0:4], "Halo")
	bad[7] = proto.InfoResponseType
	cli.Write(bad)

	// Give it a moment, ensure no game registered.
	time.Sleep(100 * time.Millisecond)
	if srv.reg.Count() != 0 {
		t.Errorf("malformed packet registered: count=%d", srv.reg.Count())
	}
}

func TestServerVersionGateBlocksPre118NonZeroAdvertised(t *testing.T) {
	srv, cli, _ := startServer(t)

	// Pre-1.1.8 with a non-zero advertised serveraddress should be dropped.
	pkt := proto.EncodeInfo(proto.InfoPacket{
		VersionMajor: 1, VersionMinor: 1, VersionRevision: 5,
		MapName:       "old",
		ServerAddress: [4]byte{203, 0, 113, 1}, // non-zero
		ServerPort:    1,
		StartTime:     1,
	})
	cli.Write(pkt)
	time.Sleep(150 * time.Millisecond)
	if srv.reg.Count() != 0 {
		t.Errorf("pre-1.1.8 with non-zero advertised registered: count=%d", srv.reg.Count())
	}
}

// timeOfStartTime returns the LastPacket of the registered game whose
// StartTime matches the given token.
func timeOfStartTime(srv *Server, token uint32) time.Time {
	for _, g := range srv.reg.Snapshot() {
		if g.StartTime == token {
			return g.LastPacket
		}
	}
	return time.Time{}
}

func waitFor(t *testing.T, d time.Duration, cond func() bool) bool {
	t.Helper()
	deadline := time.Now().Add(d)
	for time.Now().Before(deadline) {
		if cond() {
			return true
		}
		time.Sleep(10 * time.Millisecond)
	}
	return cond()
}

// testWriter routes slog into t.Logf so logs only show on test failures.
type testWriter struct{ t *testing.T }

func (w testWriter) Write(p []byte) (int, error) {
	w.t.Logf("%s", p)
	return len(p), nil
}

// Use the real UDP types to placate the compiler about netip imports in case
// tests get pruned.
var _ = netip.AddrPort{}
