#!/bin/bash
set -euo pipefail
exec 9>/run/lock/billionmail-ip-cert.lock
flock -n 9 || exit 0
cd /opt/BillionMail
core_pid=$(docker inspect -f '{{.State.Pid}}' billionmail-core-billionmail-1)
test "$core_pid" -gt 0
nsenter -t "$core_pid" -n /opt/billionmail-certbot/bin/certbot renew --cert-name billionmail-console-ip --non-interactive --no-random-sleep-on-renew
cert=/etc/letsencrypt/live/billionmail-console-ip/fullchain.pem
key=/etc/letsencrypt/live/billionmail-console-ip/privkey.pem
openssl x509 -in "$cert" -noout -checkend 86400
if ! cmp -s "$cert" ssl/cert.pem || ! cmp -s "$key" ssl/key.pem; then
  install -m 644 "$cert" ssl/cert.pem
  install -m 600 "$key" ssl/key.pem
  ./deploy.sh restart core-billionmail
fi
