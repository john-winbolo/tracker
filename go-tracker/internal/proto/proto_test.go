// SPDX-License-Identifier: GPL-3.0-or-later

package proto

import (
	"bytes"
	"encoding/binary"
	"testing"
)

func TestInfoPacketRoundTrip(t *testing.T) {
	in := InfoPacket{
		VersionMajor:    1,
		VersionMinor:    1,
		VersionRevision: 8,
		MapName:         "everard.map",
		ServerAddress:   [4]byte{203, 0, 113, 7},
		ServerPort:      50000,
		StartTime:       0xDEADBEEF,
		GameType:        GameOpen,
		AllowMines:      0x80,
		AllowAI:         AIYes,
		StartDelay:      0,
		TimeLimit:       0,
		NumPlayers:      3,
		FreePills:       12,
		FreeBases:       4,
		HasPassword:     false,
	}
	wire := EncodeInfo(in)
	if len(wire) != InfoPacketSize {
		t.Fatalf("len = %d, want %d", len(wire), InfoPacketSize)
	}
	out, err := DecodeInfo(wire)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if out != in {
		t.Errorf("round trip mismatch:\n got %+v\nwant %+v", out, in)
	}
}

// TestStartTimeBigEndian verifies the canonicalization that lets WBKA8 tokens
// match registry keys: an INFO_PACKET with start_time wire bytes [a,b,c,d]
// must produce the same uint32 as an 8-byte WBKA whose tail four bytes are
// [a,b,c,d].
func TestStartTimeBigEndian(t *testing.T) {
	wire := EncodeInfo(InfoPacket{
		VersionMajor:    1,
		VersionMinor:    2,
		VersionRevision: 0,
		StartTime:       0x01020304,
	})
	if !bytes.Equal(wire[52:56], []byte{0x01, 0x02, 0x03, 0x04}) {
		t.Errorf("start_time wire bytes = %x, want 01020304", wire[52:56])
	}

	wbka := []byte{'W', 'B', 'K', 'A', 0x01, 0x02, 0x03, 0x04}
	if got := WBKA8Token(wbka); got != 0x01020304 {
		t.Errorf("WBKA8 token = %x, want 0x01020304", got)
	}
}

// TestServerPortLegacySwap verifies the version-gated port byte order.
func TestServerPortLegacySwap(t *testing.T) {
	// Modern 1.1.4 — LE wire
	w := EncodeInfo(InfoPacket{VersionMajor: 1, VersionMinor: 1, VersionRevision: 4, ServerPort: 0x1234})
	if !bytes.Equal(w[48:50], []byte{0x34, 0x12}) {
		t.Errorf("modern wire port = %x, want 3412 (LE)", w[48:50])
	}

	// Legacy 1.1.2 — BE wire
	w = EncodeInfo(InfoPacket{VersionMajor: 1, VersionMinor: 1, VersionRevision: 2, ServerPort: 0x1234})
	if !bytes.Equal(w[48:50], []byte{0x12, 0x34}) {
		t.Errorf("legacy wire port = %x, want 1234 (BE)", w[48:50])
	}
}

func TestDecodeInfoBadSig(t *testing.T) {
	b := make([]byte, InfoPacketSize)
	copy(b[0:4], "Halo")
	if _, err := DecodeInfo(b); err == nil {
		t.Error("expected error on bad signature")
	}
}

func TestDecodeInfoBadType(t *testing.T) {
	b := make([]byte, InfoPacketSize)
	copy(b[0:4], InfoPacketSignature)
	b[InfoPacketTypeOffset] = 99
	if _, err := DecodeInfo(b); err == nil {
		t.Error("expected error on bad type")
	}
}

func TestDecodeInfoWrongLen(t *testing.T) {
	if _, err := DecodeInfo(make([]byte, 75)); err == nil {
		t.Error("expected error on short packet")
	}
	if _, err := DecodeInfo(make([]byte, 77)); err == nil {
		t.Error("expected error on long packet")
	}
}

func TestPascalString(t *testing.T) {
	in := InfoPacket{
		VersionMajor:    1,
		VersionMinor:    2,
		VersionRevision: 0,
		MapName:         "test",
	}
	w := EncodeInfo(in)
	if w[8] != 4 {
		t.Errorf("pascal len = %d, want 4", w[8])
	}
	if string(w[9:13]) != "test" {
		t.Errorf("pascal data = %q, want test", w[9:13])
	}
	out, _ := DecodeInfo(w)
	if out.MapName != "test" {
		t.Errorf("decoded mapname = %q, want test", out.MapName)
	}
}

// TestExtendedInfoRoundTrip checks the 111-byte format encodes/decodes
// byte-exactly, including the repurposed spare bytes and appended fields.
func TestExtendedInfoRoundTrip(t *testing.T) {
	in := InfoPacket{
		VersionMajor:    1,
		VersionMinor:    8,
		VersionRevision: 8,
		MapName:         "everard.map",
		ServerAddress:   [4]byte{203, 0, 113, 7},
		ServerPort:      50000,
		StartTime:       0xDEADBEEF,
		GameType:        GameOpen,
		AllowMines:      0x80,
		AllowAI:         AINone,
		NumPlayers:      5,
		FreePills:       12,
		FreeBases:       4,
		HasPassword:     true,
		Extended:        true,
		Flags:           FlagAllowNewPlayers | FlagInLobby,
		SpectatorCount:  0,
		NumHumans:       3,
		NumBots:         2,
		MaxPlayers:      16,
		MapMD5:          "0123456789abcdef0123456789abcdef",
	}
	wire := EncodeInfo(in)
	if len(wire) != InfoPacketSizeExtended {
		t.Fatalf("len = %d, want %d", len(wire), InfoPacketSizeExtended)
	}
	out, err := DecodeInfo(wire)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if out != in {
		t.Errorf("round trip mismatch:\n got %+v\nwant %+v", out, in)
	}
}

