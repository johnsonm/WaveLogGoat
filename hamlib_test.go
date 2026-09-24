package main

import (
	"bytes"
	"context"
	"net"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

// MockConn is a mock implementation of net.Conn.
type MockConn struct {
	net.Conn
	ReadBuffer  *bytes.Buffer
	WriteBuffer *bytes.Buffer
}

func (m *MockConn) Read(b []byte) (n int, err error) {
	return m.ReadBuffer.Read(b)
}

func (m *MockConn) Write(b []byte) (n int, err error) {
	return m.WriteBuffer.Write(b)
}

func (m *MockConn) Close() error {
	return nil
}

// MockConnectionProvider is a mock implementation of the ConnectionProvider interface.
type MockConnectionProvider struct {
	mock.Mock
}

func (m *MockConnectionProvider) Dial(ctx context.Context, network, address string) (net.Conn, error) {
	args := m.Called(ctx, network, address)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(net.Conn), args.Error(1)
}

func TestHamlibClient_GetData(t *testing.T) {
	ctx := context.Background()
	host := "localhost"
	port := 4532
	maxPower := 100.0

	t.Run("successful get data", func(t *testing.T) {
		mockDialer := new(MockConnectionProvider)

		// Mock responses for:
		// f\n -> 14074000
		// m\n -> USB\n0\n
		// l RFPOWER\n -> 50
		readBuffer := bytes.NewBufferString("14074000\nUSB\n0\n50\n")
		mockConn := &MockConn{ReadBuffer: readBuffer, WriteBuffer: new(bytes.Buffer)}

		mockDialer.On("Dial", ctx, "tcp", "localhost:4532").Return(mockConn, nil).Once()

		client := NewHamlibClient(ctx, host, port, maxPower, mockDialer)
		data, err := client.GetData()

		assert.NoError(t, err)
		assert.Equal(t, 14074000.0, data.FreqVFOA)
		assert.Equal(t, "USB", data.Mode)
		assert.Equal(t, 50.0, data.Power)
		assert.True(t, data.PowerValid)
		assert.Equal(t, 0, data.Split)

		mockDialer.AssertExpectations(t)
	})
}

func TestHamlibClient_SetData(t *testing.T) {
	ctx := context.Background()
	host := "localhost"
	port := 4532
	maxPower := 100.0

	t.Run("successful set data", func(t *testing.T) {
		mockDialer := new(MockConnectionProvider)
		mockConn := &MockConn{ReadBuffer: new(bytes.Buffer), WriteBuffer: new(bytes.Buffer)}

		mockDialer.On("Dial", ctx, "tcp", "localhost:4532").Return(mockConn, nil).Once()

		client := NewHamlibClient(ctx, host, port, maxPower, mockDialer)
		err := client.SetData(7074000.0, "LSB")

		assert.NoError(t, err)
		assert.Contains(t, mockConn.WriteBuffer.String(), "F 7074000\n")
		assert.Contains(t, mockConn.WriteBuffer.String(), "M LSB 0\n")

		mockDialer.AssertExpectations(t)
	})
}
