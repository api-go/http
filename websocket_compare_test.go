package http_test

import (
	"bytes"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"runtime"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	gohttp "github.com/api-go/http"
)

// Same TLS server, payload and Gorilla framing on both protocols. No compression.
// Warm echo excludes setup; handshake benchmarks separately measure setup cost.
func BenchmarkWebSocketTLSCompare(b *testing.B) {
	requireExtendedConnectBenchmark(b)
	for _, size := range []int{64, 1024, 65536} {
		for _, parallel := range []bool{false, true} {
			for _, h2 := range []bool{false, true} {
				protocol := "H1"
				if h2 {
					protocol = "H2"
				}
				workers := 1
				if parallel {
					workers = runtime.GOMAXPROCS(0)
				}
				b.Run(fmt.Sprintf("%s/%dB/%dStreams", protocol, size, workers), func(b *testing.B) {
					var tcp atomic.Int32
					server := httptest.NewUnstartedServer(http.HandlerFunc(autoEcho))
					server.EnableHTTP2 = true
					server.Config.ConnState = func(_ net.Conn, state http.ConnState) {
						if state == http.StateNew {
							tcp.Add(1)
						}
					}
					server.StartTLS()
					defer server.Close()
					client := gohttp.NewClient(5 * time.Second)
					client.GetRawClient().Transport = server.Client().Transport
					client.SetWebSocketHTTP2(h2)
					defer client.Destroy()
					conns := make([]*gohttp.WebSocketConn, workers)
					for i := range conns {
						conn, response, err := client.Open("wss" + server.URL[5:])
						if err != nil {
							b.Fatal(err)
						}
						if (response.ProtoMajor == 2) != h2 {
							b.Fatal(response.Proto)
						}
						conns[i] = conn
						defer conn.Close()
					}
					payload := bytes.Repeat([]byte("x"), size)
					echo := func(conn *gohttp.WebSocketConn) bool {
						if err := conn.Send(payload); err != nil {
							b.Error(err)
							return false
						}
						data, err := conn.ReadBytes()
						if err != nil || len(data) != size {
							b.Errorf("echo %d %v", len(data), err)
							return false
						}
						return true
					}
					b.SetBytes(int64(size))
					b.ReportAllocs()
					b.ResetTimer()
					if parallel {
						var sequence atomic.Int32
						b.RunParallel(func(pb *testing.PB) {
							conn := conns[int(sequence.Add(1))-1]
							for pb.Next() {
								if !echo(conn) {
									return
								}
							}
						})
					} else {
						for i := 0; i < b.N; i++ {
							if !echo(conns[0]) {
								break
							}
						}
					}
					b.StopTimer()
					b.ReportMetric(float64(tcp.Load()), "tcp-conns")
				})
			}
		}
	}
}
func BenchmarkWebSocketTLSHandshake(b *testing.B) {
	requireExtendedConnectBenchmark(b)
	for _, warm := range []bool{false, true} {
		for _, h2 := range []bool{false, true} {
			protocol := "H1"
			if h2 {
				protocol = "H2"
			}
			state := "cold"
			if warm {
				state = "warm"
			}
			b.Run(protocol+"/"+state, func(b *testing.B) {
				server := httptest.NewUnstartedServer(http.HandlerFunc(autoEcho))
				server.EnableHTTP2 = true
				server.StartTLS()
				defer server.Close()
				newClient := func() *gohttp.Client {
					client := gohttp.NewClient(5 * time.Second)
					client.GetRawClient().Transport = server.Client().Transport
					client.SetWebSocketHTTP2(h2)
					return client
				}
				client := newClient()
				defer client.Destroy()
				if warm {
					conn, _, err := client.Open("wss" + server.URL[5:])
					if err != nil {
						b.Fatal(err)
					}
					conn.Close()
				}
				b.ReportAllocs()
				b.ResetTimer()
				for i := 0; i < b.N; i++ {
					if !warm {
						client = newClient()
					}
					conn, _, err := client.Open("wss" + server.URL[5:])
					if err != nil {
						b.Fatal(err)
					}
					conn.Close()
					if !warm {
						client.Destroy()
					}
				}
			})
		}
	}
}
func requireExtendedConnectBenchmark(b *testing.B) {
	// The server-side Go flag is read at process initialization.
	if !strings.Contains(os.Getenv("GODEBUG"), "http2xconnect=1") {
		b.Skip("requires GODEBUG=http2xconnect=1")
	}
}
