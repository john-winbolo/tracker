package httpsrv

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/john-winbolo/tracker/go-tracker/internal/proto"
	"github.com/john-winbolo/tracker/go-tracker/internal/registry"
	"github.com/john-winbolo/tracker/go-tracker/internal/stats"
)

func TestEmptyResponseExactBytes(t *testing.T) {
	reg := registry.New()
	st := stats.New()

	var b bytes.Buffer
	if err := WriteResponse(&b, reg, st, 0, 0); err != nil {
		t.Fatal(err)
	}
	out := b.Bytes()

	// Must begin with the literal header followed by a NUL byte (matching
	// the C tracker's sizeof-on-string-literal behaviour).
	wantHeader := "HTTP/1.1 200 OK\nContent-Type: text/html\n\n" +
		"<HTML><BODY><meta http-equiv=\"pragma\" content=\"no-cache\"></meta>" +
		"This is an html output of the game list<BR><BR>\r\n"
	if !bytes.HasPrefix(out, []byte(wantHeader)) {
		t.Fatalf("missing header prefix; got first 200 bytes: %q", string(out[:200]))
	}
	if out[len(wantHeader)] != 0 {
		t.Errorf("expected NUL byte after header, got %q", out[len(wantHeader)])
	}

	// Stats and footer should be present.
	rest := string(out[len(wantHeader)+1:])
	wantSubstr := []string{
		"Tracker Statistics<BR>\r\n",
		"Number of Tracked Games: 0<BR>\r\n",
		"Number of Tracker Game List Requests: 0<BR>\r\n",
		"Number of Interesting Game List Requests: 0<BR><BR>\r\n",
		"Number of Tracker Web Requests: 0<BR><BR>\r\n",
		"Most http requests at one time: 0<BR>\r\n",
		"Most udp requests at one time: 0<BR><BR>\r\n",
		"There are no games at the moment - maybe you could start one.\r\n",
		"</PRE></BODY></HTML>\r\n",
	}
	for _, s := range wantSubstr {
		if !strings.Contains(rest, s) {
			t.Errorf("missing %q\nfull rest:\n%s", s, rest)
		}
	}
}

func TestSingleGameResponse(t *testing.T) {
	reg := registry.New()
	st := stats.New()
	reg.Update(registry.Game{
		Key:         registry.Key{IP: [4]byte{10, 0, 0, 1}, Port: 50000, StartTime: 1},
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
	}, time.Now())

	var b bytes.Buffer
	WriteResponse(&b, reg, st, 0, 0)
	out := b.String()

	// Single-game uses the singular phrasing.
	if !strings.Contains(out, "There is currently 1 game.<BR><BR><PRE>\r\n\n") {
		t.Errorf("missing singular game count line")
	}
	wantGameLine := `Host: <B><A HREF=winbolo://host.example:50000>host.example:50000</A></B>  Version: <B>1.18</B>  Players: <B>3</B>  Bases: <B>4</B>  Pills: <B>16</B>` + "\n" +
		`Map: <B>everard</B>  Game: <B>Open</B>  Hidden Mines: <I>Yes</I>  Bots: <I>yes</I>  PW: <I>No</I>` + "\n\n"
	if !strings.Contains(out, wantGameLine) {
		t.Errorf("missing game line; got:\n%s\n\nwant:\n%s", out, wantGameLine)
	}
}

func TestMultipleGamesResponse(t *testing.T) {
	reg := registry.New()
	st := stats.New()
	now := time.Now()
	reg.Update(registry.Game{Key: registry.Key{IP: [4]byte{1, 0, 0, 0}, Port: 1, StartTime: 1}, Address: "a"}, now)
	reg.Update(registry.Game{Key: registry.Key{IP: [4]byte{2, 0, 0, 0}, Port: 2, StartTime: 2}, Address: "b"}, now.Add(time.Second))

	var b bytes.Buffer
	WriteResponse(&b, reg, st, 0, 0)
	out := b.String()
	if !strings.Contains(out, "There are currently 2 games.<BR><BR><PRE>\r\n\n") {
		t.Errorf("missing plural game count line")
	}
}

func TestPeakStatsRendered(t *testing.T) {
	reg := registry.New()
	st := stats.New()
	var b bytes.Buffer
	WriteResponse(&b, reg, st, 7, 9)
	out := b.String()
	if !strings.Contains(out, "Most http requests at one time: 7<BR>\r\n") {
		t.Error("peak HTTP not rendered")
	}
	if !strings.Contains(out, "Most udp requests at one time: 9<BR><BR>\r\n") {
		t.Error("peak UDP not rendered")
	}
}
