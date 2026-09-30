# WinBolo Tracker

The WinBolo tracker's main job today is **NAT traversal**. It keeps NAT mappings alive with keepalive packets and coordinates UDP hole-punching, so players behind home routers can connect to each other's games.

It also keeps the original game list. Game servers register over UDP, and clients fetch the list of live games over TCP or a simple HTTP view. This list is kept for **legacy WinBolo 1.x clients**. Modern game browsers should use [WBN (winbolo.net)](https://winbolo.net), which provides a richer data set.

Future plans for this server include matchmaking and similar features.

## Implementations

This repository contains two implementations of the same wire protocol:

| Directory     | Language | Status |
|---------------|----------|--------|
| `go-tracker/` | Go       | **Current.** This is what the Docker image builds and what runs in production. |
| `tracker/`    | C        | The original implementation (2000). Deprecated and kept for reference; it doesn't receive new features. |

The Go port produces the same bytes as the C tracker on every port, including its formatting quirks, because external consumers scrape the TCP and HTTP output. Keep that in mind before changing any output format.

## Ports

| Port        | Purpose |
|-------------|---------|
| `50000/udp` | Game registration (`INFO_PACKET`), `WBKA` NAT keepalives, and NAT hole-punch coordination |
| `50000/tcp` | Full game list, in text/MOTD format |
| `50001/tcp` | "Interesting" games only: not password-protected, fewer than 6 players, with unclaimed bases (or an open game) |
| `50005/tcp` | Web view of the game list plus tracker statistics |

Registrations expire if a server doesn't refresh them within 4 minutes 10 seconds.

### Protocol notes

- **Registration packets.** These come in two sizes. The legacy 76-byte `INFO_PACKET` is accepted from every client version. The 111-byte extended packet is accepted from WinBolo 1.8.8 and later; it adds the human and bot player counts, maximum players, flags, and a map MD5. If an older client sends the extended size, the packet is dropped.
- **Byte order.** Most fields are little-endian because the packet is a packed x86 struct. `StartTime` is the exception and is read big-endian. `ServerPort` is big-endian only for WinBolo 1.1.1–1.1.3.
- **Advertised addresses.** Clients from 1.1.8 onward may advertise a UPnP/NAT-PMP-mapped external address. For older clients, the packet's source address is always used.
- **Hole-punch packets.** These start with a `"WB"` header, use big-endian fields, and have opcodes 153–157.

`go-tracker/internal/proto` is the authoritative reference for the wire format. It contains every encoder and decoder, plus tests.

## Building and running

### Docker (recommended)

```bash
docker compose up --build
```

This builds a static Go binary in a multi-stage build and runs it on Alpine as an unprivileged user, with all four ports published.

### Go

Requires Go 1.22 or later.

```bash
cd go-tracker
go build -o tracker ./cmd/tracker
./tracker            # listen on the default ports
./tracker -debug     # verbose logging
./tracker -h         # list all flags
```

| Flag           | Default  | Description |
|----------------|----------|-------------|
| `-udp`         | `:50000` | UDP listen address (registration, keepalive, punch) |
| `-tcp`         | `:50000` | TCP listen address (full game list) |
| `-interesting` | `:50001` | TCP listen address (interesting-games subset) |
| `-http`        | `:50005` | Web view listen address |
| `-bans`        | none     | Bans file: one entry per line, matched as a substring of the server's reverse-DNS name (or its IP address if there is no reverse DNS). A missing file is fine. |
| `-debug`       | `false`  | Enable debug logging |

Run the tests:

```bash
cd go-tracker
go test ./...
go vet ./...
```

### C (reference only)

```bash
cd tracker && make
# or from the repo root:
cmake -S . -B build && cmake --build build
```

The C code builds on Linux (epoll, pthreads) and Windows (WSAPoll, Winsock).

## Project layout

```
go-tracker/
  cmd/tracker/        entry point: starts all listeners and purges expired games
  internal/proto/     packet encode/decode (no I/O)
  internal/registry/  live game set, keyed by (IP, port, start time)
  internal/udp/       registration, keepalive and hole-punch server
  internal/tcp/       game-list servers (ports 50000 and 50001)
  internal/httpsrv/   web view (port 50005)
  internal/bans/      substring ban list
  internal/stats/     counters shown in the TCP MOTD and on the web view
tracker/              original C implementation
```

## Deploying

The only thing the server needs is Docker with the Compose plugin. The Go binary is compiled during the image build, so you don't need a Go toolchain on the server.

### First install

```bash
git clone https://github.com/john-winbolo/tracker.git
cd tracker
docker compose up -d --build
```

Open these ports in the server's firewall. Port 50000 must be open for **both** TCP and UDP.

| Port  | Protocol  |
|-------|-----------|
| 50000 | TCP + UDP |
| 50001 | TCP       |
| 50005 | TCP       |

The container restarts automatically, including after a reboot (`restart: unless-stopped`).

### Updating

```bash
cd tracker
git pull
docker compose build      # the old container keeps serving while this runs
docker compose up -d      # replaces the container only if the image changed
docker image prune -f     # optional: remove old, unused images
```

Running `build` before `up` means a failed build leaves the running tracker untouched.

### Checking it's running

```bash
docker compose ps                 # status, including the health check
docker compose logs -f tracker    # follow the logs
nc localhost 50000                # should print the game list
```

The web view is at `http://<server>:50005/`.

### Changing options

The image's entrypoint is the tracker binary, so any command you set in Compose is passed to it as flags. For example, to enable debug logging and use a bans file, add this to the `tracker` service in `docker-compose.yml`:

```yaml
    command: ["-debug", "-bans", "/app/bans.txt"]
    volumes:
      - ./bans.txt:/app/bans.txt:ro
```

Then apply it with `docker compose up -d`.

### Without Compose

```bash
docker build -t winbolo-tracker .
docker run -d --name tracker --restart unless-stopped \
  -p 50000:50000/tcp -p 50000:50000/udp \
  -p 50001:50001/tcp -p 50005:50005/tcp \
  winbolo-tracker
```

## License

Copyright (c) 2000–2026 John Morrison and contributors.

This program is free software; you can redistribute it and/or modify it under the terms of the GNU General Public License as published by the Free Software Foundation; either version 3 of the License, or (at your option) any later version.

This program is distributed in the hope that it will be useful, but WITHOUT ANY WARRANTY; without even the implied warranty of MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the [LICENSE](LICENSE) file for details.
