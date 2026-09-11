package http_test

import (
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	ah "apigo.cc/go/http"
)

func TestOptimizationForms(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.Header.Get("Content-Type"), "multipart/form-data") {
			_ = r.ParseMultipartForm(10 << 20)
		} else {
			_ = r.ParseForm()
		}
		fmt.Fprintf(w, "foo=%v", r.Form["foo"])
	})
	server := &http.Server{Addr: "127.0.0.1:18088", Handler: handler}
	go func() { _ = server.ListenAndServe() }()
	defer server.Close()
	time.Sleep(100 * time.Millisecond)

	c := ah.NewClient(time.Second)

	// Test map[string][]string
	r1 := c.Post("http://127.0.0.1:18088/", map[string][]string{"foo": {"bar", "baz"}})
	if r1.Error != nil {
		t.Fatalf("map[string][]string failed: %v", r1.Error)
	}
	if r1.String() != "foo=[bar baz]" {
		t.Errorf("expected foo=[bar baz], got %s", r1.String())
	}

	// Test map[string][]any (now Multipart)
	r2 := c.Post("http://127.0.0.1:18088/", map[string][]any{"foo": {"bar", 123}})
	if r2.Error != nil {
		t.Fatalf("map[string][]any failed: %v", r2.Error)
	}
	// Multipart output might look different depending on how the server parses it,
	// but r.Form["foo"] should still work if it's multipart.
	if r2.String() != "foo=[bar 123]" {
		t.Errorf("expected foo=[bar 123], got %s", r2.String())
	}
}

func TestMultipartStreaming(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		err := r.ParseMultipartForm(10 << 20)
		if err != nil {
			t.Errorf("ParseMultipartForm failed: %v", err)
		}
		f1 := r.FormValue("foo")
		file, _, err := r.FormFile("file")
		var fileContent []byte
		if file != nil {
			fileContent, _ = io.ReadAll(file)
		}
		fmt.Fprintf(w, "foo=%s,file=%s", f1, string(fileContent))
	})
	server := &http.Server{Addr: "127.0.0.1:18089", Handler: handler}
	go func() { _ = server.ListenAndServe() }()
	defer server.Close()
	time.Sleep(100 * time.Millisecond)

	c := ah.NewClient(time.Second)
	// Test streaming Multipart
	r := c.Post("http://127.0.0.1:18089/", ah.Multipart{
		"foo":  "bar",
		"file": io.NopCloser(strings.NewReader("baz")), // use Reader to force streaming
	})
	if r.Error != nil {
		t.Fatalf("Post with Multipart streaming failed: %v", r.Error)
	}
	if r.String() != "foo=bar,file=baz" {
		t.Errorf("expected foo=bar,file=baz, got %s", r.String())
	}
}

func TestMultipartMultipleParts(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseMultipartForm(10 << 20)
		fmt.Fprintf(w, "foo=%v", r.MultipartForm.Value["foo"])
	})
	server := &http.Server{Addr: "127.0.0.1:18090", Handler: handler}
	go func() { _ = server.ListenAndServe() }()
	defer server.Close()
	time.Sleep(100 * time.Millisecond)

	c := ah.NewClient(time.Second)
	r := c.Post("http://127.0.0.1:18090/", ah.Multipart{
		"foo": []string{"bar", "baz"},
	})
	if r.Error != nil {
		t.Fatalf("Post with Multipart multiple parts failed: %v", r.Error)
	}
	if r.String() != "foo=[bar baz]" {
		t.Errorf("expected foo=[bar baz], got %s", r.String())
	}
}
