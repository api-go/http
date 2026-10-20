package http

import (
	"context"
	"net/http"
	"time"

	"apigo.cc/go/cast"
	"apigo.cc/go/file"
	"apigo.cc/go/jsmod"
)

func init() {
	jsmod.Register("http", map[string]any{
		// Static requests with default timeout (30s)
		"GET":    jsGET,
		"POST":   jsPOST,
		"PUT":    jsPUT,
		"DELETE": jsDELETE,
		"Open":   jsOpen,

		// Client creation
		"New":    jsNewClient,
		"NewH2C": jsNewClientH2C,

		// Data markers
		"Form":      jsForm,
		"Multipart": jsMultipart,
	})
}

// Keep jsResult (which has Error()) separate from the bridge's error-return slot.
func jsGET(ctx context.Context, url string, headers map[string]string) (*jsResult, error) {
	return wrapResult(ctx, DefaultClient.DoContext(ctx, http.MethodGet, url, nil, flattenHeaders(headers)...)), nil
}

func jsPOST(ctx context.Context, url string, data any, headers map[string]string) (*jsResult, error) {
	if err := verifyMultipart(ctx, data); err != nil {
		return nil, jsmod.MakeError(err)
	}
	return wrapResult(ctx, DefaultClient.DoContext(ctx, http.MethodPost, url, data, flattenHeaders(headers)...)), nil
}

func jsPUT(ctx context.Context, url string, data any, headers map[string]string) (*jsResult, error) {
	if err := verifyMultipart(ctx, data); err != nil {
		return nil, jsmod.MakeError(err)
	}
	return wrapResult(ctx, DefaultClient.DoContext(ctx, http.MethodPut, url, data, flattenHeaders(headers)...)), nil
}

func jsDELETE(ctx context.Context, url string, data any, headers map[string]string) (*jsResult, error) {
	if err := verifyMultipart(ctx, data); err != nil {
		return nil, jsmod.MakeError(err)
	}
	return wrapResult(ctx, DefaultClient.DoContext(ctx, http.MethodDelete, url, data, flattenHeaders(headers)...)), nil
}

func jsOpen(ctx context.Context, url string, headers map[string]any) (*jsWebSocketConn, error) {
	conn, _, err := OpenContext(ctx, url, websocketHeaders(headers))
	if err != nil {
		return nil, jsmod.MakeError(err)
	}
	return &jsWebSocketConn{conn: conn}, nil
}

func jsNewClient(ms int) *jsClient {
	return &jsClient{c: NewClient(time.Duration(ms) * time.Millisecond)}
}

func jsNewClientH2C(ms int) *jsClient {
	return &jsClient{c: NewClientH2C(time.Duration(ms) * time.Millisecond)}
}

func jsForm(data map[string]string) Form {
	return Form(data)
}

func jsMultipart(data map[string]any) Multipart {
	return Multipart(data)
}

func flattenHeaders(m map[string]string) []string {
	if len(m) == 0 {
		return nil
	}
	res := make([]string, 0, len(m)*2)
	for k, v := range m {
		res = append(res, k, v)
	}
	return res
}

func verifyMultipart(ctx context.Context, data any) error {
	if m, ok := data.(Multipart); ok {
		for _, v := range m {
			if s, ok := v.(string); ok && file.Exists(s) {
				if _, err := file.VerifyPathForSafeMode(ctx, s); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

// jsClient wraps the internal Client to provide a JS-friendly interface
type jsClient struct {
	c *Client
}

func (jc *jsClient) GET(ctx context.Context, url string, headers map[string]string) (*jsResult, error) {
	return wrapResult(ctx, jc.c.DoContext(ctx, http.MethodGet, url, nil, flattenHeaders(headers)...)), nil
}

func (jc *jsClient) POST(ctx context.Context, url string, data any, headers map[string]string) (*jsResult, error) {
	if err := verifyMultipart(ctx, data); err != nil {
		return nil, jsmod.MakeError(err)
	}
	return wrapResult(ctx, jc.c.DoContext(ctx, http.MethodPost, url, data, flattenHeaders(headers)...)), nil
}

func (jc *jsClient) PUT(ctx context.Context, url string, data any, headers map[string]string) (*jsResult, error) {
	if err := verifyMultipart(ctx, data); err != nil {
		return nil, jsmod.MakeError(err)
	}
	return wrapResult(ctx, jc.c.DoContext(ctx, http.MethodPut, url, data, flattenHeaders(headers)...)), nil
}

func (jc *jsClient) DELETE(ctx context.Context, url string, data any, headers map[string]string) (*jsResult, error) {
	if err := verifyMultipart(ctx, data); err != nil {
		return nil, jsmod.MakeError(err)
	}
	return wrapResult(ctx, jc.c.DoContext(ctx, http.MethodDelete, url, data, flattenHeaders(headers)...)), nil
}

func (jc *jsClient) Open(ctx context.Context, url string, headers map[string]any) (*jsWebSocketConn, error) {
	conn, _, err := jc.c.OpenContext(ctx, url, websocketHeaders(headers))
	if err != nil {
		return nil, jsmod.MakeError(err)
	}
	return &jsWebSocketConn{conn: conn}, nil
}

type jsWebSocketConn struct {
	conn *WebSocketConn
}

func (conn *jsWebSocketConn) Send(data any) error {
	if err := conn.conn.Send(data); err != nil {
		return jsmod.MakeError(err)
	}
	return nil
}

func (conn *jsWebSocketConn) ReadString() (string, error) {
	value, err := conn.conn.ReadString()
	if err != nil {
		return "", jsmod.MakeError(err)
	}
	return value, nil
}

func (conn *jsWebSocketConn) ReadBytes() ([]byte, error) {
	value, err := conn.conn.ReadBytes()
	if err != nil {
		return nil, jsmod.MakeError(err)
	}
	return value, nil
}

func (conn *jsWebSocketConn) ReadJSON(value any) error {
	if err := conn.conn.ReadJSON(value); err != nil {
		return jsmod.MakeError(err)
	}
	return nil
}

func (conn *jsWebSocketConn) Close() error {
	if err := conn.conn.Close(); err != nil {
		return jsmod.MakeError(err)
	}
	return nil
}

// jsResult wraps *Result to hide sensitive methods like Save()
type jsResult struct {
	ctx context.Context
	r   *Result
}

func wrapResult(ctx context.Context, r *Result) *jsResult {
	return &jsResult{ctx: ctx, r: r}
}

func (jr *jsResult) String() string {
	return jr.r.String()
}

func (jr *jsResult) Bytes() []byte {
	return jr.r.Bytes()
}

func (jr *jsResult) Map() map[string]any {
	return jr.r.Map()
}

func (jr *jsResult) Slice() []any {
	return jr.r.Slice()
}

func (jr *jsResult) Status() int {
	if jr.r.Response != nil {
		return jr.r.Response.StatusCode
	}
	return 0
}

func (jr *jsResult) Error() string {
	if jr.r.Error != nil {
		return jr.r.Error.Error()
	}
	return ""
}

func (jr *jsResult) Save(filename string) error {
	p, err := file.VerifyPathForSafeMode(jr.ctx, filename)
	if err != nil {
		return jsmod.MakeError(err)
	}
	if err := jr.r.Save(p); err != nil {
		return jsmod.MakeError(err)
	}
	return nil
}

func websocketHeaders(headers map[string]any) http.Header {
	result := make(http.Header)
	for key, value := range headers {
		switch value.(type) {
		case []string, []any:
			for _, item := range cast.To[[]string](value) {
				result.Add(key, item)
			}
		default:
			result.Set(key, cast.String(value))
		}
	}
	return result
}
