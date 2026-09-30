// SPDX-License-Identifier: GPL-3.0-or-later

// Package proto encodes/decodes the WinBolo tracker wire protocol.
//
// Byte-exact port of the C tracker's UDP handlers. The C struct uses
// #pragma pack(1) and the original WinBolo client is x86 little-endian, so
// most multi-byte fields are native LE on the wire even though that's
// unusual for a network protocol. The hole-punch additions (PUNCH_*) follow
// a separate "WB" header convention with explicit big-endian fields.
package proto

import (
	"encoding/binary"
	"errors"
	"fmt"
)

const (
	InfoPacketSize = 76
	// InfoPacketSizeExtended is the size of the 111-byte INFO_PACKET sent by
	// WinBolo ≥1.8.8. The first 76 bytes are byte-compatible with the legacy
	// layout (the two former spare bytes are repurposed); 35 bytes are
	// appended. See DecodeInfo.
	InfoPacketSizeExtended = 111
	InfoPacketSignature    = "Bolo"
	InfoPacketTypeOffset   = 7
	InfoResponseType       = 14
)

// Bits in the extended INFO_PACKET flags byte (offset 59, was spare1). Only
// meaningful for the 111-byte format. 0x40/0x80 are reserved.
const (
	FlagAllowNewPlayers = 0x01 // server is accepting joins
	FlagLocked          = 0x02 // server locked / not accepting joins
	FlagRanked          = 0x04 // ranked game
	FlagRandomMap       = 0x08 // random map mode
	FlagAllowSpectators = 0x10 // spectators allowed (future; currently 0)
	FlagInLobby         = 0x20 // in pre-game lobby (else in-game)
)

// Hole-punch opcodes carried in byte 2 of a "WB"-headed packet.
const (
	PunchRequest      = 153
	PunchNotify       = 154
	PunchRequestAck   = 155
	PunchProbeRequest = 156
	PunchProbeReply   = 157
)

// Status byte values for PunchRequestAck.
const (
	PunchAckOK              = 0
	PunchAckHostNotFound    = 1
	PunchAckHostUnreachable = 2
)

// Game type from INFO_PACKET.gametype.
type GameType uint8

const (
	GameOpen             GameType = 1
	GameTournament       GameType = 2
	GameStrictTournament GameType = 3
)

// AI type from INFO_PACKET.allow_AI.
type AIType uint8

const (
	AINone         AIType = 0
	AIYes          AIType = 1
	AIYesAdvantage AIType = 2
	AIYesFull      AIType = 3
)

// InfoPacket is the decoded form of the UDP registration packet (the 76-byte
// legacy form, or the 111-byte extended form sent by WinBolo ≥1.8.8).
//
// MapName is decoded from the wire's Pascal string. ServerPort and StartTime
// are stored in the canonical form the registry uses (see DecodeInfo).
type InfoPacket struct {
	VersionMajor    uint8
	VersionMinor    uint8
	VersionRevision uint8

	MapName       string  // decoded from Pascal string (max 35 chars)
	ServerAddress [4]byte // network byte order (big-endian) — 0.0.0.0 means "use packet source"
	ServerPort    uint16  // canonical host-byte-order, see notes below
	StartTime     uint32  // canonical key value, see notes below

	GameType    GameType
	AllowMines  uint8 // 0x80 = hidden mines, 0xC0 = visible
	AllowAI     AIType
	StartDelay  int32
	TimeLimit   int32
	NumPlayers  uint16
	FreePills   uint16
	FreeBases   uint16
	HasPassword bool

	// Extended is true when this was decoded from the 111-byte format
	// (WinBolo ≥1.8.8). The fields below are only meaningful when Extended is
	// set; for the legacy 76-byte format they are zero values.
	Extended       bool
	Flags          uint8  // bitfield, see Flag* constants (offset 59, was spare1)
	SpectatorCount uint8  // offset 75, was spare2; 0 for now
	NumHumans      uint8  // human players among NumPlayers
	NumBots        uint8  // AI bots among NumPlayers (NumHumans+NumBots == NumPlayers)
	MaxPlayers     uint8  // server join-slot cap
	MapMD5         string // 32 lowercase hex chars; "" when zero-filled (no md5)
}

