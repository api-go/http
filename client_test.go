package http

import (
	"fmt"
	"io"
	"net"
	"net/http"
	"testing"
	"time"

	"golang.org/x/net/http2"
)

func TestHttp(t *testing.T) {
	c := NewClient(5 * time.Second)
	// 使用 httpbin 或者可靠的地址
	r := c.Get("https://httpbin.org/get")
	if r.Error != nil {
		t.Skip("network unreachable, skipping remote test:", r.Error)
		return
	}
	if r.Response.StatusCode != 200 {
		t.Errorf("expected 200, got %d", r.Response.StatusCode)
	}

	type HttpBinGet struct {
		Url string
	}
	res, err := ToT[HttpBinGet](r)
	if err != nil {
		t.Errorf("ToT failed: %v", err)
	}
	if res.Url != "https://httpbin.org/get" {
		t.Errorf("expected url match, got %s", res.Url)
	}
}

func TestLocalServer(t *testing.T) {
	handler := http.NewServeMux()
	handler.HandleFunc("/hello", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Test-Echo", r.Header.Get("X-Test"))
		_, _ = w.Write([]byte("world"))
	})

	server := &http.Server{Addr: ":18080", Handler: handler}
	go func() { _ = server.ListenAndServe() }()
	defer server.Close()

	// 等待启动
	time.Sleep(100 * time.Millisecond)

	c := NewClient(time.Second)
	r := c.Get("http://127.0.0.1:18080/hello", "X-Test", "hi")
	if r.Error != nil {
		t.Fatalf("request failed: %v", r.Error)
	}
	if r.String() != "world" {
		t.Errorf("expected world, got %s", r.String())
	}
	if r.Response.Header.Get("X-Test-Echo") != "hi" {
		t.Errorf("header not echoed")
	}
}

func TestH2C(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:18081")
	if err != nil {
		t.Fatalf("failed to listen: %v", err)
	}
	defer ln.Close()

	s2 := &http2.Server{}
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, "Hello H2C")
	})

	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			go s2.ServeConn(conn, &http2.ServeConnOpts{
				Handler: handler,
			})
		}
	}()

	c := NewClientH2C(time.Second)
	r := c.Get("http://127.0.0.1:18081/")
	if r.Error != nil {
		t.Fatalf("h2c request failed: %v", r.Error)
	}
	if r.String() != "Hello H2C" {
		t.Errorf("expected Hello H2C, got %s", r.String())
	}
}

func BenchmarkGet(b *testing.B) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("ok"))
	})
	server := &http.Server{Addr: ":18082", Handler: handler}
	go func() { _ = server.ListenAndServe() }()
	defer server.Close()
	time.Sleep(100 * time.Millisecond)

	c := NewClient(0)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		r := c.Get("http://127.0.0.1:18082/")
		if r.Error != nil {
			b.Fatal(r.Error)
		}
	}
}

func TestManualDo(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("stream"))
	})
	server := &http.Server{Addr: ":18083", Handler: handler}
	go func() { _ = server.ListenAndServe() }()
	defer server.Close()
	time.Sleep(100 * time.Millisecond)

	c := NewClient(time.Second)
	r := c.ManualDo("GET", "http://127.0.0.1:18083/", nil)
	if r.Error != nil {
		t.Fatal(r.Error)
	}
	if r.data != nil {
		t.Error("expected data to be nil in ManualDo")
	}
	defer r.Response.Body.Close()
	buf, _ := io.ReadAll(r.Response.Body)
	if string(buf) != "stream" {
		t.Errorf("expected stream, got %s", string(buf))
	}
}
