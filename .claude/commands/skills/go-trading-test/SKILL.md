---
name: go-trading-test
description: Use when writing tests for Go trading code in this repo. Triggers on writing _test.go files in internal/strategy/, internal/risk/, internal/execution/, or when user mentions "test", "coverage", or "table-driven test" in the context of trading logic. Enforces project test standards: table-driven, Decimal not float, deterministic time, no network calls.
---

# Trading Code Test Standards

## Required Patterns

### 1. Table-driven tests
Every test must use table-driven pattern:

\`\`\`go
func TestMomentumDetector(t *testing.T) {
    tests := []struct {
        name      string
        ticks     []PriceTick
        threshold decimal.Decimal
        wantSignal bool
        wantDir   Direction
    }{
        {
            name: "no signal below threshold",
            // ...
        },
        // ... more cases
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            // test logic
        })
    }
}
\`\`\`

### 2. Decimal, never float
\`\`\`go
// ❌ price := 78100.50
// ✅ price := decimal.NewFromString("78100.50")
\`\`\`

### 3. Deterministic time
Never use `time.Now()` directly. Inject a clock:

\`\`\`go
type Clock interface {
    Now() time.Time
}

type Detector struct {
    clock Clock
    // ...
}
\`\`\`

In tests, use a fake clock from `github.com/benbjohnson/clock`.

### 4. No network in unit tests
WebSocket clients tested with `httptest.Server`. Polymarket REST tested with mock HTTP client. No real network calls in `go test ./...`.

### 5. Coverage requirements
- `internal/strategy/`: 100%
- `internal/risk/`: 100%
- `internal/execution/`: 90%
- `internal/feeds/`: 80%

Run: `go test -cover ./internal/...` before declaring test done.

## Anti-patterns to Reject

- `t.Skip()` without comment explaining why
- Tests that depend on test execution order
- `time.Sleep()` in tests (use fake clock instead)
- Asserting on log output (test behavior, not logging)
- Mocking what you don't own (e.g., mocking decimal.Decimal — wrap your own)