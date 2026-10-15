# Spend guard test helper

Provides a deterministic in-memory `spend.Guard` for tests of code that issues billed provider requests. This is test support, not a production spend guard.

Read [flow.md](flow.md) for how tests record holds/settlements and [decision.md](decision.md) for constraints on using this helper safely.

## Source map

- `spendtest.go`
