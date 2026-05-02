package http

import (
	"bytes"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net"
	"net/http"
	"net/http/cookiejar"
	"net/textproto"
	url2 "net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"apigo.cc/go/cast"
	"apigo.cc/go/convert"
	"apigo.cc/go/encoding"
	"apigo.cc/go/file"
	"apigo.cc/go/log"
	"apigo.cc/go/rand"
	"golang.org/x/net/http2"
)

type Client struct {
	pool             *http.Client
	GlobalHeaders    map[string]string
	NoBody           bool
	Debug            bool
	DownloadPartSize int64
}

type Result struct {
	Error    error
	Response *http.Response
	data     []byte
}

type Form = map[string]string

func NewClient(timeout time.Duration) *Client {
	if timeout < time.Millisecond && timeout > 0 {
		timeout *= time.Millisecond
	}
	jar, _ := cookiejar.New(nil)
	return &Client{
		pool: &http.Client{
			Timeout: timeout,
			CheckRedirect: func(req *http.Request, via []*http.Request) error {
				return http.ErrUseLastResponse
			},
			Jar: jar,
		},
		GlobalHeaders:    map[string]string{},
		DownloadPartSize: 4194304,
	}
}

func NewClientH2C(timeout time.Duration) *Client {
	if timeout < time.Millisecond && timeout > 0 {
		timeout *= time.Millisecond
	}
	jar, _ := cookiejar.New(nil)
	clientConfig := &http.Client{
		Transport: &http2.Transport{
			AllowHTTP: true,
			DialTLS: func(network, addr string, cfg *tls.Config) (net.Conn, error) {
				return net.Dial(network, addr)
			},
		},
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
		Timeout: timeout,
		Jar:     jar,
	}
	return &Client{
		pool:             clientConfig,
		GlobalHeaders:    map[string]string{},
		DownloadPartSize: 4194304,
	}
}

func (c *Client) GetRawClient() *http.Client {
	return c.pool
}

func (c *Client) EnableRedirect() {
	c.pool.CheckRedirect = nil
}

func (c *Client) SetGlobalHeader(k, v string) {
	if v == "" {
		delete(c.GlobalHeaders, k)
	} else {
		c.GlobalHeaders[k] = v
	}
}

func (c *Client) Destroy() {
	if c.pool != nil {
		c.pool.CloseIdleConnections()
		c.pool = nil
	}
}

func (c *Client) Get(url string, headers ...string) *Result {
	return c.Do("GET", url, nil, headers...)
}

func (c *Client) Post(url string, data any, headers ...string) *Result {
	return c.Do("POST", url, data, headers...)
}

func (c *Client) Put(url string, data any, headers ...string) *Result {
	return c.Do("PUT", url, data, headers...)
}

func (c *Client) Delete(url string, data any, headers ...string) *Result {
	return c.Do("DELETE", url, data, headers...)
}

func (c *Client) Head(url string, headers ...string) *Result {
	return c.Do("HEAD", url, nil, headers...)
}

func (c *Client) DoByRequest(request *http.Request, method, url string, data any, settedHeaders ...string) *Result {
	return c.doByRequest(false, request, method, url, data, settedHeaders...)
}

func (c *Client) ManualDoByRequest(request *http.Request, method, url string, data any, settedHeaders ...string) *Result {
	return c.doByRequest(true, request, method, url, data, settedHeaders...)
}

func (c *Client) doByRequest(manualDo bool, request *http.Request, method, url string, data any, settedHeaders ...string) *Result {
	headers := map[string]string{}

	// 续传指定的头
	for _, h := range RelayHeaders {
		if v := request.Header.Get(h); v != "" {
			headers[h] = v
		}
	}

	// 续传 X-Forwarded-For
	xForwardFor := request.Header.Get(HeaderForwardedFor)
	remoteIP, _, _ := net.SplitHostPort(request.RemoteAddr)
	if remoteIP == "" {
		remoteIP = request.RemoteAddr
	}
	if xForwardFor != "" {
		xForwardFor = remoteIP + ", " + xForwardFor
	} else {
		xForwardFor = remoteIP
	}
	headers[HeaderForwardedFor] = xForwardFor

	// 处理请求唯一编号
	if headers[HeaderRequestID] == "" {
		headers[HeaderRequestID] = string(encoding.Hex(rand.Bytes(16)))
	}

	for i := 1; i < len(settedHeaders); i += 2 {
		headers[settedHeaders[i-1]] = settedHeaders[i]
	}

	headerArgs := make([]string, 0, len(headers)*2)
	for k, v := range headers {
		headerArgs = append(headerArgs, k, v)
	}

	if manualDo {
		return c.ManualDo(method, url, data, headerArgs...)
	}
	return c.Do(method, url, data, headerArgs...)
}