// TestExtendedInfoVersionGate checks the 111-byte format is only accepted from
// WinBolo ≥1.8.8; older versions sending the extended size are rejected.
func TestExtendedInfoVersionGate(t *testing.T) {
	cases := []struct {
		major, minor, rev uint8
		wantOK            bool
	}{
		{1, 8, 8, true},   // exactly the gate
		{1, 8, 9, true},   // newer revision
		{1, 9, 0, true},   // newer minor
		{2, 0, 0, true},   // newer major
		{1, 8, 7, false},  // one below
		{1, 7, 99, false}, // older minor
		{1, 1, 8, false},  // legacy
	}
	for _, c := range cases {
		wire := EncodeInfo(InfoPacket{
			VersionMajor: c.major, VersionMinor: c.minor, VersionRevision: c.rev,
			Extended: true,
		})
		_, err := DecodeInfo(wire)
		if (err == nil) != c.wantOK {
			t.Errorf("%d.%d.%d: err=%v, wantOK=%v", c.major, c.minor, c.rev, err, c.wantOK)
		}
	}
}

// TestExtendedInfoEmptyMD5 verifies a zero-filled md5 field decodes to "".
func TestExtendedInfoEmptyMD5(t *testing.T) {
	wire := EncodeInfo(InfoPacket{
		VersionMajor: 1, VersionMinor: 8, VersionRevision: 8,
		Extended: true,
		// MapMD5 left empty → 32 zero bytes on the wire.
	})
	if !bytes.Equal(wire[79:111], make([]byte, 32)) {
		t.Errorf("md5 field = %x, want 32 zero bytes", wire[79:111])
	}
	out, err := DecodeInfo(wire)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if out.MapMD5 != "" {
		t.Errorf("MapMD5 = %q, want empty", out.MapMD5)
	}
}

// TestExtendedInfoRepurposedSpares verifies offsets 59 and 75 carry flags /
// spectator_count in the extended format (they were spare1/spare2 before).
func TestExtendedInfoRepurposedSpares(t *testing.T) {
	wire := EncodeInfo(InfoPacket{
		VersionMajor: 1, VersionMinor: 8, VersionRevision: 8,
		Extended:       true,
		Flags:          FlagRanked,
		SpectatorCount: 7,
	})
	if wire[59] != FlagRanked {
		t.Errorf("wire[59] = %#x, want %#x", wire[59], FlagRanked)
	}
	if wire[75] != 7 {
		t.Errorf("wire[75] = %d, want 7", wire[75])
	}
}

func TestPunchRequestRoundTrip(t *testing.T) {
	b := make([]byte, 14)
	b[0] = 'W'
	b[1] = 'B'
	b[2] = PunchRequest
	copy(b[8:12], []byte{10, 0, 0, 5})
	binary.BigEndian.PutUint16(b[12:14], 7777)

	if !IsPunchRequest(b) {
		t.Fatal("not recognized as PunchRequest")
	}
	ip, port := PunchRequestTarget(b)
	if ip != [4]byte{10, 0, 0, 5} {
		t.Errorf("ip = %v, want [10 0 0 5]", ip)
	}
	if port != 7777 {
		t.Errorf("port = %d, want 7777", port)
	}
}

func TestEncodePunchNotify(t *testing.T) {
	b := EncodePunchNotify([4]byte{1, 2, 3, 4}, 5555)
	if len(b) != 14 || b[0] != 'W' || b[1] != 'B' || b[2] != PunchNotify {
		t.Fatalf("bad header: %x", b[0:3])
	}
	if !bytes.Equal(b[8:12], []byte{1, 2, 3, 4}) {
		t.Errorf("ip bytes wrong: %x", b[8:12])
	}
	if binary.BigEndian.Uint16(b[12:14]) != 5555 {
		t.Errorf("port wrong: %x", b[12:14])
	}
}

func TestEncodePunchAck(t *testing.T) {
	b := EncodePunchAck(PunchAckHostNotFound)
	if len(b) != 9 {
		t.Fatalf("len = %d", len(b))
	}
	if b[8] != PunchAckHostNotFound {
		t.Errorf("status = %d", b[8])
	}
}

func TestVersionString(t *testing.T) {
	// Must reproduce C tracker's "%d.%d%d" — note the missing dot.
	p := InfoPacket{VersionMajor: 1, VersionMinor: 1, VersionRevision: 7}
	if got := p.VersionString(); got != "1.17" {
		t.Errorf("version = %q, want 1.17", got)
	}
	p = InfoPacket{VersionMajor: 1, VersionMinor: 1, VersionRevision: 8}
	if got := p.VersionString(); got != "1.18" {
		t.Errorf("version = %q, want 1.18", got)
	}
}

func TestAcceptsAdvertisedAddress(t *testing.T) {
	cases := []struct {
		major, minor, rev uint8
		want              bool
	}{
		{1, 1, 7, false},
		{1, 1, 8, true},
		{1, 2, 0, true},
		{2, 0, 0, true},
		{1, 0, 99, false},
	}
	for _, c := range cases {
		p := InfoPacket{VersionMajor: c.major, VersionMinor: c.minor, VersionRevision: c.rev}
		if got := p.AcceptsAdvertisedAddress(); got != c.want {
			t.Errorf("%d.%d.%d: got %v, want %v", c.major, c.minor, c.rev, got, c.want)
		}
	}
}
