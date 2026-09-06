# Contributing to Zyvor Fleet

Thanks for improving the open edge control plane.

## Development

Requirements: Go 1.27+ and Node 20+ (Node is used only for JavaScript syntax validation; the product has no npm runtime dependencies).

```bash
make check
make build
ZYVOR_FLEET_ADMIN_PASSWORD=zyvor-fleet-demo \
ZYVOR_FLEET_SESSION_SECRET=local-demo-session-secret-change-me-1234567890 \
./bin/fleetd --demo
```

Keep changes small, add tests for behavior, run `gofmt`, and avoid adding dependencies for functionality that the Go standard library can safely provide.

## Design rules

- Edge operation must remain useful during WAN loss.
- The agent never accepts arbitrary shell from the server.
- Control-plane desired state is declarative and replayable.
- Enrollment and site identities must not be logged in plaintext.
- The web console must work without CDNs, external fonts, trackers, or third-party JavaScript.
- Nodra owns edge data ingress/twins/routes; Fleet owns site lifecycle and desired state. Do not merge those responsibilities.

By contributing, you agree that your contribution is licensed under Apache-2.0.
