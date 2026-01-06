package imagestreamimport

import (
	"context"
	"errors"
	"net/http"
	"net/netip"
	"strings"
	"testing"
)

// mockRoundTripper is a mock implementation of http.RoundTripper for testing
type mockRoundTripper struct {
	responses []*http.Response
	callCount int
	err       error
}

func (m *mockRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	if m.err != nil {
		return nil, m.err
	}

	if len(m.responses) > 0 {
		if m.callCount >= len(m.responses) {
			return nil, errors.New("mock: unexpected call to RoundTrip")
		}
		resp := m.responses[m.callCount]
		m.callCount++
		return resp, nil
	}

	// Default response when no responses configured
	return &http.Response{
		StatusCode: 200,
		Body:       http.NoBody,
		Request:    req,
	}, nil
}

func TestRestrictedTransport(t *testing.T) {
	mockShouldContactHost := func(hostResults map[string]error) hostContactCheck {
		return func(_ context.Context, h string, _, _ []netip.Prefix) error {
			return hostResults[h]
		}
	}

	testCases := []struct {
		name        string
		url         string
		hostResults map[string]error
		errContains string
		mockError   error
		responses   []*http.Response
	}{
		{
			name:        "nil shouldContactHost returns error",
			url:         "http://safe.example.com:5000/v2/",
			errContains: "nil validation function",
		},
		{
			name: "shouldContactHost blocks request",
			url:  "http://blocked.local:5000/v2/",
			hostResults: map[string]error{
				"blocked.local": errors.New("blocked by policy"),
			},
			errContains: "blocked by policy",
		},
		{
			name:        "shouldContactHost allows request",
			url:         "http://allowed.example.com:5000/v2/",
			hostResults: map[string]error{},
		},
		{
			name: "shouldContactHost allows initial host but blocks redirect",
			url:  "http://safe.example.com/v2/",
			responses: []*http.Response{
				{
					StatusCode: 302,
					Header: http.Header{
						"Location": []string{"http://evil.local/v2/"},
					},
					Body: http.NoBody,
				},
			},
			hostResults: map[string]error{
				"safe.example.com": nil,
				"evil.local":       errors.New("redirect blocked"),
			},
			errContains: "redirect blocked",
		},
		{
			name: "shouldContactHost allows redirect chain",
			url:  "http://safe1.example.com/v2/",
			responses: []*http.Response{
				{
					StatusCode: 302,
					Header: http.Header{
						"Location": []string{"http://safe2.example.com/v2/"},
					},
					Body: http.NoBody,
				},
				{
					StatusCode: 200,
					Body:       http.NoBody,
				},
			},
			hostResults: map[string]error{
				"safe1.example.com": nil,
				"safe2.example.com": nil,
			},
		},
		{
			name:        "propagate wrapped transport error",
			url:         "http://safe.local:5000/v2/",
			hostResults: map[string]error{},
			mockError:   errors.New("connection refused"),
			errContains: "connection refused",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			mock := &mockRoundTripper{
				responses: tc.responses,
				err:       tc.mockError,
			}

			transport := NewRestrictedTransport(mock, nil, nil)
			transport.shouldContactHost = nil
			if tc.hostResults != nil {
				transport.shouldContactHost = mockShouldContactHost(tc.hostResults)
			}

			client := &http.Client{Transport: transport}
			resp, err := client.Get(tc.url)
			defer func() {
				if resp != nil && resp.Body != nil {
					resp.Body.Close()
				}
			}()

			if tc.errContains != "" {
				if err == nil {
					t.Errorf("expected error but got none")
					return
				}
				if !strings.Contains(err.Error(), tc.errContains) {
					t.Errorf("expected error to contain %q, got: %v", tc.errContains, err)
				}
				return
			}

			if err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		})
	}
}
