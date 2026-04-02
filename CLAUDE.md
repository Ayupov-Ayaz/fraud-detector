# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Commands

```bash
# Run directly
go run main.go

# Compile and run (better performance for large datasets)
go build -o fraud-detector main.go
./fraud-detector
```

No tests exist. No linter is configured beyond `go vet`.

## Architecture

Single-file Go application (`main.go`) with zero external dependencies. The tool reads all `*.json` files from the current directory and outputs a fraud analysis report to stdout.

### Data Pipeline

```
main() → run()
  findJSONFiles()          → discovers all *.json in cwd
  readMultipleLogFiles()   → loads raw []LogEntry arrays
  parseGameData()          → unmarshals the embedded "line" JSON field into []GameData
  generateReport()         → aggregates stats, detects fraud, returns Report
  printReport()            → formats and prints to terminal
```

### Key Types

- `LogEntry` — raw Loki log entry; the `Line` field contains a JSON-encoded `GameData`
- `GameData` — parsed transaction (bet/win amounts, player_id, game_id, timestamps)
- `Report` — complete analysis result (player stats, game stats, hourly breakdown, suspicious events)
- `PlayerStat` / `GameStat` / `TimeStat` — aggregated metrics per dimension

### Fraud Detection Logic (`generateReport`)

- **Duplicate detection**: tracks seen `betID` and `winID` values in maps; duplicates are skipped and counted
- **High RTP alert**: flags players with RTP > 150% AND > 100 bets as suspicious events

### Currency

Amounts are stored as `int64` in minor units (e.g., kobo for NGN). Currency symbol is detected dynamically from log data and threaded through formatting functions. Currently optimized for NGN.

## Input Format

JSON files must contain arrays of Loki log entries. The `line` field holds a JSON string with the actual game event:

```json
[
  {
    "line": "{\"level\":\"info\",\"ts\":1766833263.671,\"msg\":\"SendBet\",\"player_id\":\"123\",\"bet\":100000,\"bet_id\":\"abc\",\"balance\":950000}",
    "timestamp": "2025-12-25T22:57:43.671Z",
    "fields": {}
  }
]
```

## Loki Export Constraint

Loki caps downloads at **1000 logs per export**. When a file contains exactly 1000 entries, the tool warns about possible truncation. Work around this by exporting smaller time windows and placing multiple JSON files in the directory — the tool merges them automatically.
