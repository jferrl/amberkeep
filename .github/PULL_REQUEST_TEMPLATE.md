<!-- What was learned, not what was typed: the bug found, the measurement that
     settled an argument, the thing that turned out to be wrong. -->

## What this changes

## Why

## Checks

- [ ] `gofmt -w . && go build ./... && go test ./... -race -count=1 && golangci-lint run ./...`
- [ ] If anything under `web/` changed: `cd web && npm run typecheck && npm run lint && npm test && npm run doctor && npm run build`, and `internal/viewer/dist` rebuilt and staged
- [ ] If a handler's response changed: recordings re-made with `go test ./internal/api -run TestTheRecordedRepliesStillMatch -update`, and `web/src/api/types.ts` updated in the same commit
- [ ] Tests added or extended, and any regression test checked against the unfixed code
- [ ] An ADR in `docs/adr/` if a decision was made
- [ ] Signed off: `git commit -s`

<!-- No real message content, database, backup or key is in this change. -->
