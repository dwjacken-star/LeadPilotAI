#!/bin/bash
set -euo pipefail
exec 9>/run/lock/billionmail-ip-cert.lock
flock -n 9 || exit 0
cd /opt/BillionMail
test "$(docker inspect -f '{{.State.Running}}' billionmail-core-billionmail-1)" = true
docker run --rm --name billionmail-certbot-renew --network container:billionmail-core-billionmail-1 \
 -v /etc/letsencrypt:/etc/letsencrypt -v /var/lib/letsencrypt:/var/lib/letsencrypt -v /var/log/letsencrypt:/var/log/letsencrypt \
 certbot/certbot:v5.4.0 renew --cert-name billionmail-console-ip --non-interactive --no-random-sleep-on-renew --allow-subset-of-names
cert=/etc/letsencrypt/live/billionmail-console-ip/fullchain.pem
key=/etc/letsencrypt/live/billionmail-console-ip/privkey.pem
openssl x509 -in "$cert" -noout -checkend 86400
# Keep configured browser domains valid when an optional legacy IP is dropped.
python3 - "$cert" <<'PY'
import pathlib,ssl,sys,urllib.parse
env=dict(line.split('=',1) for line in pathlib.Path('.env').read_text().splitlines() if '=' in line and not line.startswith('#'))
certificate=ssl._ssl._test_decode_cert(sys.argv[1])
hosts=[env.get('BILLIONMAIL_HOSTNAME',''),urllib.parse.urlparse(env.get('PUBLIC_BASE_URL','')).hostname]
for host in hosts:
    if host and not host.endswith('.invalid') and host!='mail.example.com':
        ssl.match_hostname(certificate,host)
PY
if ! cmp -s "$cert" ssl/cert.pem || ! cmp -s "$key" ssl/key.pem; then
 install -m 644 "$cert" ssl/cert.pem
 install -m 600 "$key" ssl/key.pem
 ./deploy.sh restart core-billionmail
fi
