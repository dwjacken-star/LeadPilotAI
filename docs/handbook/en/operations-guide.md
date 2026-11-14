# Deployment, Maintenance and Troubleshooting

**LeadPilot AI | English Operations Guide | Version 1.0 | 2026-11-14**

These are procedures, not evidence that production deployment, backup or recovery was performed in this task. Examples run from the project root and retain actual Compose service identifiers. Production changes require a verified target, backup, maintenance window and rollback owner.

## 1. Published files and private dependencies

GitHub contains source, documentation, fictional-data screenshots and the explicitly requested PDFs. Normal application packages, archives, build output, runtime data, SQL, private keys and credentials remain ignored. The PDFs are authorized documentation deliverables, not application installers.

**A public clone alone cannot build the backend.** Source embeds ignored private SQL at:

- `core/internal/service/governance/`: `migration.sql`, `growth.sql`, `delivery.sql`, `statistics.sql`, `social.sql`, `master.sql`.
- `core/internal/testsupport/`: `base.sql`.

Initialization inputs, environment, certificates and existing deployment settings are privately managed. Supply files matching the selected source version at their original paths. Do not disguise SQL under another extension. A source archive is not a complete runnable backup.

## 2. Environment and configuration

Start in an isolated Linux/Docker environment with Docker Engine/Compose, Node.js 22, pnpm compatible with the lockfile, and Go 1.23.2. A native Windows Go build does not establish a functioning Linux mail stack.

1. Clone and verify the source:

```sh
git clone https://github.com/dwjacken-star/LeadPilotAI.git
cd LeadPilotAI
git log -1 --oneline
```

2. Supply private SQL/configuration and verify each embedded dependency.
3. Copy `.env.example` to `.env` and privately fill administrator/database/Redis values. Do not overwrite an existing production environment.
4. Configure actual hostname, SafePath, HTTPS, domains and controlled outbound mail.
5. Restrict file permissions for environment, private certificates, master key and backups.
6. Verify local-only database/Redis bindings and the privileged Docker-socket trust boundary.

Empty-password mailbox requests use private `DEFAULT_NEW_MAILBOX_PASSWORD` only when configured. Otherwise they fail. Never restore a hardcoded shared source password.

## 3. Build and validation

### 3.1 Frontend

```sh
cd core/frontend
pnpm install --frozen-lockfile
pnpm build
pnpm i18n:check
pnpm test:workspace
pnpm test:repair
cd ../..
```

Output is `core/frontend/dist`; the Dockerfile copies it to Core's public resources. `build:git` contains Git/asset-copy behavior and `build:deploy` invokes deployment code. For build-only validation use `build`, not a deployment shortcut.

### 3.2 Backend

Provide the private SQL first, then explicitly build the production entry point:

```sh
cd core
go mod download
go build -mod=readonly -trimpath -o ../.tools/leadpilot-core main.go
go test ./internal/service/mail_boxes ./internal/controller/mail_boxes ./internal/controller/settings
cd ..
```

Development entry files exist; do not compile multiple main files inadvertently with `go build .`. Database integration tests create/delete disposable databases when `BM_TEST_PG_HOST` and related variables are supplied. Use only an **isolated test instance** with appropriate privileges, never production. Skipped tests are not full verification.

### 3.3 Publication safeguards

```sh
node --test scripts/check-publish.test.cjs
git ls-files -ci --exclude-standard
node scripts/check-publish.cjs
```

The checker examines staged blobs and is not a comprehensive security audit. Simple hardcoded strings, customer privacy and screenshot content still need human review.

## 4. Compose deployment

Default Compose references an upstream Core image. Customized code uses the project override:

```sh
docker compose -f docker-compose.yml -f docker-compose.enterprise.yml config --quiet
docker compose -f docker-compose.yml -f docker-compose.enterprise.yml build core-billionmail
```

After approval, a complete backup and matching private script verification, deploy/update in the agreed window:

```sh
docker compose -f docker-compose.yml -f docker-compose.enterprise.yml up -d
```

These commands were not executed by this documentation task. **An upgrade is not a fresh installation:** preserve environment, database, mail storage, Postfix queue, materials, master key, old image and grants. Do not run `down -v` or replace the customized Core using an upstream update shortcut.

Avoid simultaneous old/new send schedulers. Switch Core under the change plan and verify migration/leases. Adding replicas alone is not high availability.

## 5. Health checks and release acceptance

Read-only examples:

```sh
docker compose ps
docker compose logs --tail=120 core-billionmail
docker compose logs --tail=80 pgsql-billionmail
docker compose logs --tail=80 postfix-billionmail
docker compose logs --tail=80 dovecot-billionmail
docker compose logs --tail=80 rspamd-billionmail
```

