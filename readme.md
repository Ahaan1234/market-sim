# market-sim

A 24/7 live market simulation engine. Streams price ticks as newline-delimited JSON to stdout.

---

## What this project is, and how it works

### The big picture

This project simulates a financial market — specifically, the moment-to-moment price of a single asset (think of it like a stock). It doesn't connect to any real exchange. Instead, it generates a stream of synthetic prices using mathematics, printing one price per second, forever. The goal is to produce output that *looks and behaves* like a real market feed, so that a dashboard, chart, or server can consume it without knowing — or caring — whether the prices are real.

---

### How prices move: the random walk

At the heart of the simulation is a concept called a **random walk**. The idea is simple:

> The next price is the current price, shifted by a small random amount.

More precisely, at each step the program draws a random number `r` uniformly from the range **[-0.01, +0.01]** — meaning any value between -1% and +1% is equally likely. The new price is then:

```
new price = current price × (1 + r)
```

For example, if the price is $100.00 and `r` happens to be +0.006:

```
new price = 100.00 × 1.006 = $100.60
```

This is called a **multiplicative** random walk. The percentage move is random and bounded, but the dollar move scales with the price — a $10 asset and a $1000 asset both move at most 1% per tick, not the same fixed dollar amount. This matches how real asset prices behave.

#### Why multiplicative, not additive?

An additive walk would do `new price = current price + r`, where `r` is a fixed dollar amount. The problem is that prices would eventually drift negative. Multiplying instead of adding keeps prices strictly positive (they can approach zero but never cross it), which is mathematically consistent with how stocks, commodities, and currencies are modelled.

#### The floor

To prevent the price from collapsing toward zero in a long run of bad luck, a hard floor of **$10.00** is enforced. If a tick would push the price below that level, it is clamped to $10.00.

---

### Randomness and reproducibility

The random numbers come from a **Mersenne Twister** — a widely used algorithm that produces a long sequence of pseudo-random numbers from a starting value called a **seed**. The seed is fixed (`42`), which means:

- Every run of the program produces *exactly the same sequence of prices*.
- This is intentional: reproducibility lets you test and debug downstream systems (charts, servers) without the data changing under you.

To get a different price path, you would change the seed.

---

### The output format

Each tick is printed as a single line of **JSON** — a lightweight, human-readable data format that nearly every programming language and tool can parse:

```json
{"tick":1,"price":100.59,"ts":1778064686}
```

| Field  | Meaning |
|--------|---------|
| `tick` | How many steps have elapsed since the simulation started |
| `price`| The current simulated asset price in dollars |
| `ts`   | A Unix timestamp — the number of seconds since 1 January 1970, incremented by 1 per tick |

The timestamp is synthetic (not real wall-clock time), but it increases by exactly one second per tick. This gives downstream consumers — a charting library, a database, a WebSocket server — a consistent time axis to plot or index against.

---

### What has been built so far

| Component | What it does |
|-----------|-------------|
| `PriceEngine` | Holds the current price and the random number generator. Exposes a single `nextPrice()` call that advances one tick and returns the new price. |
| `main.cpp` | Calls `nextPrice()` 1000 times in a loop, formats each result as JSON with a synthetic timestamp, and prints it to the terminal. |
| `CMakeLists.txt` | Build instructions. Tells the compiler to use C++17 and to flag any code quality warnings. |

---

### What comes next

The 1000-tick JSON stream printed to the terminal is the **contract** between this engine and the rest of the system. Future sessions will build:

- A **Go server** that runs the engine as a subprocess and forwards its output over **WebSockets** to a browser in real time.
- A **browser dashboard** with live candlestick charts, order book depth, and market statistics — all fed by this stream.

---

## Build

```bash
mkdir -p build && cd build
cmake .. && make
```

## Run

```bash
./market-sim          # all 1000 ticks
./market-sim | less   # page through (press q to exit)
./market-sim | head -20
```
