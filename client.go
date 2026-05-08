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

type Form map[string]string
type Multipart map[string]any

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

func (client *Client) GetRawClient() *http.Client {
	return client.pool
}

func (client *Client) EnableRedirect() {
	client.pool.CheckRedirect = nil
}

func (client *Client) SetGlobalHeader(key, value string) {
	client.headersMu.Lock()
	defer client.headersMu.Unlock()
	if value == "" {
		delete(client.globalHeaders, key)
	} else {
		client.globalHeaders[key] = value
	}
}

func (client *Client) GetGlobalHeader(key string) string {
	client.headersMu.RLock()
	defer client.headersMu.RUnlock()
	return client.globalHeaders[key]
}

func (client *Client) Destroy() {
	if client.pool != nil {
		client.pool.CloseIdleConnections()
		client.pool = nil
	}
}

func (client *Client) Get(url string, headers ...string) *Result {
	return client.Do("GET", url, nil, headers...)
}

func (client *Client) Post(url string, data any, headers ...string) *Result {
	return client.Do("POST", url, data, headers...)
}

func (client *Client) Put(url string, data any, headers ...string) *Result {
	return client.Do("PUT", url, data, headers...)
}

func (client *Client) Delete(url string, data any, headers ...string) *Result {
	return client.Do("DELETE", url, data, headers...)
}

func (client *Client) Head(url string, headers ...string) *Result {
	return client.Do("HEAD", url, nil, headers...)
}

func (client *Client) DoByRequest(request *http.Request, method, url string, data any, settedHeaders ...string) *Result {
	return client.doByRequest(false, request, method, url, data, settedHeaders...)
}

func (client *Client) ManualDoByRequest(request *http.Request, method, url string, data any, settedHeaders ...string) *Result {
	return client.doByRequest(true, request, method, url, data, settedHeaders...)
}

func (client *Client) doByRequest(manualDo bool, request *http.Request, method, url string, data any, settedHeaders ...string) *Result {
	headers := make([]string, 0, len(RelayHeaders)*2+len(settedHeaders)+4)

	// 续传指定的头
	for _, headerName := range RelayHeaders {
		if value := request.Header.Get(headerName); value != "" {
			headers = append(headers, headerName, value)
		}
	}

	// 确保 Request-ID 存在
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

	// 续传 X-Forwarded-For
	xForwardFor := request.Header.Get(HeaderForwardedFor)
	remoteIP, _, err := net.SplitHostPort(request.RemoteAddr)
	if err != nil {
		remoteIP = request.RemoteAddr
	}

	if xForwardFor != "" {
		xForwardFor = remoteIP + ", " + xForwardFor
	} else {
		xForwardFor = remoteIP
	}
	headers = append(headers, HeaderForwardedFor, xForwardFor)

	headers = append(headers, settedHeaders...)

	if manualDo {
		return client.ManualDo(method, url, data, headers...)
	}
	return client.Do(method, url, data, headers...)
}

func (client *Client) Do(method, url string, data any, headers ...string) *Result {
	return client.do(true, method, url, data, headers...)
}

