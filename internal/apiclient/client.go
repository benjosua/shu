package apiclient

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
)

func APIBase() string   { return getenv("SHU_SERVER", "http://localhost:8090") }
func Workspace() string { return os.Getenv("SHU_WORKSPACE") }
func Token() string     { return os.Getenv("SHU_TOKEN") }

func Request(method, path string, body any) ([]byte, error) {
	var rd io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		rd = bytes.NewReader(b)
	}
	req, err := http.NewRequest(method, APIBase()+path, rd)
	if err != nil {
		return nil, err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if Token() != "" {
		req.Header.Set("Authorization", "Bearer "+Token())
	}
	if Workspace() != "" {
		req.Header.Set("X-Workspace", Workspace())
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 300 {
		return b, fmt.Errorf("%s %s: %s", method, path, string(b))
	}
	return b, nil
}

func getenv(k, d string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return d
}
