// tracker is the WinBolo tracker daemon — Go port of the C tracker.
//
// Listens on:
//
//	:50000/udp  — INFO_PACKET registration, WBKA keepalives, hole-punch
//	:50000/tcp  — full game list (text/MOTD format)
//	:50001/tcp  — interesting-games subset
//	:50005/tcp  — HTTP-ish web view
//
// Wire format on every port matches the C tracker byte-for-byte.
package main

import (
	"context"
	"flag"
	"log/slog"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/john-winbolo/tracker/go-tracker/internal/bans"
	"github.com/john-winbolo/tracker/go-tracker/internal/httpsrv"
	"github.com/john-winbolo/tracker/go-tracker/internal/registry"
	"github.com/john-winbolo/tracker/go-tracker/internal/stats"
	"github.com/john-winbolo/tracker/go-tracker/internal/tcp"
	"github.com/john-winbolo/tracker/go-tracker/internal/udp"
)

const (
	defaultUDPAddr         = ":50000"
	defaultTCPAddr         = ":50000"
	defaultInterestingAddr = ":50001"
	defaultHTTPAddr        = ":50005"
	purgeInterval          = 30 * time.Second
)

func main() {
	var (
		udpAddr  = flag.String("udp", defaultUDPAddr, "UDP listen address (INFO_PACKET, WBKA, punch)")
		tcpAddr  = flag.String("tcp", defaultTCPAddr, "TCP listen address (game list)")
		intAddr  = flag.String("interesting", defaultInterestingAddr, "TCP listen address (interesting-games subset)")
		httpAddr = flag.String("http", defaultHTTPAddr, "TCP listen address (web view)")
		bansFile = flag.String("bans", "", "Path to a bans file (one substring match per line; missing file is OK)")
		debug    = flag.Bool("debug", false, "Enable debug logging")
	)
	flag.Parse()

	level := slog.LevelInfo
	if *debug {
		level = slog.LevelDebug
	}
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: level}))
	slog.SetDefault(logger)

	bl, err := bans.Load(*bansFile)
	if err != nil {
		logger.Error("loading bans", "path", *bansFile, "err", err)
		os.Exit(1)
	}

	reg := registry.New()
	st := stats.New()

	udpSrv, err := udp.Listen(*udpAddr, reg, bl, st, logger)
	if err != nil {
		logger.Error("udp listen", "addr", *udpAddr, "err", err)
		os.Exit(1)
	}
	defer udpSrv.Close()
	logger.Info("listening", "udp", udpSrv.LocalAddr().String())

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	var wg sync.WaitGroup
	run := func(name string, fn func() error) {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := fn(); err != nil {
				logger.Error(name+" exited", "err", err)
				cancel()
			}
		}()
	}

	run("udp", func() error { return udpSrv.Serve(ctx) })
	run("tcp", func() error { return tcp.Serve(ctx, *tcpAddr, reg, st, tcp.FilterAll, logger) })
	run("interesting", func() error { return tcp.Serve(ctx, *intAddr, reg, st, tcp.FilterInteresting, logger) })
	run("http", func() error {
		return httpsrv.Serve(ctx, *httpAddr, reg, st,
			func() int64 { return st.PeakHTTPThreads.Load() },
			func() int64 { return st.PeakUDPThreads.Load() },
			logger)
	})
	run("purge", func() error {
		t := time.NewTicker(purgeInterval)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return nil
			case now := <-t.C:
				reg.Purge(now)
			}
		}
	})

	logger.Info("listening",
		"tcp", *tcpAddr, "interesting", *intAddr, "http", *httpAddr)

	<-ctx.Done()
	logger.Info("shutting down")
	wg.Wait()
}
