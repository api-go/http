package http

import (
	"context"
	"time"

	"apigo.cc/go/file"
	"apigo.cc/go/jsmod"
)

func init() {
	jsmod.Register("http", map[string]any{
		// Static requests with default timeout (30s)
		"GET": func(ctx context.Context, url string, headers map[string]string) *jsResult {
			return wrapResult(ctx, Get(url, flattenHeaders(headers)...))
		},
		"POST": func(ctx context.Context, url string, data any, headers map[string]string) (*jsResult, error) {
			if err := verifyMultipart(ctx, data); err != nil {
				return nil, err
			}
			return wrapResult(ctx, Post(url, data, flattenHeaders(headers)...)), nil
		},
		"PUT": func(ctx context.Context, url string, data any, headers map[string]string) (*jsResult, error) {
			if err := verifyMultipart(ctx, data); err != nil {
				return nil, err
			}
			return wrapResult(ctx, Put(url, data, flattenHeaders(headers)...)), nil
		},
		"DELETE": func(ctx context.Context, url string, data any, headers map[string]string) *jsResult {
			return wrapResult(ctx, Delete(url, data, flattenHeaders(headers)...))
		},

		// Client creation
		"New": func(ms int) *jsClient {
			return &jsClient{c: NewClient(time.Duration(ms) * time.Millisecond)}
		},
		"NewH2C": func(ms int) *jsClient {
			return &jsClient{c: NewClientH2C(time.Duration(ms) * time.Millisecond)}
		},

		// Data markers
		"Form": func(data map[string]string) Form {
			return Form(data)
		},
		"Multipart": func(data map[string]any) Multipart {
			return Multipart(data)
		},
	})
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

func (jc *jsClient) GET(ctx context.Context, url string, headers map[string]string) *jsResult {
	return wrapResult(ctx, jc.c.Get(url, flattenHeaders(headers)...))
}

func (jc *jsClient) POST(ctx context.Context, url string, data any, headers map[string]string) (*jsResult, error) {
	if err := verifyMultipart(ctx, data); err != nil {
		return nil, err
	}
	return wrapResult(ctx, jc.c.Post(url, data, flattenHeaders(headers)...)), nil
}

func (jc *jsClient) PUT(ctx context.Context, url string, data any, headers map[string]string) (*jsResult, error) {
	if err := verifyMultipart(ctx, data); err != nil {
		return nil, err
	}
	return wrapResult(ctx, jc.c.Put(url, data, flattenHeaders(headers)...)), nil
}

func (jc *jsClient) DELETE(ctx context.Context, url string, data any, headers map[string]string) *jsResult {
	return wrapResult(ctx, jc.c.Delete(url, data, flattenHeaders(headers)...))
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
		return err
	}
	return jr.r.Save(p)
}
