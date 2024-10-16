package req

import (
    "io"
    "net/http"
    "os"
)

// Check 检查文件
func Check(url string) (bool, error) {
    resp, err := http.Head(url)
    if err != nil {
        return false, err
    }

    defer resp.Body.Close()
    return resp.StatusCode == http.StatusOK, nil
}

// Download 下载文件
func Download(url string, fileName string) error {
    resp, err := http.Get(url)
    if err != nil {
        return err
    }
    defer resp.Body.Close()

    file, err := os.Create(fileName)
    if err != nil {
        return err
    }
    defer file.Close()

    _, err = io.Copy(file, resp.Body)
    if err != nil {
        return err
    }

    return nil
}
