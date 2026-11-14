#!/bin/bash
set -euo pipefail
exec 9>/run/lock/maplemail-bestmaple-cert.lock
flock -n 9 || exit 0
cd /opt/BillionMail
pid=$(docker inspect -f '{{.State.Pid}}' billionmail-core-billionmail-1)
nsenter -t "$pid" -n /opt/billionmail-certbot/bin/certbot renew --cert-name maplemail-bestmaple --standalone --http-01-port 60880 --non-interactive --no-random-sleep-on-renew
cert=/etc/letsencrypt/live/maplemail-bestmaple/fullchain.pem
key=/etc/letsencrypt/live/maplemail-bestmaple/privkey.pem
openssl x509 -in "$cert" -noout -checkend 86400
if ! cmp -s "$cert" ssl/cert.pem || ! cmp -s "$key" ssl/key.pem; then
 install -d -m 755 ssl/mail.best-maple.com
 install -m 644 "$cert" ssl/cert.pem
 install -m 644 "$cert" ssl/certificate.pem
 install -m 644 "$cert" ssl/mail.best-maple.com/fullchain.pem
 install -m 600 "$key" ssl/key.pem
 install -m 600 "$key" ssl/mail.best-maple.com/privkey.pem
 docker exec billionmail-postfix-billionmail-1 postmap -F hash:/etc/postfix/conf/vmail_ssl.map
 python3 - <<'PY'
import subprocess,time
base=['docker','exec','-i','billionmail-pgsql-billionmail-1','psql','-U','billionmail','-d','billionmail','-X','-qAt','-v','ON_ERROR_STOP=1']
lock=subprocess.Popen(base,stdin=subprocess.PIPE,stdout=subprocess.PIPE,stderr=subprocess.PIPE,text=True,bufsize=1)
try:
 lock.stdin.write("BEGIN; SELECT pg_advisory_xact_lock(614806);\n\\echo FROZEN\n");lock.stdin.flush()
 while 'FROZEN' not in lock.stdout.readline():
  if lock.poll() is not None:raise RuntimeError('Cannot freeze sender claims')
 for _ in range(40):
  count=subprocess.check_output(base+['-c',"SELECT (SELECT count(*) FROM recipient_info WHERE delivery_state IN('claimed','sending'))+(SELECT count(*) FROM bm_growth_forwards WHERE status='sending')+(SELECT count(*) FROM bm_crm_messages WHERE status='sending')"]).decode().strip()
  if int(count)==0:break
  time.sleep(1)
 else:raise RuntimeError('Active sends did not drain; defer certificate reload')
 subprocess.check_call(['./deploy.sh','restart','core-billionmail','postfix-billionmail','dovecot-billionmail'])
finally:
 if lock.poll() is None:
  try:lock.stdin.write('ROLLBACK;\n\\q\n');lock.stdin.flush()
  except BrokenPipeError:pass
 lock.communicate(timeout=10)
PY
fi
