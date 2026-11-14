# Persistent import previews

The XLSX import panel now creates a persistent preview job. Upload/keyword parsing
runs once. Page changes, status filters, progress reads and downloads pass only
the scoped job ID. The previous endpoint remains compatible with older clients.

Rows distinguish queued, checking, valid, invalid, filtered, duplicate and
unconfirmed. The page polls only while queued/checking rows remain. Catch-all,
SMTP policy blocks, transient DNS/SMTP failures and timeouts remain unconfirmed,
downloadable and excluded from contact groups. Unknown SMTP results normally
back off for five minutes; SMTP policy failures for fifteen minutes; catch-all
for six hours. A manual retry can enqueue only unconfirmed rows whose backoff
has elapsed. No automatic recurring retry occurs for completed rows.

Eight shared workers claim rows with durable leases. A last-dispatch timestamp
shares capacity between imports. Interrupted first attempts resume; a second
interruption stops for manual review. A single catch-all response ends that
SMTP probe without redundantly testing additional IPs. Validation issues no DATA.

The additive, idempotent governance migration creates bm_contact_import_jobs and
bm_contact_import_rows, without deleting or replacing existing contact/mail data.
Jobs are accessible only to their originating account and department, with fresh
import permission and sender authorization checks. They are available for 24
hours. Expired jobs are cleaned after an additional day; immutable approved
snapshots retain the existing 30-minute confirmation window.

Only SMTP-confirmed (or authoritative local mailbox) contacts enter an approval
snapshot. Later background progress cannot expand an already approved import.
Verification remains in the import stage and is not added to the sending path.

After deployment, refresh an already-open administration page to load the new
panel. Verify authenticated first upload, identifier-only polling and pagination,
terminal uncertain states, XLSX downloads, access isolation, valid-only import,
job recovery and preservation of existing accounts/campaigns/mailboxes.
