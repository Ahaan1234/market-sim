# market-sim

A 24/7 live market simulation engine. Streams price ticks as newline-delimited JSON to stdout.

## Build

```bash
mkdir build && cd build
cmake .. && make
```

## Run

```bash
./market-sim | head -20
```

## Output format

```json
{"tick":1,"price":100.42,"ts":1746000001}
```

Each tick represents one second. Price moves at most ±1% per tick via a uniform random walk with a floor of 10.0.
