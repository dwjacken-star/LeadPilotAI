# Facebook authorization-shaped errors and missing views: 2026-11-10

## Verified on the current production account

- Existing Page token is valid and has pages_read_engagement, read_insights, and the expected Page-level grants. Reading the Page and its posts succeeds.
- Published delivery 17 (remote post 1466113573248548_122104256793539408) returns Graph code 10 for object reads and code 100/subcode 33 for comments. Other posts work with the same token. This does not establish whether the post was deleted, hidden, restricted, or otherwise inaccessible.
- Delivery 15 returns an invalid-metric error for post_impressions and post_impressions_unique. The independently requested post_media_view returns 17 at diagnosis time; post_clicks returns a real 0.

## Changes

- Verify Page identity and posts access before isolating an inaccessible post. Continue syncing accessible posts, retain publication receipts, and never republish in response to a statistics error. Actual token and Page permission failures still surface for reconnection.
- Store per-delivery sync state, reason, and attempt time using additive schema changes. Preserve historical counts but mark them unavailable until successfully reread.
- Fetch Facebook post_media_view and post_clicks independently. An unavailable metric cannot discard another valid metric. Missing data is not fabricated as zero.
- Show Facebook views separately from LinkedIn impressions and unique reach in analysis, aggregate cards, and XLSX exports. Account and delivery errors have readable English, Chinese, and Japanese explanations.

## Validation

- Nine backend packages pass; outreachguard compiles with no test files. The social lifecycle regression covers one inaccessible post beside an accessible post, recovery, genuine Page permission errors, expired tokens, independent metric errors, and preservation of publication status.
- 23 frontend tests pass; 2583 translation keys align and message syntax compiles in English, Chinese, and Japanese. ESLint and the production frontend build pass.
- Live read-only sync, release hashes, database/configuration preservation, and HTTPS are checked after deployment. See the accompanying acceptance JSON for current counts and results.
- No new social post or customer message is sent as part of acceptance. No new interactive OAuth consent, Meta App Review, or browser visual acceptance is claimed. The required browser runtime is unavailable.

## Production result

- Alibaba runs `leadpilot/leadpilot-ai:20261110-facebook-sync-v1`; backend SHA-256 `8c0b74b92a0942a984f6d0e4230afab57cec5b80bfcda6c747b779864401ca0d` and frontend index SHA-256 `8db89c30699afdba64ff2389dae6fe453f152ab26c22f201616264b4d4f89c24` match the build.
- Backup: `/opt/BillionMail/backup/20261110-leadpilot-ai-155148`. Deployment uses additive schema updates and preserves existing database contents, credentials, configuration hashes, and other service container identities.
- At 2026-11-10 23:54 Asia/Shanghai, the native sync worker successfully updated deliveries 13, 14, and 15 to 13, 18, and 17 Facebook views respectively (48 total available views). Authenticated reporting and XLSX export passed; the Page remains connected.
- Delivery 17 is still inaccessible through Meta and explicitly marked unavailable with code 10/0. Its publication receipt is preserved; it was not resent. Resolving that remote object's access is separate from statistics synchronization.
- Nine live acceptance checks passed. Evidence: `facebook-statistics-acceptance-20261110.json` and `facebook-statistics-deployment-20261110.json`.
