package main

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

// MockXMLRPCClient is a mock implementation of the XMLRPCClient interface.
type MockXMLRPCClient struct {
	mock.Mock
}

func (m *MockXMLRPCClient) Call(method string, args interface{}, reply interface{}) error {
	callArgs := m.Called(method, args, reply)

	// If a reply object is expected, set its value from the mock return.
	if reply != nil && len(callArgs) > 1 && callArgs.Get(0) != nil {
		switch r := reply.(type) {
		case *string:
			*r = callArgs.Get(0).(string)
		case *int:
			*r = callArgs.Get(0).(int)
		case *float64:
			*r = callArgs.Get(0).(float64)
		default:
			// Handle other types if necessary, or panic for unhandled types
			// For now, we'll assume string/int/float64 for simplicity in flrig
		}
	}

	if len(callArgs) > 1 {
		return callArgs.Error(1)
	}
	return callArgs.Error(0)
}

func (m *MockXMLRPCClient) Close() error {
	args := m.Called()
	return args.Error(0)
}

func TestFlrigClient_SetData(t *testing.T) {
	ctx := context.Background()
	host := "localhost"
	port := 8080

	t.Run("successful set frequency and mode", func(t *testing.T) {
		mockClient := new(MockXMLRPCClient)
		mockClient.On("Call", "rig.set_frequency", mock.AnythingOfType("float64"), nil).Return(nil).Once()
		mockClient.On("Call", "rig.set_mode", mock.AnythingOfType("string"), nil).Return(nil).Once()
		mockClient.On("Close").Return(nil).Once()

		flrig := NewFlrigClient(ctx, host, port, mockClient)
		err := flrig.SetData(14074000.0, "USB")

		assert.NoError(t, err)
		mockClient.AssertExpectations(t)
	})

	t.Run("successful set frequency only", func(t *testing.T) {
		mockClient := new(MockXMLRPCClient)
		mockClient.On("Call", "rig.set_frequency", mock.AnythingOfType("float64"), nil).Return(nil).Once()
		mockClient.On("Close").Return(nil).Once()

		flrig := NewFlrigClient(ctx, host, port, mockClient)
		err := flrig.SetData(7074000.0, "")

		assert.NoError(t, err)
		mockClient.AssertExpectations(t)
	})

	t.Run("successful set mode only", func(t *testing.T) {
		mockClient := new(MockXMLRPCClient)
		mockClient.On("Call", "rig.set_mode", mock.AnythingOfType("string"), nil).Return(nil).Once()
		mockClient.On("Close").Return(nil).Once()

		flrig := NewFlrigClient(ctx, host, port, mockClient)
		err := flrig.SetData(0, "CW")

		assert.NoError(t, err)
		mockClient.AssertExpectations(t)
	})

	t.Run("error on set frequency", func(t *testing.T) {
		mockClient := new(MockXMLRPCClient)
		expectedErr := errors.New("failed to set frequency")
		mockClient.On("Call", "rig.set_frequency", mock.AnythingOfType("float64"), nil).Return(expectedErr).Once()
		mockClient.On("Close").Return(nil).Once()

		flrig := NewFlrigClient(ctx, host, port, mockClient)
		err := flrig.SetData(14074000.0, "USB")

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "call failed to rig.set_frequency")
		mockClient.AssertExpectations(t)
	})

	t.Run("error on set mode", func(t *testing.T) {
		mockClient := new(MockXMLRPCClient)
		mockClient.On("Call", "rig.set_frequency", mock.AnythingOfType("float64"), nil).Return(nil).Once()
		expectedErr := errors.New("failed to set mode")
		mockClient.On("Call", "rig.set_mode", mock.AnythingOfType("string"), nil).Return(expectedErr).Once()
		mockClient.On("Close").Return(nil).Once()

		flrig := NewFlrigClient(ctx, host, port, mockClient)
		err := flrig.SetData(14074000.0, "USB")

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "call failed to rig.set_mode")
		mockClient.AssertExpectations(t)
	})
}

