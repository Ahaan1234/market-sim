# market-sim

A self-contained market simulation system. A C++ engine runs a synthetic order book and emits a live JSON event stream; a Go server broadcasts that stream over WebSockets, persists tick history in SQLite, and manages sandboxed Python trader processes in Docker.

---

## Architecture

```
┌─────────────────────────────────────────────────────────┐
│                     Go Server (server/)                 │
│                                                         │
│  ┌──────────┐  ┌─────────┐  ┌───────────┐  ┌────────┐  │
│  │ engine/  │  │  ws/    │  │ sandbox/  │  │  db/   │  │
│  │ Runner   │  │  Hub    │  │ Manager   │  │ Store  │  │
│  └────┬─────┘  └────┬────┘  └─────┬─────┘  └───┬────┘  │
│       │stdout       │WS           │Unix sock    │SQLite │
└───────┼─────────────┼─────────────┼─────────────┼───────┘
        │             │             │             │
  ┌─────┴──────┐  browser      sandbox        tick
  │ C++ engine │  clients    containers      history
  │ (quantsim) │
  └────────────┘
```

The engine is the single source of truth. Everything else is wiring.

---

## Components

### 1. C++ Simulation Engine (`src/`)

The engine runs forever, emitting newline-delimited JSON events to stdout and accepting order JSON on stdin. It simulates:

- **PriceEngine** — geometric random walk with configurable volatility, bid/ask spread, and a $10 price floor
- **OrderBook** — price-time priority limit order book with market and limit order matching
- **Three built-in agent types** running every tick:
  - `MakerTrader` — posts resting limit orders on both sides of the book
  - `TakerTrader` — crosses the spread with market orders
  - `WhaleTrader` — places occasional large orders that move the price
- **PositionTracker** — per-trader P&L accounting (realized and unrealized), volume-weighted average cost
- **OrderQueue** — thread-safe queue bridging the stdin reader thread and the main simulation loop
- **Protocol.h** — hand-rolled JSON parser for incoming orders; never throws

#### Output events (stdout)

**Tick** — emitted every simulation step:
```json
{
  "type": "tick",
  "tick": 42,
  "ts": 1717200000,
  "price": 103.45,
  "bid": 103.20,
  "ask": 103.70,
  "spread": 0.50,
  "positions": [
    { "trader_id": "maker_0", "net_qty": 12.0, "unrealised_pnl": 4.20, "realised_pnl": 31.00 }
  ]
}
```

`positions` is included only every 10 ticks.

**Fill** — when an incoming order executes:
```json
{ "type": "fill", "trader_id": "my_bot", "order_id": "ord_001", "side": "BUY", "price": 103.20, "qty": 5.0, "ts": 1717200001 }
```

**Reject** — when an order is invalid or unexecutable:
```json
{ "type": "reject", "trader_id": "my_bot", "order_id": "ord_001", "reason": "insufficient_liquidity", "ts": 1717200001 }
```

#### Input orders (stdin)

```json
{ "trader_id": "my_bot", "order_id": "ord_001", "side": "BUY", "type": "LIMIT", "price": 103.00, "qty": 5.0 }
```

`type` is `"LIMIT"` or `"MARKET"`. For market orders, `price` is ignored.

#### CLI flags

```
--makers N     number of maker agents   (default: 3)
--takers N     number of taker agents   (default: 2)
--whales N     number of whale agents   (default: 1)
--tick-ms N    milliseconds per tick    (default: 500)
```

### 2. Go WebSocket Server (`server/`)

Runs the C++ engine as a managed subprocess and exposes its event stream over HTTP/WebSocket.

#### `engine/runner.go`

Launches `quantsim` as a child process, reads stdout line by line, and forwards events to an internal channel. Auto-restarts on crash with a 2-second delay. Writes incoming orders to the engine's stdin (thread-safe).

#### `ws/hub.go`

Gorilla WebSocket hub. Accepts browser connections, fans out every engine event to all connected clients. Slow clients are evicted rather than blocking the broadcaster. Incoming browser messages are validated and translated to engine order format before being forwarded.

**Browser → server order format:**
```json
{ "type": "order", "trader_id": "...", "order_id": "...", "side": "BUY", "order_type": "LIMIT", "price": 103.00, "qty": 5.0 }
```

(Note: browsers send `order_type`; the server renames it to `type` before writing to the engine.)

#### `db/store.go`

SQLite store with WAL mode. Persists every tick event; trims to the latest 10,000 rows asynchronously. Serves historical data oldest-first via `History(limit)`.

