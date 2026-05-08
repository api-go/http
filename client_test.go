package http_test

import (
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"testing"
	"time"

	ah "apigo.cc/go/http"
	"golang.org/x/net/http2"
)

func TestHttp(t *testing.T) {
	c := ah.NewClient(5 * time.Second)
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
	res, err := ah.To[HttpBinGet](r)
	if err != nil {
		t.Errorf("Bind failed: %v", err)
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

	c := ah.NewClient(time.Second)
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

	c := ah.NewClientH2C(time.Second)
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

	c := ah.NewClient(0)
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

	c := ah.NewClient(time.Second)
	r := c.ManualDo("GET", "http://127.0.0.1:18083/", nil)
	if r.Error != nil {
		t.Fatal(r.Error)
	}

	defer r.Response.Body.Close()
	buf, _ := io.ReadAll(r.Response.Body)
	if string(buf) != "stream" {
		t.Errorf("expected stream, got %s", string(buf))
	}
}

func TestDownload(t *testing.T) {
	content := "0123456789ABCDEF"
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "HEAD" {
			w.Header().Set("Content-Length", fmt.Sprintf("%d", len(content)))
			return
		}
		rangeHeader := r.Header.Get("Range")
		if rangeHeader != "" {
			var start, end int64
			fmt.Sscanf(rangeHeader, "bytes=%d-%d", &start, &end)
			w.Header().Set("Content-Range", fmt.Sprintf("bytes %d-%d/%d", start, end, len(content)))
			w.WriteHeader(http.StatusPartialContent)
			_, _ = w.Write([]byte(content[start : end+1]))
			return
		}
		_, _ = w.Write([]byte(content))
	})
	server := &http.Server{Addr: ":18084", Handler: handler}
	go func() { _ = server.ListenAndServe() }()
	defer server.Close()
	time.Sleep(100 * time.Millisecond)

	c := ah.NewClient(time.Second)
	c.DownloadPartSize = 4
	tmpFile := "test_download.tmp"
	defer os.Remove(tmpFile)

	_, err := c.Download(tmpFile, "http://127.0.0.1:18084/", nil)
	if err != nil {
		t.Fatalf("download failed: %v", err)
	}

	data, _ := os.ReadFile(tmpFile)
	if string(data) != content {
		t.Errorf("expected %s, got %s", content, string(data))
	}
}

func TestMultipartDo(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseMultipartForm(10 << 20)
		f := r.FormValue("foo")
		file, _, _ := r.FormFile("file")
		var fileContent []byte
		if file != nil {
			fileContent, _ = io.ReadAll(file)
		}
		fmt.Fprintf(w, "foo=%s,file=%s", f, string(fileContent))
	})
	server := &http.Server{Addr: ":18086", Handler: handler}
	go func() { _ = server.ListenAndServe() }()
	defer server.Close()
	time.Sleep(100 * time.Millisecond)

	c := ah.NewClient(time.Second)
	r := c.Post("http://127.0.0.1:18086/", ah.Multipart{"foo": "bar", "file": []byte("baz")})
	if r.Error != nil {
		t.Fatalf("Post with Multipart failed: %v", r.Error)
	}
	if r.String() != "foo=bar,file=baz" {
		t.Errorf("expected foo=bar,file=baz, got %s", r.String())
	}
}

func TestFormAndMap(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ct := r.Header.Get("Content-Type")
		body, _ := io.ReadAll(r.Body)
		fmt.Fprintf(w, "ct=%s,body=%s", ct, string(body))
	})
	server := &http.Server{Addr: ":18087", Handler: handler}
	go func() { _ = server.ListenAndServe() }()
	defer server.Close()
	time.Sleep(100 * time.Millisecond)

	c := ah.NewClient(time.Second)

	// Test Form (urlencoded)
	r1 := c.Post("http://127.0.0.1:18087/", ah.Form{"foo": "bar"})
	if r1.String() != "ct=application/x-www-form-urlencoded,body=foo=bar" {
		t.Errorf("Form failed, got: %s", r1.String())
	}

	// Test map[string]string (JSON)
	r2 := c.Post("http://127.0.0.1:18087/", map[string]string{"foo": "bar"})
	if r2.String() != `ct=application/json,body={"foo":"bar"}` {
		t.Errorf("map[string]string failed, got: %s", r2.String())
	}
}
