// SPDX-License-Identifier: GPL-3.0-or-later

package tcp

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/john-winbolo/tracker/go-tracker/internal/proto"
	"github.com/john-winbolo/tracker/go-tracker/internal/registry"
	"github.com/john-winbolo/tracker/go-tracker/internal/stats"
)

func TestEmptyAllResponse(t *testing.T) {
	reg := registry.New()
	st := stats.New()
	st.Games.Store(0)
	st.TCP.Store(7)
	st.Interesting.Store(2)
	st.HTTP.Store(11)

	var b bytes.Buffer
	if err := WriteGameList(&b, reg, st, FilterAll); err != nil {
		t.Fatal(err)
	}
	want := "TVERSION=1\r\n" +
		"MOTDL=6\r\n" +
		"MOTD=This is a simple tracker\r\n" +
		"MOTD=Tracker Statistics\r\n" +
		"MOTD=Number of Tracked Games: 0\r\n" +
		"MOTD=Number of Tracker Game List Requests: 7\r\n" +
		"MOTD=Number of Tracker Interesting Game List Requests: 2\r\n" +
		"MOTD=Number of Tracker Web Requests: 11\r\n" +
		"NGAMES=0\r\n"
	if b.String() != want {
		t.Errorf("output mismatch:\nGOT:\n%q\n\nWANT:\n%q", b.String(), want)
	}
}

func TestEmptyInterestingResponse(t *testing.T) {
	reg := registry.New()
	st := stats.New()

	var b bytes.Buffer
	if err := WriteGameList(&b, reg, st, FilterInteresting); err != nil {
		t.Fatal(err)
	}
	want := "TVERSION=1\r\n" +
		"MOTDL=10\r\n" +
		"MOTD=This is a simple tracker\r\n" +
		"MOTD=\r\n" +
		"MOTD=This is the INTERESTING games list, where interesting means\r\n" +
		"MOTD=non-PW, < 6 players, and unclaimed bases (or open game).\r\n" +
		"MOTD=\r\n" +
		"MOTD=Tracker Statistics\r\n" +
		"MOTD=Number of Tracked Games: 0\r\n" +
		"MOTD=Number of Tracker Game List Requests: 0\r\n" +
		"MOTD=Number of Tracker Interesting Game List Requests: 0\r\n" +
		"MOTD=Number of Tracker Web Requests: 0\r\n" +
		"NGAMES=0\r\n"
	if b.String() != want {
		t.Errorf("output mismatch:\nGOT:\n%q\n\nWANT:\n%q", b.String(), want)
	}
}

func TestSingleGameAllFilter(t *testing.T) {
	reg := registry.New()
	st := stats.New()
	now := time.Unix(1700000000, 0)
	reg.Update(registry.Game{
		Key:         registry.Key{IP: [4]byte{10, 0, 0, 1}, Port: 50000, StartTime: 0x12345678},
		Address:     "host.example",
		MapName:     "everard",
		Version:     "1.18",
		NumPlayers:  3,
		NumBases:    4,
		NumPills:    16,
		HiddenMines: true,
		GameType:    proto.GameOpen,
		AI:          proto.AIYes,
		Password:    false,
		StartDelay:  100,
		TimeLimit:   200,
	}, now)

	var b bytes.Buffer
	if err := WriteGameList(&b, reg, st, FilterAll); err != nil {
		t.Fatal(err)
	}
	got := b.String()
	mustContain(t, got, "NGAMES=1\r\n")
	mustContain(t, got, "GAME000=host.example:50000\r\n")
	mustContain(t, got, "VERSION=1.18\r\n")
	mustContain(t, got, "MAP=everard\r\n")
	mustContain(t, got, "TYPE=Open\r\n")
	mustContain(t, got, "PLAYERS=3\r\n")
	mustContain(t, got, "BASES=4\r\n")
	mustContain(t, got, "PILLS=16\r\n")
	mustContain(t, got, "HIDMINES=Yes\r\n")
	mustContain(t, got, "PASSWORD=No\r\n")
	mustContain(t, got, "BRAINS=yes\r\n")
	mustContain(t, got, "DELAY=100\r\n")
	mustContain(t, got, "LIMIT=200\r\n")
	mustContain(t, got, "STARTTIME=305419896\r\n") // 0x12345678
}