// HiddenMines reports whether the AllowMines byte indicates hidden-mines mode
// (0x80). The C tracker stores this as a bool with the same semantics.
func (p *InfoPacket) HiddenMines() bool { return p.AllowMines == 0x80 }

// VersionString formats the version as "major.minor revision" — note the
// missing dot between minor and revision. This matches the C tracker's
// sprintf("%d.%d%d", ...) bug-for-bug. e.g. 1.1.7 → "1.17".
func (p *InfoPacket) VersionString() string {
	return fmt.Sprintf("%d.%d%d", p.VersionMajor, p.VersionMinor, p.VersionRevision)
}

// IsLegacyByteSwappedPort reports whether this client's wire format put the
// server port in big-endian (true) vs little-endian (false). Only WinBolo
// 1.1.1, 1.1.2, and 1.1.3 sent BE; everything else is LE.
func (p *InfoPacket) isLegacyByteSwappedPort() bool {
	return p.VersionMajor == 1 && p.VersionMinor == 1 &&
		(p.VersionRevision == 1 || p.VersionRevision == 2 || p.VersionRevision == 3)
}

// AcceptsAdvertisedAddress reports whether this client version is permitted
// to advertise a non-zero ServerAddress (a UPnP/NAT-PMP/PCP-mapped external
// IP). Pre-1.1.8 clients sending non-zero are dropped as malformed.
func (p *InfoPacket) AcceptsAdvertisedAddress() bool {
	if p.VersionMajor > 1 {
		return true
	}
	if p.VersionMajor == 1 && p.VersionMinor > 1 {
		return true
	}
	if p.VersionMajor == 1 && p.VersionMinor == 1 && p.VersionRevision > 7 {
		return true
	}
	return false
}

// supportsExtendedInfo reports whether this client version (≥1.8.8) may use
// the 111-byte extended INFO_PACKET. Older clients sending the extended size
// are dropped as malformed. Note 1.8.8 is the VersionString "1.88".
func (p *InfoPacket) supportsExtendedInfo() bool {
	if p.VersionMajor != 1 {
		return p.VersionMajor > 1
	}
	if p.VersionMinor != 8 {
		return p.VersionMinor > 8
	}
	return p.VersionRevision >= 8
}