func TestFlrigClient_GetData(t *testing.T) {
	ctx := context.Background()
	host := "localhost"
	port := 8080

	t.Run("successful get data", func(t *testing.T) {
		mockClient := new(MockXMLRPCClient)
		mockClient.On("Call", "rig.get_vfo", nil, mock.Anything).Return("14074000.0", nil).Once()
		mockClient.On("Call", "rig.get_mode", nil, mock.Anything).Return("USB", nil).Once()
		mockClient.On("Call", "rig.get_power", nil, mock.Anything).Return(100, nil).Once()
		mockClient.On("Call", "rig.get_split", nil, mock.Anything).Return(0, nil).Once()
		mockClient.On("Call", "rig.get_vfoB", nil, mock.Anything).Return("14074000.0", nil).Once()
		mockClient.On("Call", "rig.get_modeB", nil, mock.Anything).Return("USB", nil).Once()
		mockClient.On("Close").Return(nil).Once()

		flrig := NewFlrigClient(ctx, host, port, mockClient)
		data, err := flrig.GetData()

		assert.NoError(t, err)
		assert.Equal(t, 14074000.0, data.FreqVFOA)
		assert.Equal(t, "USB", data.Mode)
		assert.Equal(t, 100.0, data.Power)
		assert.True(t, data.PowerValid)
		assert.Equal(t, 0, data.Split)
		assert.Equal(t, 14074000.0, data.FreqVFOB)
		assert.Equal(t, "USB", data.ModeB)
		mockClient.AssertExpectations(t)
	})

	t.Run("error on get_vfo", func(t *testing.T) {
		mockClient := new(MockXMLRPCClient)
		expectedErr := errors.New("failed to get vfo")
		mockClient.On("Call", "rig.get_vfo", nil, mock.Anything).Return(nil, expectedErr).Once()
		mockClient.On("Close").Return(nil).Once()

		flrig := NewFlrigClient(ctx, host, port, mockClient)
		_, err := flrig.GetData()

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "call failed to rig.get_vfo")
		mockClient.AssertExpectations(t)
	})

	t.Run("error parsing vfo frequency", func(t *testing.T) {
		mockClient := new(MockXMLRPCClient)
		mockClient.On("Call", "rig.get_vfo", nil, mock.Anything).Return("invalid_float", nil).Once()
		mockClient.On("Close").Return(nil).Once()

		flrig := NewFlrigClient(ctx, host, port, mockClient)
		_, err := flrig.GetData()

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "Failed to parse vfo frequency")
		mockClient.AssertExpectations(t)
	})

	t.Run("error on get_mode", func(t *testing.T) {
		mockClient := new(MockXMLRPCClient)
		mockClient.On("Call", "rig.get_vfo", nil, mock.Anything).Return("14074000.0", nil).Once()
		expectedErr := errors.New("failed to get mode")
		mockClient.On("Call", "rig.get_mode", nil, mock.Anything).Return(nil, expectedErr).Once()
		mockClient.On("Close").Return(nil).Once()

		flrig := NewFlrigClient(ctx, host, port, mockClient)
		_, err := flrig.GetData()

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "call failed to rig.get_mode")
		mockClient.AssertExpectations(t)
	})

	t.Run("error on get_power defaults to 0", func(t *testing.T) {
		mockClient := new(MockXMLRPCClient)
		mockClient.On("Call", "rig.get_vfo", nil, mock.Anything).Return("14074000.0", nil).Once()
		mockClient.On("Call", "rig.get_mode", nil, mock.Anything).Return("USB", nil).Once()
		mockClient.On("Call", "rig.get_power", nil, mock.Anything).Return(nil, errors.New("power error")).Once()
		mockClient.On("Call", "rig.get_split", nil, mock.Anything).Return(0, nil).Once()
		mockClient.On("Call", "rig.get_vfoB", nil, mock.Anything).Return("14074000.0", nil).Once()
		mockClient.On("Call", "rig.get_modeB", nil, mock.Anything).Return("USB", nil).Once()
		mockClient.On("Close").Return(nil).Once()

		flrig := NewFlrigClient(ctx, host, port, mockClient)
		data, err := flrig.GetData()

		assert.NoError(t, err)
		assert.Equal(t, 0.0, data.Power) // Expect 0.0 power on error
		assert.True(t, data.PowerValid)  // PowerValid should still be true as it's a known state
		mockClient.AssertExpectations(t)
	})

	t.Run("error on get_split defaults to 0", func(t *testing.T) {
		mockClient := new(MockXMLRPCClient)
		mockClient.On("Call", "rig.get_vfo", nil, mock.Anything).Return("14074000.0", nil).Once()
		mockClient.On("Call", "rig.get_mode", nil, mock.Anything).Return("USB", nil).Once()
		mockClient.On("Call", "rig.get_power", nil, mock.Anything).Return(100, nil).Once()
		mockClient.On("Call", "rig.get_split", nil, mock.Anything).Return(nil, errors.New("split error")).Once()
		mockClient.On("Call", "rig.get_vfoB", nil, mock.Anything).Return("14074000.0", nil).Once()
		mockClient.On("Call", "rig.get_modeB", nil, mock.Anything).Return("USB", nil).Once()
		mockClient.On("Close").Return(nil).Once()

		flrig := NewFlrigClient(ctx, host, port, mockClient)
		data, err := flrig.GetData()

		assert.NoError(t, err)
		assert.Equal(t, 0, data.Split) // Expect 0 split on error
		mockClient.AssertExpectations(t)
	})

	t.Run("error on get_vfoB defaults to vfoA", func(t *testing.T) {
		mockClient := new(MockXMLRPCClient)
		mockClient.On("Call", "rig.get_vfo", nil, mock.Anything).Return("14074000.0", nil).Once()
		mockClient.On("Call", "rig.get_mode", nil, mock.Anything).Return("USB", nil).Once()
		mockClient.On("Call", "rig.get_power", nil, mock.Anything).Return(100, nil).Once()
		mockClient.On("Call", "rig.get_split", nil, mock.Anything).Return(0, nil).Once()
		mockClient.On("Call", "rig.get_vfoB", nil, mock.Anything).Return(nil, errors.New("vfoB error")).Once()
		mockClient.On("Call", "rig.get_modeB", nil, mock.Anything).Return("USB", nil).Once()
		mockClient.On("Close").Return(nil).Once()

		flrig := NewFlrigClient(ctx, host, port, mockClient)
		data, err := flrig.GetData()

		assert.NoError(t, err)
		assert.Equal(t, 14074000.0, data.FreqVFOB) // Expect VFOA freq for VFOB on error
		mockClient.AssertExpectations(t)
	})

	t.Run("error parsing vfoB frequency returns error", func(t *testing.T) {
		mockClient := new(MockXMLRPCClient)
		mockClient.On("Call", "rig.get_vfo", nil, mock.Anything).Return("14074000.0", nil).Once()
		mockClient.On("Call", "rig.get_mode", nil, mock.Anything).Return("USB", nil).Once()
		mockClient.On("Call", "rig.get_power", nil, mock.Anything).Return(100, nil).Once()
		mockClient.On("Call", "rig.get_split", nil, mock.Anything).Return(0, nil).Once()
		mockClient.On("Call", "rig.get_vfoB", nil, mock.Anything).Return("invalid_float", nil).Once()
		mockClient.On("Close").Return(nil).Once()

		flrig := NewFlrigClient(ctx, host, port, mockClient)
		data, err := flrig.GetData()

		assert.Error(t, err)
		assert.Equal(t, RigData{}, data)
		mockClient.AssertExpectations(t)
	})

	t.Run("error on get_modeB defaults to modeA", func(t *testing.T) {
		mockClient := new(MockXMLRPCClient)
		mockClient.On("Call", "rig.get_vfo", nil, mock.Anything).Return("14074000.0", nil).Once()
		mockClient.On("Call", "rig.get_mode", nil, mock.Anything).Return("USB", nil).Once()
		mockClient.On("Call", "rig.get_power", nil, mock.Anything).Return(100, nil).Once()
		mockClient.On("Call", "rig.get_split", nil, mock.Anything).Return(0, nil).Once()
		mockClient.On("Call", "rig.get_vfoB", nil, mock.Anything).Return("14074000.0", nil).Once()
		mockClient.On("Call", "rig.get_modeB", nil, mock.Anything).Return(nil, errors.New("modeB error")).Once()
		mockClient.On("Close").Return(nil).Once()

		flrig := NewFlrigClient(ctx, host, port, mockClient)
		data, err := flrig.GetData()

		assert.NoError(t, err)
		assert.Equal(t, "USB", data.ModeB) // Expect ModeA for ModeB on error
		mockClient.AssertExpectations(t)
	})
}
