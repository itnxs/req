package req

import (
	"context"
	"crypto/md5"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"time"

	"github.com/chromedp/chromedp"
	"github.com/imroc/req"
	jsoniter "github.com/json-iterator/go"
	"github.com/pkg/errors"
	"golang.org/x/sync/errgroup"
)

var (
    // defaultLimit 并发数量
    defaultLimit = 10
    // defaultTimeout 超时时间
    defaultTimeout = time.Second * 10
    // defaultCachePath 文件缓存路径
    defaultCachePath = ""
    // defaultRetryCount 重试次数
    defaultRetryCount = 3
    // defaultRetrySleep 重试暂停时长
    defaultRetrySleepTime = time.Millisecond * 200
)

func init() {
    req.SetTimeout(defaultTimeout)
}

// SetLimit 设置并发数量
func SetLimit(limit int) {
    defaultLimit = limit
}

// SetTimeout 设置超时时间
func SetTimeout(timeout time.Duration) {
    defaultTimeout = timeout
    req.SetTimeout(defaultTimeout)
}

// SetRetryCount 设置重试次数
func SetRetryCount(retryCount int) {
    defaultRetryCount = retryCount
}

// SetRetrySleepTime 设置重试暂停时长
func SetRetrySleepTime(sleep time.Duration) {
    defaultRetrySleepTime = sleep
}

// SetCachePath 设置缓存目录
func SetCachePath(dir string) {
    path, err := filepath.Abs(dir)
    if err != nil {
        panic(errors.WithStack(err))
    }

    if !fileExist(path) {
        err = os.MkdirAll(path, os.ModePerm)
        if err != nil {
            panic(errors.WithStack(err))
        }
    }

    defaultCachePath = path
}

// cacheName 缓存名称
func cacheName(method, url string, v ...interface{}) string {
    if defaultCachePath != "" {
        var args string
        if len(v) > 0 {
            args, _ = jsoniter.MarshalToString(v)
        }
        return fmt.Sprintf("%s/.%s.%s.cache", defaultCachePath, md5sum([]byte(url+args)), method)
    }
    return ""
}

func doRequest(method, url string, retryCount int, v ...interface{}) (string, error) {
    name := cacheName(method, url, v...)
    if name != "" && fileExist(name) {
        data, err := os.ReadFile(name)
        return string(data), errors.WithStack(err)
    }

    rep, err := req.Do(method, url, v...)
    if err != nil {
        return "", errors.WithStack(err)
    } else if rep.Response().StatusCode != 200 {
        if retryCount < defaultRetryCount {
            retryCount++
            time.Sleep(defaultRetrySleepTime)
            return doRequest(method, url, retryCount, v...)
        }
        return "", errors.WithStack(fmt.Errorf("http status code: %d", rep.Response().StatusCode))
    }

    body := rep.Bytes()
    if name != "" {
        err = os.WriteFile(name, body, os.ModePerm)
        if err != nil {
            return "", errors.WithStack(err)
        }
    }

    return string(body), nil
}

// Get GET请求内容
func Get(url string, v ...interface{}) (string, error) {
    return doRequest(http.MethodGet, url, 0, v...)
}

// Post POST请求内容
func Post(url string, v ...interface{}) (string, error) {
    return doRequest(http.MethodPost, url, 0, v...)
}

// BatchGet 批量请求内容
func BatchGet(urls []string, v ...interface{}) (resMap, errMap map[int]string, err error) {
    var (
        group    errgroup.Group
        resMutex sync.Mutex
        errMutex sync.Mutex
    )

    resMap = make(map[int]string)
    errMap = make(map[int]string)

    group.SetLimit(defaultLimit)
    for i, url := range urls {
        i := i
        url := url
        group.Go(func() error {
            body, err := Get(url, v...)
            if err != nil {
                errMutex.Lock()
                resMap[i] = url
                errMutex.Unlock()
            } else {
                resMutex.Lock()
                resMap[i] = body
                resMutex.Unlock()
            }
            return nil
        })
    }

    err = group.Wait()
    return resMap, errMap, errors.WithStack(err)
}

// ChromeGet 模拟Chrome访问
func ChromeGet(ctx context.Context, url string) (string, error) {
    name := cacheName(http.MethodGet, url)
    if name != "" && fileExist(name) {
        if data, err := os.ReadFile(name); err == nil && len(data) > 0 {
            return string(data), nil
        }
    }

    ctx, cancel := chromedp.NewContext(ctx)
    defer cancel()

    var body string
    err := chromedp.Run(ctx,
        chromedp.Navigate(url),
        chromedp.OuterHTML(`body`, &body, chromedp.NodeVisible),
    )
    if err != nil {
        return "", errors.WithStack(err)
    }

    if name != "" {
        err = os.WriteFile(name, []byte(body), os.ModePerm)
        if err != nil {
            return "", errors.WithStack(err)
        }
    }

    return body, nil
}

// CurlGet 模拟CURL请求
func CurlGet(url string, headers ...req.Header) (string, error) {
    name := cacheName(http.MethodGet, url)
    if name != "" && fileExist(name) {
        if data, err := os.ReadFile(name); err == nil && len(data) > 0 {
            return string(data), nil
        }
    }

    var header req.Header
    if len(headers) > 0 {
        header = headers[0]
    }

    args := []string{url}
    for k, v := range header {
        args = append(args, "-H", fmt.Sprintf("%s: %v", k, v))
    }

    cmd := exec.Command("curl", args...)
    output, err := cmd.Output()
    if err != nil {
        return "", errors.WithStack(err)
    }

    if name != "" {
        err := os.WriteFile(name, output, os.ModePerm)
        if err != nil {
            return "", errors.WithStack(err)
        }
    }

    return string(output), nil
}

// RemoveCache 删除缓存文件
func RemoveCache(url string) error {
    name := cacheName(http.MethodGet, url)
    return fileRemove(name)
}

// fileExist 是否存在
// name 文件或则目录名称
func fileExist(name string) bool {
    _, err := os.Stat(name)
    if err == nil {
        return true
    }
    if os.IsNotExist(err) {
        return false
    }
    return false
}

// fileRemove 删除文件
func fileRemove(name string) error {
    if len(name) > 0 && fileExist(name) {
        return os.Remove(name)
    }
    return nil
}

// md5sum MD5值计算
func md5sum(v []byte) string {
    return fmt.Sprintf("%x", md5.Sum(v))
}
