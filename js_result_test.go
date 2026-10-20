package http_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"apigo.cc/go/file"
	_ "apigo.cc/go/http"
	"apigo.cc/go/jsmod"
)

func TestJavaScriptHTTPResultAndCancellation(t *testing.T) {
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		if r.URL.Path == "/wait" {
			<-r.Context().Done()
			return
		}
		if r.URL.Path == "/unavailable" {
			w.WriteHeader(503)
		}
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer server.Close()
	exports := jsmod.GetModules()["http"].Exports
	client := reflect.ValueOf(exports["New"]).Call([]reflect.Value{reflect.ValueOf(5000)})[0]
	for _, method := range []string{"GET", "DELETE"} {
		for _, scope := range []string{"static", "client"} {
			t.Run(scope+method, func(t *testing.T) {
				fn := reflect.ValueOf(exports[method])
				if scope == "client" {
					fn = client.MethodByName(method)
				}
				if fn.Type().NumOut() != 2 || fn.Type().Out(1) != reflect.TypeOf((*error)(nil)).Elem() {
					t.Fatal("result with Error() must not occupy the bridge's error-return slot")
				}
				invoke := func(ctx context.Context, path string) reflect.Value {
					args := []reflect.Value{reflect.ValueOf(ctx), reflect.ValueOf(server.URL + path)}
					if method == "DELETE" {
						args = append(args, reflect.Zero(fn.Type().In(2)))
					}
					args = append(args, reflect.ValueOf(map[string]string{}))
					out := fn.Call(args)
					if !out[1].IsNil() {
						t.Fatalf("unexpected bridge error: %v", out[1].Interface())
					}
					return out[0]
				}
				for _, pair := range []struct {
					path   string
					status int
				}{{"/", 200}, {"/unavailable", 503}} {
					result := invoke(context.Background(), pair.path)
					if got := int(result.MethodByName("Status").Call(nil)[0].Int()); got != pair.status {
						t.Fatalf("status %d, want %d", got, pair.status)
					}
					if text := result.MethodByName("String").Call(nil)[0].String(); !strings.Contains(text, `"ok":true`) {
						t.Fatalf("body lost: %s", text)
					}
				}
				ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
				defer cancel()
				started := time.Now()
				result := invoke(ctx, "/wait")
				if time.Since(started) > time.Second {
					t.Fatal("request ignored invocation cancellation")
				}
				if message := result.MethodByName("Error").Call(nil)[0].String(); !strings.Contains(message, "context deadline exceeded") {
					t.Fatalf("transport error lost: %s", message)
				}
				if method == "DELETE" {
					filename := filepath.Join(t.TempDir(), "fixture.txt")
					if err := file.Write(filename, "fixture"); err != nil {
						t.Fatal(err)
					}
					multipart := reflect.ValueOf(exports["Multipart"]).Call([]reflect.Value{reflect.ValueOf(map[string]any{"upload": filename})})[0]
					safeContext := jsmod.NewContext(context.Background(), map[string]any{"SafeMode": true})
					before := requests.Load()
					out := fn.Call([]reflect.Value{reflect.ValueOf(safeContext), reflect.ValueOf(server.URL), multipart, reflect.ValueOf(map[string]string{})})
					if out[1].IsNil() || requests.Load() != before {
						t.Fatal("DELETE read/sent a local file outside the safe-mode grant")
					}
				}
			})
		}
	}
}
