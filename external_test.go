package http_test

import (
	gohttp "github.com/api-go/http"
	"crypto/tls"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestExternalAddress(t *testing.T) {
	trusted, _ := gohttp.NewExternalAddressResolver(gohttp.ExternalAddressPolicy{TrustedProxies: []string{"127.0.0.1/32"}})
	direct, _ := gohttp.NewExternalAddressResolver(gohttp.ExternalAddressPolicy{Mode: "direct"})
	for _, tc := range []struct {
		name, host, scheme, origin string
		headers                    http.Header
		trust, tls, bad            bool
	}{
		{name: "direct HTTP", host: "Example.COM:80", origin: "http://example.com"},
		{name: "direct TLS", host: "Example.COM:443", origin: "https://example.com", tls: true},
		{name: "IPv6", host: "[2001:db8::1]:8443", origin: "https://[2001:db8::1]:8443", tls: true},
		{name: "trusted", host: "internal:80", origin: "https://public.example", trust: true, headers: http.Header{"X-Forwarded-Host": {"Public.Example:443"}, "X-Forwarded-Proto": {"https"}}},
		{name: "spoof", host: "internal:80", origin: "http://internal", headers: http.Header{"X-Host": {"evil"}, "X-Scheme": {"https"}, "Forwarded": {"host=evil;proto=https"}}},
		{name: "partial", host: "internal", trust: true, bad: true, headers: http.Header{"X-Scheme": {"https"}}},
		{name: "list", host: "internal", trust: true, bad: true, headers: http.Header{"X-Host": {"public,evil"}, "X-Scheme": {"https"}}},
		{name: "duplicate", host: "internal", trust: true, bad: true, headers: http.Header{"X-Host": {"public", "public"}, "X-Scheme": {"https"}}},
		{name: "conflict", host: "internal", trust: true, bad: true, headers: http.Header{"X-Host": {"public"}, "X-Scheme": {"https"}, "X-Forwarded-Host": {"evil"}, "X-Forwarded-Proto": {"https"}}},
		{name: "unsupported", host: "internal", trust: true, bad: true, headers: http.Header{"Forwarded": {"host=public;proto=https"}}},
		{name: "invalid port", host: "example:65536", bad: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := httptest.NewRequest("GET", "http://internal/", nil)
			r.Host = tc.host
			r.RemoteAddr = "127.0.0.1:1234"
			r.Header = tc.headers.Clone()
			if r.Header == nil {
				r.Header = make(http.Header)
			}
			if tc.tls {
				r.TLS = &tls.ConnectionState{}
			}
			resolver := direct
			if tc.trust {
				resolver = trusted
			}
			a := resolver.Prepare(r)
			if (a.Err != nil) != tc.bad || !tc.bad && a.Origin != tc.origin {
				t.Fatalf("%+v", a)
			}
			r.Header.Set("X-Host", "changed")
			if gohttp.RequestExternalAddress(r).Origin != a.Origin {
				t.Fatal("snapshot changed")
			}
			if err := a.RelayHeaders(r.Header); (err != nil) != tc.bad {
				t.Fatal(err)
			}
			if tc.bad && r.Header.Get("X-Host") != "" {
				t.Fatal("invalid metadata relayed")
			}
		})
	}
}
func TestExternalSameOrigin(t *testing.T) {
	r := httptest.NewRequest("GET", "https://Example.COM:443/", nil)
	a := gohttp.RequestExternalAddress(r)
	for _, origin := range []string{"https://example.com", "https://EXAMPLE.com:443"} {
		if !a.SameOrigin(http.Header{"Origin": {origin}}) {
			t.Fatal(origin)
		}
	}
	for _, origin := range []string{"null", "", "https://example.com/", "https://example.com?", "https://example.com#", "https://user@example.com", "https://example.com.evil", "https://example.com:444", "http://example.com", "https://example.com https://evil"} {
		if a.SameOrigin(http.Header{"Origin": {origin}}) {
			t.Fatal(origin)
		}
	}
	if !a.SameOrigin(nil) || a.SameOrigin(http.Header{"Origin": {"https://example.com", "https://example.com"}}) {
		t.Fatal("missing/duplicate policy")
	}
}
func BenchmarkExternalAddress(b *testing.B) {
	resolver, _ := gohttp.NewExternalAddressResolver(gohttp.ExternalAddressPolicy{})
	for b.Loop() {
		r := httptest.NewRequest("GET", "https://example.com/", nil)
		resolver.Prepare(r)
	}
}

func TestDefaultExternalRelayAndOptionalStrictPolicy(t *testing.T) {
	for _, tc := range []struct {
		name, mode, id, want string
		proxies              []string
		bad                  bool
	}{
		{name: "first hop", want: "http://internal"},
		{name: "internal default", id: "from-edge", want: "https://browser.example"},
		{name: "explicit legacy", mode: "legacy", id: "from-edge", want: "https://browser.example"},
		{name: "optional direct", mode: "direct", id: "from-edge", want: "http://internal"},
		{name: "optional CIDR", mode: "trusted", proxies: []string{"203.0.113.1"}, id: "from-edge", want: "http://internal"},
		{name: "invalid mode", mode: "bad", bad: true},
		{name: "missing proxies", mode: "trusted", bad: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			resolver, err := gohttp.NewExternalAddressResolver(gohttp.ExternalAddressPolicy{Mode: tc.mode, TrustedProxies: tc.proxies})
			if tc.bad {
				if err == nil {
					t.Fatal("accepted invalid policy")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			r := httptest.NewRequest("GET", "http://internal/", nil)
			r.Header.Set("X-Host", "browser.example")
			r.Header.Set("X-Scheme", "https")
			if tc.id != "" {
				r.Header.Set("X-Request-ID", tc.id)
			}
			a := resolver.Prepare(r)
			if a.Err != nil || a.Origin != tc.want {
				t.Fatal(a)
			}
			if a.TrustedProxy {
				t.Fatal("legacy relay was mislabeled CIDR trusted")
			}
			r.Header.Set("X-Request-ID", "generated-later")
			if resolver.Prepare(r).Origin != tc.want {
				t.Fatal("snapshot changed after ID generation")
			}
		})
	}
}
