# Alibaba dual-domain deployment

- Active sender domains: `best-maple.com`, `autoretsworld.com`.
- The management console's default sender domain controls Postfix `myorigin`,
  the fallback outbound `smtp_helo_name`, and the initial authorized mailbox for
  new marketing tasks, APIs, schedules and automations.
- Each direct sender-domain transport uses `mail.<sender-domain>` as its HELO.
  Existing relay and dedicated-IP mappings retain precedence. Explicit mailbox
  selection, sender rotation and message From/envelope addresses are preserved.
- The server hostname and HTTPS/public mail endpoint remain independent from the
  selected default sender domain. No database migration or mailbox rewrite is
  needed. Postfix configuration persists in the existing host-mounted files and
  is synchronized on application startup, relay synchronization and default changes.
- Managed file: `conf/postfix/conf/maplemail_sender_domains`, compiled by postmap.
  The corresponding master.cf services are enclosed in the `MAPLEMAIL SENDER
  DOMAINS` markers. The fallback hash map is appended after existing maps.
- A failed configuration check, reload or option publication restores the old
  files and reloads the old configuration. Check application logs for sync errors.
- Huawei stack was stopped with all seven restart policies set to `no`.
  Its database, mailbox volumes and shutdown backup remain on that server.
  To restore service deliberately, inspect the shutdown backup before enabling
  restart policies and using the server's existing `deploy.sh up -d`.

Release verification must include real authenticated default changes in both
directions, lookup results for both domains, Postfix check/reload, preserved
credentials/data and HTTPS asset availability. SMTP acceptance/configuration
verification does not establish delivery to an external inbox.