func (client *Client) ManualDo(method, url string, data any, headers ...string) *Result {
	return client.do(false, method, url, data, headers...)
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

func (client *Client) downloadPart(fp *os.File, task *downloadRange, url string, headers ...string) (int64, error) {
	partHeaders := make([]string, len(headers))
	copy(partHeaders, headers)
	partHeaders[len(partHeaders)-1] = fmt.Sprintf("bytes=%d-%d", task.Start, task.End)

	result := client.ManualDo("GET", url, nil, partHeaders...)
	if result.Error != nil {
		return 0, result.Error
	}
	defer result.Response.Body.Close()
	return io.Copy(&offsetWriter{fp: fp, offset: task.Start}, result.Response.Body)
}

func (client *Client) Download(filename, url string, callback func(start, end int64, ok bool, finished, total int64), headers ...string) (*Result, error) {
	resultHead := client.Head(url, headers...)
	if resultHead.Error != nil {
		return resultHead, resultHead.Error
	}
	total := resultHead.Response.ContentLength
	if total > 0 {
		tasks := make([]downloadRange, 0)
		for i := int64(0); i < total; i += client.DownloadPartSize {
			end := i + client.DownloadPartSize - 1
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
		if client.MaxConnsPerHost > 0 {
			concurrency = client.MaxConnsPerHost
		}
		sem := make(chan struct{}, concurrency)

		for _, task := range tasks {
			wg.Add(1)
			go func(t downloadRange) {
				defer wg.Done()
				sem <- struct{}{}
				defer func() { <-sem }()

				n, err := client.downloadPart(fp, &t, url, headers...)
				if err != nil {
					// 重试一次
					n, err = client.downloadPart(fp, &t, url, headers...)
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
		return resultHead, nil
	}

	result := client.ManualDo("GET", url, nil, headers...)
	if result.Error != nil {
		return result, result.Error
	}
	defer result.Response.Body.Close()
	file.EnsureParentDir(filename)
	fp, err := os.OpenFile(filename, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0600)
	if err != nil {
		return result, err
	}
	defer fp.Close()
	_, err = io.Copy(fp, result.Response.Body)
	return result, err
}

func (client *Client) buildMultipart(writer *multipart.Writer, data map[string]any) []error {
	errs := make([]error, 0)
	for key, value := range data {
		if filename, ok := value.(string); ok && file.Exists(filename) {
			var r io.Reader
			var closer io.Closer
			if mf := file.ReadFileFromMemory(filename); mf != nil {
				r = bytes.NewReader(mf.GetData())
			} else {
				if fp, err := os.Open(filename); err == nil {
					r = fp
					closer = fp
				} else {
					errs = append(errs, err)
					continue
				}
			}

			if part, err := writer.CreateFormFile(key, filepath.Base(filename)); err == nil {
				if _, err = io.Copy(part, r); err != nil {
					errs = append(errs, err)
				}
			} else {
				errs = append(errs, err)
			}

			if closer != nil {
				_ = closer.Close()
			}
		} else {
			var dataBytes []byte
			h := make(textproto.MIMEHeader)
			isField := false
			switch t := value.(type) {
			case io.Reader:
				dataBytes, _ = io.ReadAll(t)
				h.Set("Content-Disposition", fmt.Sprintf(`form-data; name="%s"; filename="%s"`, key, key))
				h.Set("Content-Type", "application/octet-stream")
			case []byte:
				dataBytes = t
				h.Set("Content-Disposition", fmt.Sprintf(`form-data; name="%s"; filename="%s"`, key, key))
				h.Set("Content-Type", "application/octet-stream")
			case string:
				isField = true
				dataBytes = []byte(t)
			default:
				isField = true
				dataBytes, _ = cast.ToJSONBytes(value)
			}

			if isField {
				if err := writer.WriteField(key, string(dataBytes)); err != nil {
					errs = append(errs, err)
				}
			} else {
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
	return errs
}

func (client *Client) do(fetchBody bool, method, url string, data any, headers ...string) *Result {
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
		case Form:
			values := url2.Values{}
			for k, v := range t {
				values.Set(k, v)
			}
			encoded := values.Encode()
			reader = strings.NewReader(encoded)
			contentType = "application/x-www-form-urlencoded"
			contentLength = len(encoded)
		case Multipart:
			buf := bufferPool.Get().(*bytes.Buffer)
			buf.Reset()
			defer bufferPool.Put(buf)
			writer := multipart.NewWriter(buf)
			errs := client.buildMultipart(writer, t)
			if err := writer.Close(); err != nil {
				errs = append(errs, err)
			}
			if len(errs) > 0 {
				return &Result{Error: errors.Join(errs...)}
			}
			bytesData := buf.Bytes()
			reader = bytes.NewReader(bytesData)
			contentType = writer.FormDataContentType()
			contentLength = len(bytesData)
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

	client.headersMu.RLock()
	for k, v := range client.globalHeaders {
		req.Header.Set(k, v)
	}
	client.headersMu.RUnlock()

	if client.Debug {
		log.DefaultLogger.Info("http request", "method", req.Method, "url", req.URL.String(), "headers", req.Header)
	}

	res, err := client.pool.Do(req)
	if err != nil {
		return &Result{Error: err}
	}

	if res.ContentLength == -1 {
		res.ContentLength = cast.Int64(res.Header.Get("Content-Length"))
	}

	if !fetchBody || client.NoBody {
		return &Result{Response: res}
	}

	defer res.Body.Close()
	bodyBytes, err := io.ReadAll(res.Body)
	if err != nil {
		return &Result{Error: err, Response: res}
	}

	if client.Debug {
		log.DefaultLogger.Info("http response", "status", res.StatusCode, "len", len(bodyBytes))
	}

	return &Result{data: bodyBytes, Response: res}
}

func (result *Result) Save(filename string) error {
	file.EnsureParentDir(filename)
	if result.data != nil {
		return file.WriteBytes(filename, result.data)
	}
	if result.Response != nil && result.Response.Body != nil {
		defer result.Response.Body.Close()
		fp, err := os.OpenFile(filename, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0600)
		if err != nil {
			return err
		}
		defer fp.Close()
		_, err = io.Copy(fp, result.Response.Body)
		return err
	}
	return errors.New("no data to save")
}

func (result *Result) String() string {
	return string(result.data)
}

func (result *Result) Bytes() []byte {
	return result.data
}

func (result *Result) Map() map[string]any {
	var m map[string]any
	_ = result.To(&m)
	return m
}

func (result *Result) Slice() []any {
	var a []any
	_ = result.To(&a)
	return a
}

func (result *Result) To(v any) error {
	if len(result.data) == 0 {
		return errors.New("no data")
	}
	return cast.UnmarshalJSON(result.data, v)
}

// To 使用泛型获取结果
func To[T any](result *Result) (T, error) {
	var v T
	err := result.To(&v)
	return v, err
}
