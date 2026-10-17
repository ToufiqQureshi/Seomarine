# dataforseo

The one authenticated transport to DataForSEO. Feature packages build their
own requests and map the results; this package sends them, records what they
cost and makes sure a workspace pays for them.

Flow of `Client.Do(ctx, organizationID, feature, method, path, body, retrySafe)`:

1. Validate the organization, the feature and the path.
2. Ask the spend guard (`Options.Guard`, required) to hold `EstimateUSD(path,
   body)` of the workspace's credits. A refusal stops here: nothing is sent,
   and the error wraps both the guard's error (`spend.ErrNoCredits`,
   `spend.ErrPaused`) and `ErrBillingIssue`.
3. Send the request (only `retrySafe` requests are retried, on 5xx).
4. Record every task's reported cost in `go_dataforseo_usage` and settle the
   hold with their sum for successful tasks. A failed task remains in provider
   spend telemetry but its customer credits are refunded.

`DoUnmetered` is only for free calls that still carry a cost field, such as
collecting a queued task that was charged when it was posted.

Rules that must never break:

- **Every billed request names a feature and goes through `Do`.** A new
  endpoint gets a ceiling in `pricing.go`; until then it is held at one
  dollar per task (`unknownTaskUSD`), so a missing price costs the customer
  a large hold, never Uplumo money.
- **Ceilings are maximums, not averages.** They come from DataForSEO's
  published prices: SERP pages by depth (default depth 10), queued and
  priority rates, the 5x price for search operators, July 2026 Labs and
  Backlinks rates, and endpoint-specific Business Data rates. Labs and
  Backlinks requests without a `limit` are held as 1,000 rows.
  `pricing_test.go` pins them.
- Never send a request the provider would reject. If it still fails, refund
  that task's customer credits while retaining the actual provider cost.
- Never return a provider error body or the API key to a client.
