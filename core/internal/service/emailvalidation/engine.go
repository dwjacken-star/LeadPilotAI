package emailvalidation

import (
	"context"
	"encoding/json"
	"net"
	"net/mail"
	"sort"
	"strings"
	"sync"
	"time"

	"golang.org/x/net/idna"
	"golang.org/x/sync/singleflight"
)

const (
	Valid   = "valid"
	Invalid = "invalid"
	Unknown = "unknown"
)

type Route struct {
	Host string   `json:"host"`
	IPs  []string `json:"ips"`
}
type Result struct {
	Status    string    `json:"status"`
	Code      string    `json:"code"`
	Stage     string    `json:"stage"`
	Routes    []Route   `json:"routes,omitempty"`
	CheckedAt time.Time `json:"checked_at"`
	ExpiresAt time.Time `json:"expires_at"`
}
type Resolver interface {
	LookupMX(context.Context, string) ([]*net.MX, error)
	LookupIPAddr(context.Context, string) ([]net.IPAddr, error)
}
type Cache interface {
	Get(context.Context, string) (Result, bool)
	Put(context.Context, string, Result)
}
type Engine struct {
	Resolver         Resolver
	Probe            func(context.Context, Result, string, string) Result
	Store            Cache
	Timeout          time.Duration
	mu               sync.Mutex
	cached           map[string]Result
	flight           singleflight.Group
	slots            chan struct{}
	domainQueue      []string
	domainQueued     map[string]bool
	domainWorkers    int
	domainChanged    chan struct{}
	recipientQueue   []recipientJob
	recipientQueued  map[string]bool
	recipientWorkers int
	recipientChanged chan struct{}
}

