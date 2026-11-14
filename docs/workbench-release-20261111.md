# LeadPilot AI unified workspaces — 2026-11-11

## Delivered

- A persistent administration shell with closable, scrollable workspace tabs, a pinned departmental home, bulk close/refresh actions, permission-filtered restoration and account-scoped session metadata.
- Per-pane route injection and editor stores prevent different record IDs and email drafts from sharing state. Hidden panes pause polling and hide teleported dialogs without discarding their desired state. Requests have independent cancellation controllers and retain their original pane ownership.
- A Roundcube workbench extending the installed 1.6.11 skin/plugin interfaces. Mail, reading, writing, contacts and settings use independent native document contexts. Each compose retains its native compose ID, attachments and IMAP draft semantics. Saved draft delivery preferences persist by mailbox and draft key.
- Shared teal/navy design tokens, wordmarks, quiet surfaces, light/dark mode, narrow-screen layouts, scrolling tab controls, dirty-close protection, server draft save-before-close, and separate read-only original HTML in replies.
- The composer includes a final-content preview with desktop/mobile widths, templates and AI draft suggestions. Preview uses the delivery preparation pipeline, without consuming warmup quota or requesting the generated open-tracking pixel. Suggestions require explicit application to the editor.
- A customer panel reuses CRM data and follow-ups. Explicit administrator-managed mailbox/account bindings reuse existing role and department checks on every request. No administrator credential or provider key is sent to the mailbox browser. Unlinked mailboxes retain ordinary mail functions.
- Actual From/Reply-To remains visible. Submit, error and uncertain-send states are distinct; uncertain transport results require checking before re-enabling send. Existing subscription suppression and delivery controls remain authoritative.

## Sending behavior

This release changes the workspace interfaces and their supporting APIs. Existing immediate delivery behavior remains in use. Warmup rejection leaves the editor intact; it does not claim that a deferred task was created. Personalized tracking/subscription links require one recipient in the ordinary mailbox; independent multi-recipient deliveries continue to use management campaigns. No unsupported scheduling or automatic delivery guarantee is presented in the new interface.

## Validation and release

- Real Vue components are tested using Vue's custom renderer with memory history, including retained forms, isolated route parameters, isolated editor stores, close/refresh disposal and account-switch cleanup. This is component execution, not browser visual testing.
- Existing 23 frontend regression tests pass. The new workbench suite has 3 passing tests. 2603 locale keys align across English, Chinese and Japanese; the production build passes.
- Backend PostgreSQL integration includes explicit-link authorization, permission revocation, cross-department denial and per-draft/per-mailbox preference isolation, alongside the existing delivery and opt-out integration. PHP and JavaScript syntax checks pass.
- Deployment preserves the existing PostgreSQL database, mailbox storage, credentials, provider/social configuration and other services. Schema changes add mailbox binding and draft-option tables only.
- The required browser runtime is unavailable in this session. Browser visual interaction, real attachment/draft UI operation and new received-message acceptance are not claimed. Production HTTP render checks, native compose-context checks and authenticated API checks are recorded separately after deployment. No customer email or public social post is sent by acceptance.

## Production acceptance

- Alibaba runs `leadpilot/leadpilot-ai:20261111-workbench-v2` with backend SHA-256 `495d3d319de986e5c772580dc81ba15798512dc624444c6a1fa55528046e23a3` and frontend index SHA-256 `787c442b21469dc78206c390e2e26ff5c392a1b1ed9abe24d02d132f9bb37a4b`. Public plugin assets and feature bundle hashes match the package.
- Latest rollback backup: `/opt/BillionMail/backup/20261111-workbench-172232`. The original pre-workspace release is also retained in `/opt/BillionMail/backup/20261111-workbench-170548`.
- Seven live acceptance checks pass: preserved business data/credentials/configuration/other service identities, safe administrator binding options, authenticated Roundcube workbench rendering, two distinct native compose IDs with effective sender controls, public HTTPS assets, independent authenticated draft preferences, and no customer send/public post by acceptance.
- Mailbox business features are opt-in via Mailboxes → Mailbox business access. Binding is not automatically assigned to an administrator; no production business binding was changed during acceptance.
- Evidence: `workbench-acceptance-20261111.json` and `workbench-deployment-20261111.json`. Browser visual checks and actual received-message checks remain unperformed.
