package main

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

// MockRadioClient is a mock implementation of the RadioClient interface.
type MockRadioClient struct {
	mock.Mock
}

func (m *MockRadioClient) GetData() (RigData, error) {
	args := m.Called()
	return args.Get(0).(RigData), args.Error(1)
}

func (m *MockRadioClient) SetData(freq float64, mode string) error {
	args := m.Called(freq, mode)
	return args.Error(0)
}

func TestQSYHandler(t *testing.T) {
	t.Run("successful QSY frequency and mode", func(t *testing.T) {
		mockClient := new(MockRadioClient)
		mockClient.On("SetData", 7074000.0, "LSB").Return(nil).Once()

		handler := qsyHandler(mockClient)
		req := httptest.NewRequest("GET", "/7074000/LSB", nil)
		w := httptest.NewRecorder()

		handler(w, req)

		assert.Equal(t, http.StatusOK, w.Code)
		assert.Equal(t, "application/json", w.Header().Get("Content-Type"))

		var response map[string]interface{}
		json.NewDecoder(w.Body).Decode(&response)
		assert.Equal(t, "success", response["status"])
		assert.Equal(t, float64(7074000), response["frequency"])
		assert.Equal(t, "LSB", response["mode"])

		mockClient.AssertExpectations(t)
	})

	t.Run("invalid frequency", func(t *testing.T) {
		mockClient := new(MockRadioClient)
		handler := qsyHandler(mockClient)
		req := httptest.NewRequest("GET", "/invalid/LSB", nil)
		w := httptest.NewRecorder()

		handler(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code)
		mockClient.AssertNotCalled(t, "SetData", mock.Anything, mock.Anything)
	})

	t.Run("radio client error", func(t *testing.T) {
		mockClient := new(MockRadioClient)
		mockClient.On("SetData", 7074000.0, "LSB").Return(errors.New("radio error")).Once()

		handler := qsyHandler(mockClient)
		req := httptest.NewRequest("GET", "/7074000/LSB", nil)
		w := httptest.NewRecorder()

		handler(w, req)

		assert.Equal(t, http.StatusServiceUnavailable, w.Code)

		var response map[string]string
		json.NewDecoder(w.Body).Decode(&response)
		assert.Equal(t, "QSY failed: radio control software not available", response["error"])

		mockClient.AssertExpectations(t)
	})

	t.Run("CORS options request", func(t *testing.T) {
		mockClient := new(MockRadioClient)
		handler := qsyHandler(mockClient)
		req := httptest.NewRequest("OPTIONS", "/", nil)
		w := httptest.NewRecorder()

		handler(w, req)

		assert.Equal(t, http.StatusOK, w.Code)
		assert.Equal(t, "*", w.Header().Get("Access-Control-Allow-Origin"))
	})
}