func New(resolver Resolver) *Engine {
	return &Engine{Resolver: resolver, Timeout: 8 * time.Second, cached: map[string]Result{}, slots: make(chan struct{}, 8), Probe: probeSMTP}
}
func Normalize(email string) (string, string, bool) {
	email = strings.TrimSpace(email)
	if len(email) > 254 || strings.ContainsAny(email, "\r\n\x00") {
		return "", "", false
	}
	address, err := mail.ParseAddress(email)
	if err != nil || address.Address != email {
		return "", "", false
	}
	at := strings.LastIndexByte(email, '@')
	if at < 1 || at > 64 {
		return "", "", false
	}
	domain, err := idna.Lookup.ToASCII(strings.ToLower(email[at+1:]))
	if err != nil || !strings.Contains(domain, ".") || strings.HasSuffix(domain, ".") {
		return "", "", false
	}
	return strings.ToLower(email[:at]) + "@" + domain, domain, true
}
func publicIP(ip net.IP) bool {
	if !ip.IsGlobalUnicast() || ip.IsPrivate() || ip.IsLoopback() || ip.IsLinkLocalUnicast() {
		return false
	}
	for _, cidr := range []string{"100.64.0.0/10", "192.0.0.0/24", "198.18.0.0/15", "2001:db8::/32"} {
		_, network, _ := net.ParseCIDR(cidr)
		if network.Contains(ip) {
			return false
		}
	}
	return true
}
func result(status, code, stage string) Result {
	now := time.Now()
	ttl := time.Hour
	if stage == "smtp" {
		ttl = 6 * time.Hour
	}
	if status == Unknown {
		ttl = 30 * time.Second
		if stage == "smtp" {
			ttl = 5 * time.Minute
		}
		if code == "smtp_policy" {
			ttl = 15 * time.Minute
		}
		if code == "catch_all" {
			ttl = 6 * time.Hour
		}
	}
	return Result{Status: status, Code: code, Stage: stage, CheckedAt: now, ExpiresAt: now.Add(ttl)}
}
func (e *Engine) remember(ctx context.Context, key string, check func(context.Context) Result) Result {
	e.mu.Lock()
	r, ok := e.cached[key]
	e.mu.Unlock()
	if ok && time.Now().Before(r.ExpiresAt) {
		return r
	}
	if e.Store != nil {
		if r, ok = e.Store.Get(ctx, key); ok {
			e.mu.Lock()
			e.cached[key] = r
			e.mu.Unlock()
			return r
		}
	}
	value, _, _ := e.flight.Do(key, func() (interface{}, error) {
		e.mu.Lock()
		saved, exists := e.cached[key]
		e.mu.Unlock()
		if exists && time.Now().Before(saved.ExpiresAt) {
			return saved, nil
		}
		r := check(ctx)
		if ctx.Err() == nil {
			e.mu.Lock()
			if len(e.cached) > 50000 {
				e.cached = map[string]Result{}
			}
			e.cached[key] = r
			e.mu.Unlock()
			if e.Store != nil {
				e.Store.Put(ctx, key, r)
			}
		}
		return r, nil
	})
	return value.(Result)
}
func (e *Engine) Domain(ctx context.Context, domain string) Result {
	return e.remember(ctx, "dns:"+domain, func(ctx context.Context) Result {
		ctx, cancel := context.WithTimeout(ctx, 12*time.Second)
		defer cancel()
		mx, err := e.Resolver.LookupMX(ctx, domain)
		if err != nil {
			if dns, ok := err.(*net.DNSError); !ok || !dns.IsNotFound {
				return result(Unknown, "dns_temporary", "dns")
			}
		}
		implicit := len(mx) == 0
		if implicit {
			mx = []*net.MX{{Host: domain}}
		}
		for _, host := range mx {
			if host.Host == "." {
				return result(Invalid, "null_mx", "dns")
			}
		}
		sort.SliceStable(mx, func(i, j int) bool { return mx[i].Pref < mx[j].Pref })
		r := result(Valid, "mx_valid", "dns")
		if implicit {
			r.Code = "implicit_mx"
		}
		temporary := false
		seen := map[string]bool{}
		for _, record := range mx {
			host := strings.TrimSuffix(strings.ToLower(record.Host), ".")
			if host == "" || seen[host] {
				continue
			}
			seen[host] = true
			if len(seen) > 10 {
				break
			}
			addresses, err := e.Resolver.LookupIPAddr(ctx, host)
			if err != nil {
				if dns, ok := err.(*net.DNSError); !ok || !dns.IsNotFound {
					temporary = true
				}
				continue
			}
			route := Route{Host: host, IPs: []string{}}
			for _, address := range addresses {
				if publicIP(address.IP) {
					route.IPs = append(route.IPs, address.IP.String())
				}
			}
			if len(route.IPs) > 0 {
				r.Routes = append(r.Routes, route)
			}
		}
		if len(r.Routes) > 0 {
			return r
		}
		if temporary || ctx.Err() != nil {
			return result(Unknown, "dns_temporary", "dns")
		}
		return result(Invalid, "no_mail_route", "dns")
	})
}
func (e *Engine) Check(ctx context.Context, email, sender string, smtp bool) Result {
	normalized, domain, ok := Normalize(email)
	if !ok {
		return result(Invalid, "invalid_email", "dns")
	}
	dns := e.Domain(ctx, domain)
	if dns.Status != Valid || !smtp {
		return dns
	}
	_, senderDomain, ok := Normalize(sender)
	if !ok {
		return result(Unknown, "sender_required", "smtp")
	}
	catchKey := "catchall:" + senderDomain + ":" + domain
	e.mu.Lock()
	catch, found := e.cached[catchKey]
	e.mu.Unlock()
	if !found && e.Store != nil {
		catch, found = e.Store.Get(ctx, catchKey)
	}
	if found && catch.Code == "catch_all" && time.Now().Before(catch.ExpiresAt) {
		key := "smtp:" + senderDomain + ":" + normalized
		e.mu.Lock()
		e.cached[key] = catch
		e.mu.Unlock()
		if e.Store != nil {
			e.Store.Put(ctx, key, catch)
		}
		return catch
	}
	r := e.remember(ctx, "smtp:"+senderDomain+":"+normalized, func(ctx context.Context) Result {
		select {
		case e.slots <- struct{}{}:
			defer func() { <-e.slots }()
		case <-ctx.Done():
			return result(Unknown, "smtp_timeout", "smtp")
		}
		ctx, cancel := context.WithTimeout(ctx, e.Timeout)
		defer cancel()
		return e.Probe(ctx, dns, normalized, sender)
	})
	if r.Code == "catch_all" {
		flag := r
		flag.ExpiresAt = time.Now().Add(6 * time.Hour)
		e.mu.Lock()
		e.cached[catchKey] = flag
		e.mu.Unlock()
		if e.Store != nil {
			e.Store.Put(ctx, catchKey, flag)
		}
	}
	return r
}

func encode(r Result) string { b, _ := json.Marshal(r); return string(b) }