Logs can contain internal email addresses, paths or request details. Redact before sharing; do not upload raw logs to GitHub.

Minimum acceptance:

- Matching Core/frontend version, functional login/language/static resources.
- Successful migrations without resetting historical data or credentials.
- Ordinary-member scope, private downloads and sender grants.
- Controlled final delivery, inbox access and replies using owned mailboxes.
- Unsubscribe, pause/cancel, unknown reconciliation and sending windows.
- Minimal real AI/search capability and budget tests.
- Separate approved-application social acceptance; missing credentials do not justify claims of authorization/publishing success.

## 6. Backup strategy

Include PostgreSQL, private environment/configuration, SSL keys/certificates, `core-data` including materials and `.ai-master-key`, mailbox data, applicable queues, previous image and version metadata.

**The example below is a database logical backup only, not a complete system backup:**

```sh
umask 077
BACKUP_DIR="backup/manual-$(date +%Y%m%d-%H%M%S)"
mkdir -p "$BACKUP_DIR"
docker compose exec -T pgsql-billionmail sh -c \
  'exec pg_dump -Fc -U "$POSTGRES_USER" "$POSTGRES_DB"' \
  > "$BACKUP_DIR/database.dump"
test -s "$BACKUP_DIR/database.dump"
docker compose images > "$BACKUP_DIR/images.txt"
git rev-parse HEAD > "$BACKUP_DIR/source-version.txt"
```

File/mail/queue backups must use the actual persistent paths and a consistency strategy for writes. Use an approved maintenance window or consistent snapshot; an arbitrary copy of changing directories is not a complete consistent backup.

- Encrypt or restrict backups; database copies and keys must never become public.
- Keep multiple versions and periodically restore in isolation.
- Record the matching master-key version; never generate a new key over the old one during recovery.
- Confirm integrity and real restorability, not just a successful command exit.

## 7. Upgrade and rollback

Upgrade sequence: identify version → complete backup → stop old Core from new handoffs → update matching frontend/backend/private SQL → check additive migration → verify scope/data/queue → controlled test delivery → resume business.

Pause external tasks before rollback and record ambiguous submissions. Restore application and matching data/configuration using a rehearsed plan. Rolling back only the application while retaining new department accounts may expose old code without modern access boundaries; control accounts/API rights first. Do not automatically drop databases or perform destructive restore without explicit approval.

Stopping/rolling back does not recall queued mail or external posts. Reconcile unknown records separately to avoid duplicate sending after recovery.

## 8. Troubleshooting paths

| Problem | Diagnostic order | Principle |
|---|---|---|
| Startup failure | Private SQL/config → DB connection → migration → Docker socket → logs | Keep evidence; do not bypass failed migration |
| Login/access | HTTPS/SafePath → account → token → department/roles → legacy boundary | Do not grant everything to make an error disappear |
| Blank frontend | Entry/chunk consistency → browser error → API envelope → cache/version | Preserve reproducible evidence; do not call it a login success |
| Mail stalled | State/grants → higher limits → window → SMTP/queue → bounce → network policy | Separate accepted and delivered |
| AI/search failure | Protocol/model → endpoint → private secret → budget → parsing/evidence | No fabricated response or unlimited retry |
| Social blocked | Approved app → account mode → grant/token → reviewed content → 429/unknown | Apply platform backoff; reconcile unknown |
| Decryption after restore | Matching DB/master key, storage path and permissions | Do not overwrite or disclose the key |
| Disk pressure | Data/log/queue/upload use and retention | Do not delete real mailboxes/queues as an emergency shortcut |

## 9. Daily and periodic operations

- Daily: service/disk state, unknown/pending approvals, mail queue/bounce, budgets, expired grants and backup.
- Weekly: departmental rights, unsubscribe/suppression, model settings, platform coverage and recurring errors.
- Monthly/as required: restore drill, master-key pairing, certificate renewal, dependency security and platform permission/retention review.
- Every release: record version, scope, approval, backup, rollback, migration and controlled acceptance results.

## 10. Rebuilding documentation

With Playwright Core, a supported browser, frontend dependencies and a local frontend build available:

```sh
node scripts/docs/generate-diagrams.cjs
node scripts/docs/capture-ui.mjs
node scripts/docs/build-handbook.cjs
python scripts/docs/validate-handbook.py
```

`PLAYWRIGHT_MODULE` selects the Playwright Core module file; `DOC_BROWSER` selects a local Chromium/Edge executable. Default tool paths describe the generation environment; configure them explicitly on another computer and never commit browser/dependency directories.

Captures intercept fictional APIs and restrict requests to local resources; they do not access production. Markdown and diagram sources remain independently readable without PDF tooling.

Validate links, images, PDFs and sensitive information before publishing. Verify screenshots contain no real customers, staff, tokens, private keys, private email content or financial records.
