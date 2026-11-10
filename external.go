package http

import (
	"github.com/api-go/cast"
	"context"
	"fmt"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"strings"
)

// ExternalAddressPolicy defaults to the controlled internal Request-ID relay contract.
// Mode direct always uses the connection; trusted requires matching direct-peer CIDRs.
// An omitted Mode with TrustedProxies selects trusted mode for compatibility.
type ExternalAddressPolicy struct {
	Mode           string
	TrustedProxies []string
}

// ExternalAddressResolver is immutable and safe for concurrent use.
type ExternalAddressResolver struct {
	mode     string
	networks []netip.Prefix
}

func NewExternalAddressResolver(policy ExternalAddressPolicy) (*ExternalAddressResolver, error) {
	mode := policy.Mode
	if mode == "" {
		mode = "legacy"
		if len(policy.TrustedProxies) > 0 {
			mode = "trusted"
		}
	}
	if mode != "legacy" && mode != "direct" && mode != "trusted" {
		return nil, fmt.Errorf("invalid external address mode %q", mode)
	}
	if mode == "trusted" && len(policy.TrustedProxies) == 0 {
		return nil, fmt.Errorf("trusted external address mode requires proxy addresses")
	}
	r := &ExternalAddressResolver{mode: mode}
	for _, value := range policy.TrustedProxies {
		prefix, err := netip.ParsePrefix(value)
		if err != nil {
			ip, e := netip.ParseAddr(value)
			if e != nil {
				return nil, fmt.Errorf("invalid external address proxy %q", value)
			}
			ip = ip.Unmap()
			prefix = netip.PrefixFrom(ip, ip.BitLen())
		}
		r.networks = append(r.networks, prefix.Masked())
	}
	return r, nil
}

// ExternalAddress is browser-facing context, never an RPC target or identity.
// Err must be checked before security decisions.
type ExternalAddress struct {
	Scheme       string
	Host         string
	Hostname     string
	Port         int
	Origin       string
	TrustedProxy bool
	Err          error
}

func (a ExternalAddress) IsHTTPS() bool { return a.Err == nil && a.Scheme == "https" }

type externalAddressKey struct{}

func directScheme(r *http.Request) string {
	if r.TLS != nil {
		return "https"
	}
	return "http"
}

// RequestExternalAddress returns the pinned result or prepares the default internal relay policy.
func RequestExternalAddress(r *http.Request) ExternalAddress {
	if a, ok := r.Context().Value(externalAddressKey{}).(ExternalAddress); ok {
		return a
	}
	return (*ExternalAddressResolver)(nil).Prepare(r)
}

