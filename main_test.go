package main

import (
	"github.com/magiconair/properties/assert"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestIp2Loc(t *testing.T) {
	router := getRouter()
	w := httptest.NewRecorder()

	req, _ := http.NewRequest("GET", "/ip2loc?ip=54.248.162.57", nil)
	router.ServeHTTP(w, req)
	assert.Equal(t, 200, w.Code)

}

func TestIp2Loc_ForwardedForList(t *testing.T) {
	router := getRouter()
	w := httptest.NewRecorder()

	req, _ := http.NewRequest("GET", "/ip2loc", nil)
	req.Header.Set("X-Forwarded-For", "54.248.162.57, 10.0.0.1")
	router.ServeHTTP(w, req)
	assert.Equal(t, 200, w.Code)
}

func TestIp2Loc_NoIPProvided(t *testing.T) {
	router := getRouter()
	w := httptest.NewRecorder()

	req, _ := http.NewRequest("GET", "/ip2loc", nil)
	router.ServeHTTP(w, req)
	assert.Equal(t, 400, w.Code)
}
