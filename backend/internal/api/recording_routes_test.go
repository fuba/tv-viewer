package api

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

type recordingReserverStub struct {
	programID int64
	err       error
}

func (s *recordingReserverStub) ReserveProgram(_ context.Context, programID int64) error {
	s.programID = programID
	return s.err
}

func TestReserveRecording(t *testing.T) {
	gin.SetMode(gin.TestMode)
	stub := new(recordingReserverStub)
	router := gin.New()
	router.POST("/api/recordings/reserve", reserveRecording(stub))

	req := httptest.NewRequest(http.MethodPost, "/api/recordings/reserve", bytes.NewBufferString(`{"programId":323912360812345}`))
	req.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, req)

	if response.Code != http.StatusCreated {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
	if stub.programID != 323912360812345 {
		t.Fatalf("program ID = %d", stub.programID)
	}
}

func TestReserveRecordingRejectsInvalidProgramID(t *testing.T) {
	gin.SetMode(gin.TestMode)
	stub := new(recordingReserverStub)
	router := gin.New()
	router.POST("/api/recordings/reserve", reserveRecording(stub))

	req := httptest.NewRequest(http.MethodPost, "/api/recordings/reserve", bytes.NewBufferString(`{"programId":0}`))
	req.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, req)

	if response.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
	if stub.programID != 0 {
		t.Fatal("upstream was called for invalid input")
	}
}

func TestReserveRecordingRejectsCrossSiteOrigin(t *testing.T) {
	gin.SetMode(gin.TestMode)
	stub := new(recordingReserverStub)
	router := gin.New()
	router.POST("/api/recordings/reserve", reserveRecording(stub))

	req := httptest.NewRequest(http.MethodPost, "http://puma2:18089/api/recordings/reserve", bytes.NewBufferString(`{"programId":123}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Origin", "https://evil.example")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, req)

	if response.Code != http.StatusForbidden {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
	if stub.programID != 0 {
		t.Fatal("upstream was called for a cross-site request")
	}
}