// DecodeInfo parses a 76-byte INFO_PACKET into structured form, applying the
// same canonicalization the C tracker does:
//   - StartTime is taken as the big-endian interpretation of wire bytes 52..55.
//     (The C code reads the field as native LE then applies htonl, which on
//     little-endian hosts produces the BE interpretation. The 8-byte WBKA
//     keepalive carries a token decoded the same way, so they match.)
//   - ServerPort is little-endian for modern clients (≥1.1.4) and big-endian
//     for legacy 1.1.1–1.1.3, matching the version-gated swap in udp.c.
//
// A 111-byte extended packet (WinBolo ≥1.8.8) is byte-compatible over its
// first 76 bytes — except the two former spare bytes (offsets 59 and 75) are
// repurposed as flags and spectator_count — and carries five appended fields.
// An extended packet claiming a version below 1.8.8 is rejected as malformed.
//
// Returns an error if length or signature doesn't match.
func DecodeInfo(b []byte) (InfoPacket, error) {
	var p InfoPacket
	if len(b) != InfoPacketSize && len(b) != InfoPacketSizeExtended {
		return p, fmt.Errorf("info: want %d or %d bytes, got %d", InfoPacketSize, InfoPacketSizeExtended, len(b))
	}
	if string(b[0:4]) != InfoPacketSignature {
		return p, errors.New("info: bad signature")
	}
	if b[InfoPacketTypeOffset] != InfoResponseType {
		return p, fmt.Errorf("info: bad packet type %d", b[InfoPacketTypeOffset])
	}

	p.VersionMajor = b[4]
	p.VersionMinor = b[5]
	p.VersionRevision = b[6]

	p.MapName = decodePascal(b[8:44])
	copy(p.ServerAddress[:], b[44:48])

	if p.isLegacyByteSwappedPort() {
		p.ServerPort = binary.BigEndian.Uint16(b[48:50])
	} else {
		p.ServerPort = binary.LittleEndian.Uint16(b[48:50])
	}
	// b[50:52] is padding
	p.StartTime = binary.BigEndian.Uint32(b[52:56])

	p.GameType = GameType(b[56])
	p.AllowMines = b[57]
	p.AllowAI = AIType(b[58])
	// b[59] spare1 (legacy) / flags (extended)
	p.StartDelay = int32(binary.LittleEndian.Uint32(b[60:64]))
	p.TimeLimit = int32(binary.LittleEndian.Uint32(b[64:68]))
	p.NumPlayers = binary.LittleEndian.Uint16(b[68:70])
	p.FreePills = binary.LittleEndian.Uint16(b[70:72])
	p.FreeBases = binary.LittleEndian.Uint16(b[72:74])
	p.HasPassword = b[74] != 0
	// b[75] spare2 (legacy) / spectator_count (extended)

	if len(b) == InfoPacketSizeExtended {
		if !p.supportsExtendedInfo() {
			return InfoPacket{}, fmt.Errorf("info: extended packet from unsupported version %s", p.VersionString())
		}
		p.Extended = true
		p.Flags = b[59]
		p.SpectatorCount = b[75]
		p.NumHumans = b[76]
		p.NumBots = b[77]
		p.MaxPlayers = b[78]
		// map_md5 is 32 fixed hex chars, not NUL-terminated. A leading 0x00
		// means "no md5" (random/unknown map) — leave MapMD5 empty.
		if b[79] != 0 {
			p.MapMD5 = string(b[79:111])
		}
	}

	return p, nil
}

// EncodeInfo produces the canonical wire bytes for an InfoPacket. The C
// tracker doesn't send INFO_PACKETs (it only receives them), so this is for
// tests only — round-tripping packets to verify byte-exact decode.
func EncodeInfo(p InfoPacket) []byte {
	size := InfoPacketSize
	if p.Extended {
		size = InfoPacketSizeExtended
	}
	b := make([]byte, size)
	copy(b[0:4], []byte(InfoPacketSignature))
	b[4] = p.VersionMajor
	b[5] = p.VersionMinor
	b[6] = p.VersionRevision
	b[InfoPacketTypeOffset] = InfoResponseType
	encodePascal(b[8:44], p.MapName)
	copy(b[44:48], p.ServerAddress[:])
	if p.isLegacyByteSwappedPort() {
		binary.BigEndian.PutUint16(b[48:50], p.ServerPort)
	} else {
		binary.LittleEndian.PutUint16(b[48:50], p.ServerPort)
	}
	binary.BigEndian.PutUint32(b[52:56], p.StartTime)
	b[56] = byte(p.GameType)
	b[57] = p.AllowMines
	b[58] = byte(p.AllowAI)
	binary.LittleEndian.PutUint32(b[60:64], uint32(p.StartDelay))
	binary.LittleEndian.PutUint32(b[64:68], uint32(p.TimeLimit))
	binary.LittleEndian.PutUint16(b[68:70], p.NumPlayers)
	binary.LittleEndian.PutUint16(b[70:72], p.FreePills)
	binary.LittleEndian.PutUint16(b[72:74], p.FreeBases)
	if p.HasPassword {
		b[74] = 1
	}
	if p.Extended {
		b[59] = p.Flags
		b[75] = p.SpectatorCount
		b[76] = p.NumHumans
		b[77] = p.NumBots
		b[78] = p.MaxPlayers
		copy(b[79:111], p.MapMD5)
	}
	return b
}

