// Package stats holds the request counters reported in TCP/HTTP responses.
//
// The C tracker had a stats.c file full of stub no-op functions
// (statsLogConnection, statsSaveGameStart, etc.) and a few global counter
// long ints in main.c. The counters are all that show up externally — they
// appear in TCP MOTD lines and the HTTP status block, both of which winbolo.net
// scrapes — so byte-exact parity requires we increment them at the same call
// sites the C tracker did.
package stats

import "sync/atomic"

type Counters struct {
	Games       atomic.Int64 // unique games ever registered (statsGames)
	TCP         atomic.Int64 // TCP game-list connections (statsTcp)
	UDP         atomic.Int64 // UDP info packets received (statsUdp) — declared but never incremented in C
	HTTP        atomic.Int64 // HTTP requests (statsHttp)
	Interesting atomic.Int64 // interesting-list connections (statsInteresting)

	// PeakHTTPThreads / PeakUDPThreads expose the high-water-mark
	// concurrency the HTTP page reports. The C tracker reported the live
	// value of httpNumThreads / udpNumThreads (which were monotonically
	// increasing thread-pool sizes, not actually current concurrency).
	// We mimic that by tracking a peak of in-flight handlers.
	PeakHTTPThreads atomic.Int64
	PeakUDPThreads  atomic.Int64
}

// New returns a zeroed Counters.
func New() *Counters { return &Counters{} }

// Bump atomically increments and returns the new value.
func bump(c *atomic.Int64) int64 { return c.Add(1) }

func (c *Counters) IncGames() int64       { return bump(&c.Games) }
func (c *Counters) IncTCP() int64         { return bump(&c.TCP) }
func (c *Counters) IncUDP() int64         { return bump(&c.UDP) }
func (c *Counters) IncHTTP() int64        { return bump(&c.HTTP) }
func (c *Counters) IncInteresting() int64 { return bump(&c.Interesting) }

// ObservePeak records v as a candidate peak; the stored value only goes up.
func observePeak(c *atomic.Int64, v int64) {
	for {
		cur := c.Load()
		if v <= cur {
			return
		}
		if c.CompareAndSwap(cur, v) {
			return
		}
	}
}

func (c *Counters) ObserveHTTPThreads(v int64) { observePeak(&c.PeakHTTPThreads, v) }
func (c *Counters) ObserveUDPThreads(v int64)  { observePeak(&c.PeakUDPThreads, v) }