func (c *Client) Do(method, url string, data any, headers ...string) *Result {
	return c.do(true, method, url, data, headers...)
}

func (c *Client) ManualDo(method, url string, data any, headers ...string) *Result {
	return c.do(false, method, url, data, headers...)
}

type downloadRange struct {
	Start int64
	End   int64
}

func (c *Client) downloadPart(fp *os.File, task *downloadRange, url string, headers ...string) (int64, error) {
	headers[len(headers)-1] = fmt.Sprintf("bytes=%d-%d", task.Start, task.End)
	r := c.ManualDo("GET", url, nil, headers...)
	if r.Error != nil {
		return 0, r.Error
	}
	defer r.Response.Body.Close()
	return io.Copy(fp, r.Response.Body)
}

func (c *Client) Download(filename, url string, callback func(start, end int64, ok bool, finished, total int64), headers ...string) (*Result, error) {
	r1 := c.Head(url, headers...)
	if r1.Error != nil {
		return r1, r1.Error
	}
	total := r1.Response.ContentLength
	if total > 0 {
		tasks := make([]downloadRange, 0)
		for i := int64(0); i < total; i += c.DownloadPartSize {
			end := i + c.DownloadPartSize - 1
			if end >= total {
				end = total - 1
			}
			tasks = append(tasks, downloadRange{i, end})
		}

		file.EnsureParentDir(filename)
		fp, err := os.OpenFile(filename, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0600)
		if err != nil {
			return nil, err
		}
		defer fp.Close()

		finished := int64(0)
		headers = append(headers, "Range", "")
		for _, task := range tasks {
			n, err := c.downloadPart(fp, &task, url, headers...)
			finished += n
			if callback != nil {
				callback(task.Start, task.End, err == nil, finished, total)
			}
			// 简单的重试逻辑
			if err != nil {
				n, err = c.downloadPart(fp, &task, url, headers...)
				if err == nil {
					finished += n
				}
				if callback != nil {
					callback(task.Start, task.End, err == nil, finished, total)
				}
			}
		}

		if finished < total {
			return nil, errors.New("download file failed: incomplete")
		}
		return r1, nil
	}

	r := c.ManualDo("GET", url, nil, headers...)
	if r.Error != nil {
		return r, r.Error
	}
	defer r.Response.Body.Close()
	file.EnsureParentDir(filename)
	fp, err := os.OpenFile(filename, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0600)
	if err != nil {
		return r, err
	}
	defer fp.Close()
	_, err = io.Copy(fp, r.Response.Body)
	return r, err
}

func (c *Client) MPost(url string, formData map[string]string, files map[string]any, headers ...string) (*Result, []error) {
	errs := make([]error, 0)
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)

	if formData != nil {
		for k, v := range formData {
			if err := writer.WriteField(k, v); err != nil {
				errs = append(errs, err)
			}
		}
	}

	if files != nil {
		for k, v := range files {
			if filename, ok := v.(string); ok && file.Exists(filename) {
				if fp, err := os.Open(filename); err == nil {
					if part, err := writer.CreateFormFile(k, filepath.Base(filename)); err == nil {
						if _, err = io.Copy(part, fp); err != nil {
							errs = append(errs, err)
						}
					} else {
						errs = append(errs, err)
					}
					_ = fp.Close()
				} else {
					errs = append(errs, err)
				}
			} else {
				h := make(textproto.MIMEHeader)
				var buf []byte
				switch t := v.(type) {
				case io.Reader:
					buf, _ = io.ReadAll(t)
					h.Set("Content-Disposition", fmt.Sprintf(`form-data; name="%s"; filename="%s"`, k, k))
					h.Set("Content-Type", "application/octet-stream")
				case []byte:
					buf = t
					h.Set("Content-Disposition", fmt.Sprintf(`form-data; name="%s"; filename="%s"`, k, k))
					h.Set("Content-Type", "application/octet-stream")
				case string:
					buf = []byte(t)
					h.Set("Content-Disposition", fmt.Sprintf(`form-data; name="%s"; filename="%s.txt"`, k, k))
					h.Set("Content-Type", "text/plain")
				default:
					buf = cast.MustJSONBytes(v)
					h.Set("Content-Disposition", fmt.Sprintf(`form-data; name="%s"; filename="%s.json"`, k, k))
					h.Set("Content-Type", "application/json")
				}

				if part, err := writer.CreatePart(h); err == nil {
					if _, err = part.Write(buf); err != nil {
						errs = append(errs, err)
					}
				} else {
					errs = append(errs, err)
				}
			}
		}
	}

	if err := writer.Close(); err != nil {
		errs = append(errs, err)
	}

	if len(errs) > 0 {
		return nil, errs
	}

	headers = append(headers, "Content-Type", writer.FormDataContentType())
	r := c.Post(url, body, headers...)
	if r.Error != nil {
		errs = append(errs, r.Error)
	}
	return r, errs
}