// Prepare pins a snapshot before request sharing or header mutation. First preparation wins.
func (resolver *ExternalAddressResolver) Prepare(r *http.Request) ExternalAddress {
	if a, ok := r.Context().Value(externalAddressKey{}).(ExternalAddress); ok {
		return a
	}
	a := parseExternalAddress(directScheme(r), r.Host)
	mode := "legacy"
	if resolver != nil {
		mode = resolver.mode
	}
	// Inspect the incoming ID before ingress creates one. This is the existing
	// controlled-service relay convention, not a credential or CIDR trust claim.
	relay := mode == "legacy" && r.Header.Get(HeaderRequestID) != ""
	trusted := false
	if mode == "trusted" {
		peer, _, err := net.SplitHostPort(r.RemoteAddr)
		if err != nil {
			peer = r.RemoteAddr
		}
		if ip, err := netip.ParseAddr(peer); err == nil {
			for _, prefix := range resolver.networks {
				if prefix.Contains(ip.Unmap()) {
					trusted = true
					break
				}
			}
		}
	}
	if trusted || relay {
		scheme, host, present, e := externalHeaderPair(r.Header, HeaderScheme, HeaderHost)
		scheme2, host2, present2, e2 := externalHeaderPair(r.Header, "X-Forwarded-Proto", "X-Forwarded-Host")
		if e != nil || e2 != nil {
			a.Err = fmt.Errorf("invalid external address header group")
		} else if present || present2 {
			if !present {
				scheme, host = scheme2, host2
			}
			a = parseExternalAddress(scheme, host)
			if present && present2 {
				other := parseExternalAddress(scheme2, host2)
				if other.Err != nil || a.Origin != other.Origin {
					a.Err = fmt.Errorf("conflicting external address header groups")
				}
			}
		} else if len(externalHeaderValues(r.Header, "Forwarded")) > 0 {
			a.Err = fmt.Errorf("Forwarded unsupported; configure a supported header pair")
		}
	}
	a.TrustedProxy = trusted
	*r = *r.WithContext(context.WithValue(r.Context(), externalAddressKey{}, a))
	return a
}
func externalHeaderValues(h http.Header, name string) []string {
	var values []string
	for key, items := range h {
		if strings.EqualFold(key, name) {
			values = append(values, items...)
		}
	}
	return values
}
func externalHeaderPair(h http.Header, scheme, host string) (string, string, bool, error) {
	s, v := externalHeaderValues(h, scheme), externalHeaderValues(h, host)
	if len(s)+len(v) == 0 {
		return "", "", false, nil
	}
	if len(s) != 1 || len(v) != 1 || strings.ContainsAny(s[0]+v[0], ", \t\r\n") {
		return "", "", true, fmt.Errorf("expected single complete header pair")
	}
	return s[0], v[0], true, nil
}
func parseExternalAddress(scheme, host string) ExternalAddress {
	a := ExternalAddress{Scheme: strings.ToLower(scheme)}
	invalid := func() ExternalAddress { a.Err = fmt.Errorf("invalid external address"); return a }
	if a.Scheme != "http" && a.Scheme != "https" {
		return invalid()
	}
	if host == "" || strings.ContainsAny(host, " /\\?#@,%\r\n\t") {
		return invalid()
	}
	u, err := url.Parse(a.Scheme + "://" + host)
	if err != nil || u.Host != host || u.User != nil || u.Hostname() == "" {
		return invalid()
	}
	hostname := strings.ToLower(u.Hostname())
	if strings.Contains(hostname, ":") {
		ip, e := netip.ParseAddr(hostname)
		if e != nil || !strings.HasPrefix(host, "[") {
			return invalid()
		}
		hostname = ip.String()
	} else {
		for _, c := range hostname {
			if !(c >= 'a' && c <= 'z' || c >= '0' && c <= '9' || c == '-' || c == '.' || c == '_') {
				return invalid()
			}
		}
		if strings.ContainsAny(host, "[]") {
			return invalid()
		}
	}
	port := 80
	if a.Scheme == "https" {
		port = 443
	}
	if strings.HasSuffix(host, ":") {
		return invalid()
	}
	if value := u.Port(); value != "" {
		for _, c := range value {
			if c < '0' || c > '9' {
				return invalid()
			}
		}
		port = cast.Int(value)
		if port < 1 || port > 65535 {
			return invalid()
		}
	}
	authority := hostname
	if strings.Contains(hostname, ":") {
		authority = "[" + hostname + "]"
	}
	if a.Scheme == "http" && port != 80 || a.Scheme == "https" && port != 443 {
		authority = net.JoinHostPort(hostname, cast.String(port))
	}
	a.Hostname, a.Port, a.Host = hostname, port, authority
	a.Origin = a.Scheme + "://" + authority
	return a
}

// SameOrigin permits absent Origin for existing non-browser callers; null, duplicates,
// lists, credentials, paths, queries and fragments are rejected.
func (a ExternalAddress) SameOrigin(h http.Header) bool {
	if a.Err != nil {
		return false
	}
	values := externalHeaderValues(h, "Origin")
	if len(values) == 0 {
		return true
	}
	if len(values) != 1 || values[0] == "" {
		return false
	}
	u, err := url.Parse(values[0])
	if err != nil || u.User != nil || u.Opaque != "" || u.Path != "" || u.RawQuery != "" || u.ForceQuery || u.Fragment != "" || strings.ContainsAny(values[0], "#?\\") {
		return false
	}
	other := parseExternalAddress(u.Scheme, u.Host)
	return other.Err == nil && other.Origin == a.Origin
}

// RelayHeaders replaces all address forwarding headers with canonical single-value pairs.
// Origin, request Host, URL and TLS are untouched. Errors clear the headers without replacement.
func (a ExternalAddress) RelayHeaders(h http.Header) error {
	for key := range h {
		for _, name := range []string{HeaderHost, HeaderScheme, "X-Forwarded-Host", "X-Forwarded-Proto", "Forwarded"} {
			if strings.EqualFold(key, name) {
				delete(h, key)
				break
			}
		}
	}
	if a.Err != nil {
		return a.Err
	}
	h.Set(HeaderHost, a.Host)
	h.Set(HeaderScheme, a.Scheme)
	h.Set("X-Forwarded-Host", a.Host)
	h.Set("X-Forwarded-Proto", a.Scheme)
	return nil
}

// PreparedExternalAddress distinguishes a parsed ingress snapshot (including errors)
// from legacy raw relay context. The boolean reports preparation, not proxy trust.
func PreparedExternalAddress(r *http.Request) (ExternalAddress, bool) {
	a, ok := r.Context().Value(externalAddressKey{}).(ExternalAddress)
	return a, ok
}
