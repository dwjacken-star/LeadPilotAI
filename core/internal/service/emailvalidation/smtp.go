package emailvalidation

import (
	"context"
	"crypto/rand"
	"crypto/tls"
	"encoding/hex"
	"net"
	"net/smtp"
	"net/textproto"
	"strings"
	"sync"
	"time"
)

var probeMu sync.Mutex
var nextProbe = map[string]time.Time{}
var dialSMTP = func(ctx context.Context, address string) (net.Conn, error) {
	return (&net.Dialer{Timeout: 3 * time.Second}).DialContext(ctx, "tcp", net.JoinHostPort(address, "25"))
}

func throttle(ctx context.Context, host string) bool {
	probeMu.Lock()
	now := time.Now()
	next := nextProbe[host]
	if next.Before(now) {
		next = now
	}
	nextProbe[host] = next.Add(time.Second)
	if len(nextProbe) > 20000 {
		nextProbe = map[string]time.Time{}
	}
	probeMu.Unlock()
	timer := time.NewTimer(time.Until(next))
	defer timer.Stop()
	select {
	case <-timer.C:
		return true
	case <-ctx.Done():
		return false
	}
}
func rcpt(c *smtp.Client, email string) (int, string, error) {
	id, err := c.Text.Cmd("RCPT TO:<%s>", email)
	if err != nil {
		return 0, "", err
	}
	c.Text.StartResponse(id)
	defer c.Text.EndResponse(id)
	return c.Text.ReadResponse(0)
}
func classifyRCPT(code int, message string) Result {
	if code == 250 || code == 251 {
		return result(Valid, "smtp_accepted", "smtp")
	}
	// Policy rejections, quotas, greylisting and full mailboxes do not establish
	// non-existence. Only explicit enhanced permanent recipient failures do.
	text := strings.TrimSpace(message)
	if code >= 500 && (strings.HasPrefix(text, "5.1.1") || strings.HasPrefix(text, "5.1.3") || strings.HasPrefix(text, "5.1.10")) {
		return result(Invalid, "recipient_not_found", "smtp")
	}
	return result(Unknown, "smtp_unconfirmed", "smtp")
}
func probeSMTP(ctx context.Context, dns Result, email, sender string) Result {
	_, domain, _ := Normalize(email)
	_, identity, _ := Normalize(sender)
	fallback := result(Unknown, "smtp_unreachable", "smtp")
	attempts := 0
	for _, route := range dns.Routes {
		if attempts >= 3 {
			break
		}
		if !throttle(ctx, route.Host) {
			return result(Unknown, "smtp_timeout", "smtp")
		}
		for _, address := range route.IPs {
			if attempts >= 3 {
				break
			}
			attempts++
			// Dial the validated IP directly to prevent DNS rebinding to private networks.
			ip := net.ParseIP(address)
			if ip == nil || !publicIP(ip) {
				continue
			}
			conn, err := dialSMTP(ctx, address)
			if err != nil {
				continue
			}
			deadline, _ := ctx.Deadline()
			_ = conn.SetDeadline(deadline)
			c, err := smtp.NewClient(conn, route.Host)
			if err != nil {
				conn.Close()
				continue
			}
			if err = c.Hello(identity); err == nil {
				if okay, _ := c.Extension("STARTTLS"); okay {
					err = c.StartTLS(&tls.Config{ServerName: route.Host, MinVersion: tls.VersionTLS12})
				}
			}
			if err == nil {
				err = c.Mail(sender)
			}
			if err != nil {
				c.Close()
				fallback = result(Unknown, "smtp_policy", "smtp")
				continue
			}
			code, message, err := rcpt(c, email)
			if err != nil {
				c.Close()
				continue
			}
			r := classifyRCPT(code, message)
			if r.Status == Valid {
				// A random, unallocated address detects catch-all/anti-enumeration servers.
				// Never issue DATA and never submit a message during verification.
				var random [16]byte
				_, err = rand.Read(random[:])
				if err != nil {
					c.Close()
					return result(Unknown, "smtp_unconfirmed", "smtp")
				}
				err = c.Reset()
				if err == nil {
					err = c.Mail(sender)
				}
				if err == nil {
					otherCode, otherMessage, otherErr := rcpt(c, "maplemail-probe-"+hex.EncodeToString(random[:])+"@"+domain)
					other := classifyRCPT(otherCode, otherMessage)
					if otherErr != nil {
						r = result(Unknown, "smtp_unconfirmed", "smtp")
					} else if otherCode == 250 || otherCode == 251 {
						r = result(Unknown, "catch_all", "smtp")
					} else if other.Status != Invalid {
						r = result(Unknown, "smtp_unconfirmed", "smtp")
					}
				} else {
					r = result(Unknown, "smtp_unconfirmed", "smtp")
				}
			}
			_ = c.Reset()
			_ = c.Quit()
			c.Close()
			if r.Code == "catch_all" {
				return r
			}
			if r.Status != Unknown {
				return r
			}
			fallback = r
		}
	}
	return fallback
}
func IsRecipientError(err error) bool { _, ok := err.(*textproto.Error); return ok }