func (c *Client) do(fetchBody bool, method, url string, data any, headers ...string) *Result {
	var req *http.Request
	var err error
	contentType := ""
	contentLength := 0
	var reader io.Reader

	if data != nil {
		switch t := data.(type) {
		case io.Reader:
			reader = t
		case []byte:
			reader = bytes.NewReader(t)
			contentLength = len(t)
		case string:
			reader = strings.NewReader(t)
			contentLength = len(t)
		case url2.Values:
			encoded := t.Encode()
			reader = strings.NewReader(encoded)
			contentType = "application/x-www-form-urlencoded"
			contentLength = len(encoded)
		case map[string][]string:
			values := url2.Values(t)
			encoded := values.Encode()
			reader = strings.NewReader(encoded)
			contentType = "application/x-www-form-urlencoded"
			contentLength = len(encoded)
		case map[string]string:
			values := url2.Values{}
			for k, v := range t {
				values.Set(k, v)
			}
			encoded := values.Encode()
			reader = strings.NewReader(encoded)
			contentType = "application/x-www-form-urlencoded"
			contentLength = len(encoded)
		default:
			bytesData, _ := cast.ToJSONBytes(data)
			if len(bytesData) > 0 && string(bytesData) != "null" {
				reader = bytes.NewReader(bytesData)
				contentType = "application/json"
				contentLength = len(bytesData)
			}
		}
	}

	req, err = http.NewRequest(method, url, reader)
	if err != nil {
		return &Result{Error: err}
	}

	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	if contentLength > 0 {
		req.Header.Set("Content-Length", cast.String(contentLength))
	}

	for i := 1; i < len(headers); i += 2 {
		if headers[i-1] == "Host" {
			req.Host = headers[i]
		} else {
			req.Header.Set(headers[i-1], headers[i])
		}
	}

	for k, v := range c.GlobalHeaders {
		req.Header.Set(k, v)
	}

	if c.Debug {
		log.DefaultLogger.Info("http request", "method", req.Method, "url", req.URL.String(), "headers", req.Header)
	}

	res, err := c.pool.Do(req)
	if err != nil {
		return &Result{Error: err}
	}

	if res.ContentLength == -1 {
		res.ContentLength = cast.Int64(res.Header.Get("Content-Length"))
	}

	if !fetchBody || c.NoBody {
		return &Result{Response: res}
	}

	defer res.Body.Close()
	bodyBytes, err := io.ReadAll(res.Body)
	if err != nil {
		return &Result{Error: err, Response: res}
	}

	if c.Debug {
		log.DefaultLogger.Info("http response", "status", res.StatusCode, "len", len(bodyBytes))
	}

	return &Result{data: bodyBytes, Response: res}
}

func (rs *Result) Save(filename string) error {
	file.EnsureParentDir(filename)
	if rs.data != nil {
		return file.WriteBytes(filename, rs.data)
	}
	if rs.Response != nil && rs.Response.Body != nil {
		defer rs.Response.Body.Close()
		fp, err := os.OpenFile(filename, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0600)
		if err != nil {
			return err
		}
		defer fp.Close()
		_, err = io.Copy(fp, rs.Response.Body)
		return err
	}
	return errors.New("no data to save")
}

func (rs *Result) String() string {
	return string(rs.data)
}

func (rs *Result) Bytes() []byte {
	return rs.data
}

func (rs *Result) Map() map[string]any {
	var m map[string]any
	_ = rs.To(&m)
	return m
}

func (rs *Result) Arr() []any {
	var a []any
	_ = rs.To(&a)
	return a
}

func (rs *Result) To(v any) error {
	if rs.data == nil {
		return errors.New("no data")
	}
	_, err := cast.UnmarshalJSONBytes(rs.data, v)
	if err != nil {
		// 如果 cast 直接解不出来，尝试通过 convert 做深度映射（处理 struct 字段匹配等）
		var tmp any
		if _, err2 := cast.UnmarshalJSONBytes(rs.data, &tmp); err2 == nil {
			convert.To(tmp, v)
			return nil
		}
	}
	return err
}

// ToT 使用泛型获取结果
func ToT[T any](rs *Result) (T, error) {
	var v T
	err := rs.To(&v)
	return v, err
}
