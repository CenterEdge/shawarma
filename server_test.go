package main

import (
	"fmt"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
)

type Endpoint struct {
	method   string
	url      string
	expected string
}

var serverEndpoints = []Endpoint{
	{"GET", "/_health", `{"health": "ok"}`},
	{"GET", "/deploymentstate", `{"status":"inactive","activeServices":[]}`},
}

func TestEndpoints(t *testing.T) {
	assert := assert.New(t)

	for _, endpoint := range serverEndpoints {

		req := httptest.NewRequest(endpoint.method, endpoint.url, nil)
		w := httptest.NewRecorder()
		router(endpoint, w, req)

		// Parse output
		result := w.Result()
		defer result.Body.Close()
		data, err := ioutil.ReadAll(result.Body)

		// Err validation
		if err != nil {
			t.Errorf("expected error to be nil got %v", err)
		}

		// Asserts
		assert.Equal(200, result.StatusCode, fmt.Sprintf("expected 200 - got %v", result.StatusCode))
		assert.Equal(endpoint.expected, string(data), fmt.Sprintf("expected %s - got %v", endpoint.expected, string(data)))
	}
}

func router(endpoint Endpoint, w *httptest.ResponseRecorder, req *http.Request) {
	// Route handler
	if endpoint.url == "/_health" {
		_health(w, req)
	}
	if endpoint.url == "/deploymentstate" {
		deploymentState(w, req)
	}

}