func TestInterestingFilter(t *testing.T) {
	reg := registry.New()
	st := stats.New()
	now := time.Unix(1700000000, 0)

	// Game 1: passworded → not interesting
	reg.Update(registry.Game{
		Key:      registry.Key{IP: [4]byte{1, 0, 0, 0}, Port: 1, StartTime: 1},
		Address:  "passworded",
		Password: true,
		NumBases: 4,
	}, now)
	// Game 2: open → interesting regardless of player count
	reg.Update(registry.Game{
		Key:        registry.Key{IP: [4]byte{2, 0, 0, 0}, Port: 2, StartTime: 2},
		Address:    "open",
		GameType:   proto.GameOpen,
		NumPlayers: 8,
	}, now.Add(time.Second))
	// Game 3: < 6 players, > 0 bases → interesting
	reg.Update(registry.Game{
		Key:        registry.Key{IP: [4]byte{3, 0, 0, 0}, Port: 3, StartTime: 3},
		Address:    "small",
		NumPlayers: 3,
		NumBases:   2,
	}, now.Add(2*time.Second))
	// Game 4: full and tournament → not interesting
	reg.Update(registry.Game{
		Key:        registry.Key{IP: [4]byte{4, 0, 0, 0}, Port: 4, StartTime: 4},
		Address:    "full",
		NumPlayers: 8,
		NumBases:   4,
		GameType:   proto.GameTournament,
	}, now.Add(3*time.Second))

	var b bytes.Buffer
	if err := WriteGameList(&b, reg, st, FilterInteresting); err != nil {
		t.Fatal(err)
	}
	out := b.String()
	mustContain(t, out, "NGAMES=2\r\n")
	mustContain(t, out, "GAME000=open:2\r\n")
	mustContain(t, out, "GAME001=small:3\r\n")
	if strings.Contains(out, "passworded") {
		t.Error("passworded game appeared in interesting list")
	}
	if strings.Contains(out, "full:4") {
		t.Error("full+tournament game appeared in interesting list")
	}
}

func TestGameTypeAndAIStrings(t *testing.T) {
	cases := []struct {
		gt   proto.GameType
		ai   proto.AIType
		want string
	}{
		{proto.GameOpen, proto.AINone, "TYPE=Open\r\nPLAYERS=0\r\nBASES=0\r\nPILLS=0\r\nHIDMINES=No\r\nPASSWORD=No\r\nBRAINS=no"},
		{proto.GameTournament, proto.AIYes, "TYPE=Tournament\r\nPLAYERS=0\r\nBASES=0\r\nPILLS=0\r\nHIDMINES=No\r\nPASSWORD=No\r\nBRAINS=yes"},
		{proto.GameStrictTournament, proto.AIYesAdvantage, "TYPE=Strict\r\nPLAYERS=0\r\nBASES=0\r\nPILLS=0\r\nHIDMINES=No\r\nPASSWORD=No\r\nBRAINS=yesAdv"},
		{proto.GameType(99), proto.AIYesFull, "TYPE=Strict\r\nPLAYERS=0\r\nBASES=0\r\nPILLS=0\r\nHIDMINES=No\r\nPASSWORD=No\r\nBRAINS=yesFull"},
	}
	for i, c := range cases {
		reg := registry.New()
		st := stats.New()
		reg.Update(registry.Game{
			Key:      registry.Key{IP: [4]byte{1, 1, 1, 1}, Port: 1, StartTime: uint32(i + 1)},
			Address:  "h",
			GameType: c.gt,
			AI:       c.ai,
		}, time.Now())

		var b bytes.Buffer
		WriteGameList(&b, reg, st, FilterAll)
		mustContain(t, b.String(), c.want)
	}
}

func mustContain(t *testing.T, s, want string) {
	t.Helper()
	if !strings.Contains(s, want) {
		t.Errorf("output missing %q\nfull output:\n%s", want, s)
	}
}
