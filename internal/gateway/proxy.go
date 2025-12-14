package gateway

import (
	"context"
	"net/http"
)

type ServiceProxy struct {
	baseURL string
	client  *http.Client
}

func NewServiceProxy(baseURL string, client *http.Client) *ServiceProxy {
	return &ServiceProxy{
		baseURL: baseURL,
		client:  client,
	}
}

func (p *ServiceProxy) ForwardRequest(ctx context.Context, r *http.Request, path string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, r.Method, p.baseURL+path, r.Body)
	if err != nil {
		return nil, err
	}

	if contentType := r.Header.Get("Content-Type"); contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}

	return p.client.Do(req)
}
