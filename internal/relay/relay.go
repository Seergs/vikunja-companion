// Package relay holds both sides of the content-blind APNs relay
// (docs/COMPANION.md §6.6):
//
//   - the client the companion uses to register once and push ciphertext
//     ({apns_token, ciphertext, collapse_id});
//   - the server (cmd/relay) that forwards to APNs with the .p8 key and stores
//     only opaque registration tokens — no PII, no content.
package relay

// TODO(fase-2): Client{baseURL, token}: Register(ctx); Push(ctx, req).
// TODO(fase-2): Server: POST /relay/v1/register, POST /relay/v1/push (per-token
// rate limited), APNs forwarder.
