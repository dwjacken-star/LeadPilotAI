package growth

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"golang.org/x/net/html"
)

func publicAddress(ip net.IP) bool {
	return ip != nil && ip.IsGlobalUnicast() && !ip.IsPrivate() && !ip.IsLoopback() && !ip.IsLinkLocalUnicast() && !ip.IsUnspecified() && !reservedAddress(ip)
}
func safeURL(value string) (*url.URL, error) {
	u, e := url.Parse(value)
	if e != nil || u.User != nil || (u.Scheme != "https" && u.Scheme != "http") || u.Hostname() == "" {
		return nil, errors.New("public HTTP/HTTPS URL required")
	}
	if ip := net.ParseIP(u.Hostname()); ip != nil && !publicAddress(ip) {
		return nil, errors.New("private address blocked")
	}
	if port := u.Port(); port != "" && port != "80" && port != "443" {
		return nil, errors.New("non-web ports blocked")
	}
	return u, nil
}

var crawlHTTP = &http.Client{Timeout: 15 * time.Second, Transport: &http.Transport{Proxy: nil, DialContext: func(ctx context.Context, network, address string) (net.Conn, error) {
	host, port, e := net.SplitHostPort(address)
	if e != nil {
		return nil, e
	}
	ips, e := net.DefaultResolver.LookupIPAddr(ctx, host)
	if e != nil {
		return nil, e
	}
	for _, ip := range ips {
		if !publicAddress(ip.IP) {
			return nil, errors.New("private DNS address blocked")
		}
	}
	if len(ips) == 0 {
		return nil, errors.New("host has no address")
	}
	return (&net.Dialer{Timeout: 5 * time.Second}).DialContext(ctx, network, net.JoinHostPort(ips[0].IP.String(), port))
}}, CheckRedirect: func(req *http.Request, via []*http.Request) error {
	if len(via) > 4 {
		return errors.New("too many redirects")
	}
	_, e := safeURL(req.URL.String())
	return e
}}
var emailPattern = regexp.MustCompile(`(?i)[a-z0-9.!#$%&'*+/=?^_` + "`" + `{|}~-]+@[a-z0-9](?:[a-z0-9-]*[a-z0-9])?(?:\.[a-z0-9](?:[a-z0-9-]*[a-z0-9])?)+`)

type WebPage struct {
	URL    string
	Text   string
	Links  []string
	Emails []string
}

func robotsAllows(content, path string) bool {
	applies := false
	for _, line := range strings.Split(content, "\n") {
		line = strings.TrimSpace(strings.SplitN(line, "#", 2)[0])
		pair := strings.SplitN(line, ":", 2)
		if len(pair) != 2 {
			continue
		}
		key, value := strings.ToLower(strings.TrimSpace(pair[0])), strings.TrimSpace(pair[1])
		if key == "user-agent" {
			applies = value == "*" || strings.Contains(strings.ToLower(value), "maplemail")
		}
		if applies && key == "disallow" && value != "" && (value == "/" || strings.HasPrefix(path, strings.TrimSuffix(value, "$"))) {
			return false
		}
	}
	return true
}
func fetchPublic(ctx context.Context, value string) ([]byte, *url.URL, error) {
	u, e := safeURL(value)
	if e != nil {
		return nil, nil, e
	}
	req, e := http.NewRequestWithContext(ctx, "GET", u.String(), nil)
	if e != nil {
		return nil, nil, e
	}
	req.Header.Set("User-Agent", "MapleMailResearch/1.0 (+business-contact-research)")
	response, e := crawlHTTP.Do(req)
	if e != nil {
		return nil, nil, e
	}
	defer response.Body.Close()
	if response.StatusCode != 200 {
		return nil, response.Request.URL, errors.New("website denied or unavailable")
	}
	if !strings.Contains(response.Header.Get("Content-Type"), "text/") {
		return nil, response.Request.URL, errors.New("only public text/HTML pages are collected")
	}
	raw, e := io.ReadAll(io.LimitReader(response.Body, 2*1024*1024+1))
	if e != nil || len(raw) > 2*1024*1024 {
		return nil, nil, errors.New("page too large")
	}
	return raw, response.Request.URL, nil
}

var FetchWebsite = func(ctx context.Context, value string) (WebPage, error) {
	u, e := safeURL(value)
	if e != nil {
		return WebPage{}, e
	}
	robots := *u
	robots.Path, robots.RawQuery = "/robots.txt", ""
	if body, _, err := fetchPublic(ctx, robots.String()); err == nil && !robotsAllows(string(body), u.Path) {
		return WebPage{}, errors.New("collection disallowed by robots rules")
	}
	raw, actual, e := fetchPublic(ctx, value)
	if e != nil {
		return WebPage{}, e
	}
	node, e := html.Parse(strings.NewReader(string(raw)))
	if e != nil {
		return WebPage{}, e
	}
	page := WebPage{URL: actual.String()}
	var text strings.Builder
	emails := map[string]bool{}
	links := map[string]bool{}
	var visit func(*html.Node)
	visit = func(n *html.Node) {
		if n.Type == html.ElementNode && (n.Data == "script" || n.Data == "style" || n.Data == "noscript") {
			return
		}
		if n.Type == html.TextNode {
			text.WriteString(n.Data)
			text.WriteByte(' ')
		}
		if n.Type == html.ElementNode && n.Data == "a" {
			for _, attr := range n.Attr {
				if attr.Key != "href" {
					continue
				}
				if strings.HasPrefix(strings.ToLower(attr.Val), "mailto:") {
					address := strings.SplitN(attr.Val[len("mailto:"):], "?", 2)[0]
					for _, email := range emailPattern.FindAllString(address, -1) {
						emails[strings.ToLower(email)] = true
					}
				} else if relative, err := url.Parse(attr.Val); err == nil {
					target := actual.ResolveReference(relative)
					target.Fragment = ""
					if target.Hostname() == actual.Hostname() && (strings.Contains(strings.ToLower(target.Path), "contact") || strings.Contains(strings.ToLower(target.Path), "about") || strings.Contains(strings.ToLower(target.Path), "kontakt")) {
						links[target.String()] = true
					}
				}
			}
		}
		for child := n.FirstChild; child != nil; child = child.NextSibling {
			visit(child)
		}
	}
	visit(node)
	page.Text = cleanText(strings.Join(strings.Fields(text.String()), " "), 60000)
	for _, email := range emailPattern.FindAllString(page.Text, -1) {
		emails[strings.ToLower(email)] = true
	}
	for email := range emails {
		page.Emails = append(page.Emails, email)
	}
	for link := range links {
		page.Links = append(page.Links, link)
	}
	return page, nil
}
