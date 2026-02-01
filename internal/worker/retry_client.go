package worker

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"time"
)

const maxResponseBodyBytes = 64 * 1024

type RetryConfig struct {
	MaxRetries int
	Backoff    time.Duration
	MaxBackoff time.Duration
}

type RetryHTTPClient struct {
	ctx         context.Context
	baseURL     string
	httpClient  *http.Client
	config      RetryConfig
	workerToken string
}

func NewRetryHTTPClient(ctx context.Context, baseURL string, httpClient *http.Client, config RetryConfig) *RetryHTTPClient {
	return &RetryHTTPClient{
		ctx:        ctx,
		baseURL:    baseURL,
		httpClient: httpClient,
		config:     config,
	}
}

func (c *RetryHTTPClient) SetWorkerToken(token string) {
	c.workerToken = token
}

func (c *RetryHTTPClient) Post(path string, body interface{}) (*http.Response, error) {
	url := c.baseURL + path

	var jsonBytes []byte
	if body != nil {
		var err error
		jsonBytes, err = json.Marshal(body)
		if err != nil {
			return nil, err
		}
	}

	req, err := http.NewRequestWithContext(c.ctx, http.MethodPost, url, bytes.NewReader(jsonBytes))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	if c.workerToken != "" {
		req.Header.Set("X-Worker-Token", c.workerToken)
	}
	req.GetBody = func() (io.ReadCloser, error) {
		return io.NopCloser(bytes.NewReader(jsonBytes)), nil
	}
	return c.Do(req)
}

func (c *RetryHTTPClient) Do(req *http.Request) (*http.Response, error) {
	var lastErr error
	backoff := c.config.Backoff
	if c.workerToken != "" && req.Header.Get("X-Worker-Token") == "" {
		req.Header.Set("X-Worker-Token", c.workerToken)
	}

	for attempt := 0; attempt <= c.config.MaxRetries; attempt++ {
		if attempt > 0 {
			select {
			case <-c.ctx.Done():
				return nil, c.ctx.Err()
			case <-time.After(backoff):
				backoff *= 2
				if backoff > c.config.MaxBackoff {
					backoff = c.config.MaxBackoff
				}
			}
			if req.GetBody != nil {
				body, err := req.GetBody()
				if err != nil {
					lastErr = err
					continue
				}
				req.Body = body
			}
		}

		resp, err := c.httpClient.Do(req)
		if err != nil {
			lastErr = err
			continue
		}

		if resp.StatusCode >= 500 {
			lastErr = &RetryableError{StatusCode: resp.StatusCode}
			resp.Body.Close()
			continue
		}

		return resp, nil
	}

	return nil, lastErr
}

func (c *RetryHTTPClient) BaseURL() string {
	return c.baseURL
}

type RetryableError struct {
	StatusCode int
}

func (e *RetryableError) Error() string {
	return "retryable error"
}

func ReadResponseBody(resp *http.Response) ([]byte, error) {
	if resp == nil || resp.Body == nil {
		return nil, nil
	}
	defer resp.Body.Close()
	limited := io.LimitReader(resp.Body, maxResponseBodyBytes+1)
	body, err := io.ReadAll(limited)
	if err != nil {
		return nil, err
	}
	if len(body) > maxResponseBodyBytes {
		log.Printf("[RetryHTTPClient] Response body truncated to %d bytes", maxResponseBodyBytes)
		body = body[:maxResponseBodyBytes]
	}
	return body, nil
}
