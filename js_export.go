package http

import (
	"time"

	"apigo.cc/go/jsmod"
)

func init() {
	jsmod.Register("http", map[string]any{
		// Static requests with default timeout (30s)
		"get": func(url string, headers ...string) *jsResult {
			return wrapResult(Get(url, headers...))
		},
		"post": func(url string, data any, headers ...string) *jsResult {
			return wrapResult(Post(url, data, headers...))
		},
		"put": func(url string, data any, headers ...string) *jsResult {
			return wrapResult(Put(url, data, headers...))
		},
		"delete": func(url string, data any, headers ...string) *jsResult {
			return wrapResult(Delete(url, data, headers...))
		},

		// Client creation
		"new": func(ms int) *jsClient {
			return &jsClient{c: NewClient(time.Duration(ms) * time.Millisecond)}
		},
		"newH2C": func(ms int) *jsClient {
			return &jsClient{c: NewClientH2C(time.Duration(ms) * time.Millisecond)}
		},

		// Data markers
		"form": func(data map[string]string) Form {
			return Form(data)
		},
		"multipart": func(data map[string]any) Multipart {
			return Multipart(data)
		},
	})
}

// jsClient wraps the internal Client to provide a JS-friendly interface
type jsClient struct {
	c *Client
}

func (jc *jsClient) Get(url string, headers ...string) *jsResult {
	return wrapResult(jc.c.Get(url, headers...))
}

func (jc *jsClient) Post(url string, data any, headers ...string) *jsResult {
	return wrapResult(jc.c.Post(url, data, headers...))
}

func (jc *jsClient) Put(url string, data any, headers ...string) *jsResult {
	return wrapResult(jc.c.Put(url, data, headers...))
}

func (jc *jsClient) Delete(url string, data any, headers ...string) *jsResult {
	return wrapResult(jc.c.Delete(url, data, headers...))
}

// jsResult wraps *Result to hide sensitive methods like Save()
type jsResult struct {
	r *Result
}

func wrapResult(r *Result) *jsResult {
	return &jsResult{r: r}
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