// decodePascal decodes a 36-byte WinBolo Pascal string (1 byte length, then
// up to 35 chars). Mirrors utilPtoCString — caps length at 35.
func decodePascal(b []byte) string {
	n := int(b[0])
	if n > 35 {
		n = 35
	}
	if n > len(b)-1 {
		n = len(b) - 1
	}
	return string(b[1 : 1+n])
}

func encodePascal(dst []byte, s string) {
	n := len(s)
	if n > 35 {
		n = 35
	}
	dst[0] = byte(n)
	copy(dst[1:1+n], s)
}

// IsWBKA4 reports whether b is a 4-byte "WBKA" keepalive.
func IsWBKA4(b []byte) bool {
	return len(b) == 4 && b[0] == 'W' && b[1] == 'B' && b[2] == 'K' && b[3] == 'A'
}

// IsWBKA8 reports whether b is an 8-byte "WBKA" keepalive carrying a token.
func IsWBKA8(b []byte) bool {
	return len(b) == 8 && b[0] == 'W' && b[1] == 'B' && b[2] == 'K' && b[3] == 'A'
}

// WBKA8Token extracts the big-endian uint32 token from an 8-byte WBKA. The
// token is INFO_PACKET.gameid.start_time as stored in the registry (see
// DecodeInfo for the canonicalization).
func WBKA8Token(b []byte) uint32 {
	return binary.BigEndian.Uint32(b[4:8])
}

// IsPunchProbeRequest reports whether b is an 8-byte "WB"+156 STUN-style
// self-probe.
func IsPunchProbeRequest(b []byte) bool {
	return len(b) == 8 && b[0] == 'W' && b[1] == 'B' && b[2] == PunchProbeRequest
}

// IsPunchRequest reports whether b is a 14-byte "WB"+153 hole-punch request
// from a joiner.
func IsPunchRequest(b []byte) bool {
	return len(b) == 14 && b[0] == 'W' && b[1] == 'B' && b[2] == PunchRequest
}

// PunchRequestTarget extracts (target IP in network order, target port host
// order) from a 14-byte PunchRequest.
func PunchRequestTarget(b []byte) (ipNetOrder [4]byte, port uint16) {
	copy(ipNetOrder[:], b[8:12])
	port = binary.BigEndian.Uint16(b[12:14])
	return
}

// EncodePunchAck builds the 9-byte PunchRequestAck.
//
//	0  'W'
//	1  'B'
//	2  PunchRequestAck
//	3  reserved (0)
//	4..7  sequence (BE u32, unused)
//	8  status (PunchAck*)
func EncodePunchAck(status uint8) []byte {
	b := make([]byte, 9)
	b[0] = 'W'
	b[1] = 'B'
	b[2] = PunchRequestAck
	b[8] = status
	return b
}

// EncodePunchNotify builds the 14-byte PunchNotify sent to a host's live NAT
// mapping carrying the joiner's reflexive address.
//
//	0  'W'
//	1  'B'
//	2  PunchNotify
//	3  reserved (0)
//	4..7  sequence (BE u32, unused)
//	8..11  joiner reflexive IP (network byte order)
//	12..13 joiner reflexive port (BE u16)
func EncodePunchNotify(joinerIP [4]byte, joinerPort uint16) []byte {
	b := make([]byte, 14)
	b[0] = 'W'
	b[1] = 'B'
	b[2] = PunchNotify
	copy(b[8:12], joinerIP[:])
	binary.BigEndian.PutUint16(b[12:14], joinerPort)
	return b
}

// EncodePunchProbeReply builds the 14-byte PunchProbeReply echoing the
// requester's reflexive address.
func EncodePunchProbeReply(reflexiveIP [4]byte, reflexivePort uint16) []byte {
	b := make([]byte, 14)
	b[0] = 'W'
	b[1] = 'B'
	b[2] = PunchProbeReply
	copy(b[8:12], reflexiveIP[:])
	binary.BigEndian.PutUint16(b[12:14], reflexivePort)
	return b
}