#### REST endpoints

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/api/history?limit=N` | Last N ticks (max 1000, default 200), oldest-first |
| `GET` | `/api/traders` | List all sandbox traders |
| `POST` | `/api/traders` | Spawn a new trader sandbox |
| `GET` | `/api/traders/<id>` | Get sandbox info |
| `GET` | `/api/traders/<id>/log` | Last 100 lines of container output |
| `DELETE` | `/api/traders/<id>` | Stop and remove a sandbox |
| `GET` | `/ws` | WebSocket upgrade — streams all engine events |
| `GET` | `/` | Static files from `server/static/` |

### 3. Sandbox System (`server/sandbox/`)

Each trader script runs in an isolated Docker container with no network access, a read-only filesystem, and resource limits.

#### `sandbox/relay.go`

Unix-socket relay between the engine and sandbox containers. Runs a `fanOut` goroutine that routes:
- **Tick events** → broadcast to all connected sandboxes
- **Fill/reject events** → routed only to the matching `trader_id`

Each container gets its own Unix socket at `/tmp/quantsim-sockets/<trader_id>/quantsim.sock` on the host, mounted as `/tmp/quantsim.sock` inside the container. Rate limiting: >10 orders/second → `reject` with `"reason":"rate_limited"`.

#### `sandbox/manager.go`

Manages the Docker container lifecycle:
- Validates `trader_id` (`^[a-zA-Z0-9_-]{3,32}$`) and script size (≤50 KB)
- Enforces a cap of **5 concurrent sandboxes**
- Writes the trader script to `/tmp/quantsim-scripts/<trader_id>/trader.py` (mode 0444)
- Creates a Docker container with: no network, read-only rootfs, 128 MB RAM, 0.5 CPU, auto-remove
- Monitors container health every 5 seconds; auto-kills after 24 hours
- Cleans up script and socket directories on stop

#### `sandbox-image/`

The Docker image every trader container runs:

```
FROM python:3.12-slim
```

The `quantsim` Python SDK is pre-installed. Trader scripts find the socket path via:

```python
import quantsim
# quantsim.SOCKET_PATH = os.environ["QUANTSIM_SOCKET"]  →  /tmp/quantsim.sock
```

---

## Build

### Prerequisites

- CMake ≥ 3.16
- C++17 compiler (clang++ or g++)
- Go ≥ 1.21
- Docker (running)

### C++ engine

```bash
cmake -S . -B build
cmake --build build
# binary: build/quantsim
```

### Go server

```bash
cd server
go build -o quantsim-server .
```

### Docker sandbox image

```bash
cd server/sandbox-image
docker build -t quantsim-sandbox:latest .
```

---

## Run

### Engine only (testing/debugging)

```bash
./build/quantsim --tick-ms 200 --makers 4 --takers 3
```

Pipe in orders interactively:
```bash
echo '{"trader_id":"me","order_id":"1","side":"BUY","type":"MARKET","qty":10}' | ./build/quantsim
```

### Full server

```bash
cd server
./quantsim-server \
  --engine ../build/quantsim \
  --db ./quantsim.db \
  --port 8080 \
  --tick-ms 500
```

WebSocket clients connect to `ws://localhost:8080/ws` and receive the live event stream.

---

## Trader sandbox API

**Spawn a trader:**
```bash
curl -X POST http://localhost:8080/api/traders \
  -H "Content-Type: application/json" \
  -d '{"trader_id": "my_bot", "script": "import time\nwhile True: time.sleep(1)"}'
# → {"trader_id":"my_bot","status":"spawning"}
```

**List all traders:**
```bash
curl http://localhost:8080/api/traders
```

**Check logs:**
```bash
curl http://localhost:8080/api/traders/my_bot/log
```

**Stop a trader:**
```bash
curl -X DELETE http://localhost:8080/api/traders/my_bot
```

**Error responses** (HTTP 400):
- `invalid_trader_id` — doesn't match `^[a-zA-Z0-9_-]{3,32}$`
- `script_too_large` — exceeds 50 KB
- `too_many_sandboxes` — 5 already running
- `duplicate_trader_id` — already exists

---

## Wire protocol summary

```
engine stdout  →  Go server  →  /ws WebSocket  →  browser
engine stdout  →  Go server  →  Unix socket    →  sandbox container
browser        →  /ws        →  Go server      →  engine stdin
sandbox        →  Unix sock  →  relay          →  engine stdin
```

All messages are newline-delimited JSON in both directions.

---

## Project layout

```
market-sim/
├── src/
│   ├── main.cpp
│   ├── PriceEngine.{h,cpp}
│   ├── OrderBook.{h,cpp}
│   ├── OrderQueue.h
│   ├── Protocol.h
│   ├── PositionTracker.{h,cpp}
│   ├── Simulation.{h,cpp}
│   ├── Trader.{h,cpp}
│   └── agents/
│       ├── MakerTrader.{h,cpp}
│       ├── TakerTrader.{h,cpp}
│       └── WhaleTrader.{h,cpp}
├── server/
│   ├── main.go
│   ├── engine/runner.go
│   ├── ws/hub.go
│   ├── db/store.go
│   ├── api/handlers.go
│   ├── sandbox/
│   │   ├── types.go
│   │   ├── relay.go
│   │   └── manager.go
│   ├── sandbox-image/
│   │   ├── Dockerfile
│   │   └── quantsim_sdk/
│   │       └── quantsim/__init__.py
│   └── static/           ← frontend goes here
├── CMakeLists.txt
└── readme.md
```
