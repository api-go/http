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
	"sync"
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
	globalHeaders    map[string]string
	headersMu        sync.RWMutex
	NoBody           bool
	Debug            bool
	DownloadPartSize int64
	MaxConnsPerHost  int
}

type Result struct {
	Error    error
	Response *http.Response
	data     []byte
}

type Form = map[string]string

var bufferPool = sync.Pool{
	New: func() any {
		return new(bytes.Buffer)
	},
}

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
		globalHeaders:    map[string]string{},
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
		globalHeaders:    map[string]string{},
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
	c.headersMu.Lock()
	defer c.headersMu.Unlock()
	if v == "" {
		delete(c.globalHeaders, k)
	} else {
		c.globalHeaders[k] = v
	}
}

func (c *Client) GetGlobalHeader(k string) string {
	c.headersMu.RLock()
	defer c.headersMu.RUnlock()
	return c.globalHeaders[k]
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
	headers := make([]string, 0, len(RelayHeaders)*2+len(settedHeaders)+4)

	// 续传指定的头
	for _, h := range RelayHeaders {
		if v := request.Header.Get(h); v != "" {
			headers = append(headers, h, v)
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
	headers = append(headers, HeaderForwardedFor, xForwardFor)

	// 处理请求唯一编号
	foundID := false
	for i := 1; i < len(headers); i += 2 {
		if headers[i-1] == HeaderRequestID {
			foundID = true
			break
		}
	}
	if !foundID {
		headers = append(headers, HeaderRequestID, string(encoding.Hex(rand.Bytes(16))))
	}

	headers = append(headers, settedHeaders...)

	if manualDo {
		return c.ManualDo(method, url, data, headers...)
	}
	return c.Do(method, url, data, headers...)
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

type offsetWriter struct {
	fp     *os.File
	offset int64
}

func (w *offsetWriter) Write(p []byte) (n int, err error) {
	n, err = w.fp.WriteAt(p, w.offset)
	w.offset += int64(n)
	return
}

func (c *Client) downloadPart(fp *os.File, task *downloadRange, url string, headers ...string) (int64, error) {
	partHeaders := make([]string, len(headers))
	copy(partHeaders, headers)
	partHeaders[len(partHeaders)-1] = fmt.Sprintf("bytes=%d-%d", task.Start, task.End)

	r := c.ManualDo("GET", url, nil, partHeaders...)
	if r.Error != nil {
		return 0, r.Error
	}
	defer r.Response.Body.Close()
	return io.Copy(&offsetWriter{fp: fp, offset: task.Start}, r.Response.Body)
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

		headers = append(headers, "Range", "")
		var finished int64
		var mu sync.Mutex
		var wg sync.WaitGroup
		errChan := make(chan error, len(tasks))

		// 限制并发度，默认为 4，可以通过 Client 设置
		concurrency := 4
		if c.MaxConnsPerHost > 0 {
			concurrency = c.MaxConnsPerHost
		}
		sem := make(chan struct{}, concurrency)

		for _, task := range tasks {
			wg.Add(1)
			go func(t downloadRange) {
				defer wg.Done()
				sem <- struct{}{}
				defer func() { <-sem }()

				n, err := c.downloadPart(fp, &t, url, headers...)
				if err != nil {
					// 重试一次
					n, err = c.downloadPart(fp, &t, url, headers...)
				}

				mu.Lock()
				finished += n
				if callback != nil {
					callback(t.Start, t.End, err == nil, finished, total)
				}
				mu.Unlock()

				if err != nil {
					errChan <- err
				}
			}(task)
		}

		wg.Wait()
		close(errChan)

		if len(errChan) > 0 {
			return nil, <-errChan
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

func (c *Client) PostMultipart(url string, formData map[string]string, files map[string]any, headers ...string) (*Result, []error) {
	errs := make([]error, 0)
	buf := bufferPool.Get().(*bytes.Buffer)
	buf.Reset()
	defer bufferPool.Put(buf)

	writer := multipart.NewWriter(buf)

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
				var dataBytes []byte
				switch t := v.(type) {
				case io.Reader:
					dataBytes, _ = io.ReadAll(t)
					h.Set("Content-Disposition", fmt.Sprintf(`form-data; name="%s"; filename="%s"`, k, k))
					h.Set("Content-Type", "application/octet-stream")
				case []byte:
					dataBytes = t
					h.Set("Content-Disposition", fmt.Sprintf(`form-data; name="%s"; filename="%s"`, k, k))
					h.Set("Content-Type", "application/octet-stream")
				case string:
					dataBytes = []byte(t)
					h.Set("Content-Disposition", fmt.Sprintf(`form-data; name="%s"; filename="%s.txt"`, k, k))
					h.Set("Content-Type", "text/plain")
				default:
					dataBytes = cast.MustJSONBytes(v)
					h.Set("Content-Disposition", fmt.Sprintf(`form-data; name="%s"; filename="%s.json"`, k, k))
					h.Set("Content-Type", "application/json")
				}

				if part, err := writer.CreatePart(h); err == nil {
					if _, err = part.Write(dataBytes); err != nil {
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
	r := c.Post(url, buf.Bytes(), headers...)
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

	c.headersMu.RLock()
	for k, v := range c.globalHeaders {
		req.Header.Set(k, v)
	}
	c.headersMu.RUnlock()

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

func (rs *Result) Slice() []any {
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

// Bind 使用泛型获取结果
func Bind[T any](rs *Result) (T, error) {
	var v T
	err := rs.To(&v)
	return v, err
}

