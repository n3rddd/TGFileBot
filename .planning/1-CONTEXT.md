# Phase 1: Debug Playback - Context

## Decisions

### 1. Root Cause Analysis
- **Symptom**: Player cancels request after waiting ~20 seconds without receiving data.
- **Cause**: On cold start, the MTProto connection is likely dead (TCP timeout/drop). The `DownloadChunk` call in `stream.go` has a hardcoded 90-second timeout. The underlying client hangs trying to read/write to the dead socket. Before the 90s timeout hits, the user's video player times out and closes the HTTP connection (`r.Context().Done()` fires), aborting the stream. 

### 2. Implementation Strategy (Option B: Fast Fail & Retry)
- **Action**: Reduce the `DownloadChunk` context timeout significantly (e.g., to 5-10 seconds) during the initial fetch.
- **Retry Logic**: If the chunk download times out or fails quickly due to network error, retry with backoff.
- **MTProto Reconnection**: Investigate if the `gogram` client can be forced to reconnect or if the fast-fail will naturally trigger a fresh connection on the next retry.

## Canonical References
- `f:\文档\Go\TGBot\stream.go`
- `f:\文档\Go\TGBot\http.go`

## Code Context
- `stream.go:191` - `stream.Client.DownloadChunk(..., 90*time.Second)`
- `http.go:241` - `case <-r.Context().Done():` handles player cancellation.

## Deferred Ideas
- Active Keep-Alive/Ping mechanism (rejected in favor of fast-fail).
