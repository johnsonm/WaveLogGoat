package main

import (
	"context"
	"fmt"
	"net/http"
	"strconv"

	"github.com/kolo/xmlrpc"
)

// XMLRPCClient defines the interface for XML-RPC client operations, allowing for mocking in tests.
type XMLRPCClient interface {
	Call(method string, args interface{}, reply interface{}) error
	Close() error
}

// liveXMLRPCClient is a concrete implementation of XMLRPCClient that wraps xmlrpc.Client.
type liveXMLRPCClient struct {
	client *xmlrpc.Client
}

// NewLiveXMLRPCClient creates a new liveXMLRPCClient.
func NewLiveXMLRPCClient(host string, port int, transport http.RoundTripper) (XMLRPCClient, error) {
	url := fmt.Sprintf("http://%s:%d", host, port)
	client, err := xmlrpc.NewClient(url, transport)
	if err != nil {
		return nil, err
	}
	return &liveXMLRPCClient{client: client}, nil
}

// NewFlrigRadioClient initializes a flrig client with CustomDialer for mDNS resolution.
func NewFlrigRadioClient(ctx context.Context, host string, port int) (RadioClient, error) {
	provider := &defaultTransportProvider{}
	rpcClient, err := NewLiveXMLRPCClient(host, port, provider.GetTransport())
	if err != nil {
		return nil, err
	}
	return NewFlrigClient(ctx, host, port, rpcClient), nil
}

func (r *liveXMLRPCClient) Call(method string, args interface{}, reply interface{}) error {
	return r.client.Call(method, args, reply)
}

func (r *liveXMLRPCClient) Close() error {
	// kolo/xmlrpc.Client.Close() does not return an error, but our interface expects it.
	// It's safe to return nil here.
	return nil
}

// implements RadioClient for XML-RPC communication with flrig
type FlrigClient struct {
	ctx       context.Context
	Host      string
	Port      int
	rpcClient XMLRPCClient // Inject the XML-RPC client
}

// NewFlrigClient creates a new FlrigClient with the given XMLRPCClient.
func NewFlrigClient(ctx context.Context, host string, port int, rpcClient XMLRPCClient) *FlrigClient {
	return &FlrigClient{
		ctx:       ctx,
		Host:      host,
		Port:      port,
		rpcClient: rpcClient,
	}
}

func (f *FlrigClient) SetData(freq float64, mode string) error {
	client := f.rpcClient
	defer client.Close()

	if freq > 0 {
		if err := client.Call("rig.set_frequency", freq, nil); err != nil {
			return fmt.Errorf("call failed to rig.set_frequency: %w", err)
		}
	}
	if mode != "" {
		if err := client.Call("rig.set_mode", mode, nil); err != nil {
			return fmt.Errorf("call failed to rig.set_mode: %w", err)
		}
	}
	return nil
}

func (f *FlrigClient) GetData() (RigData, error) {
	var data RigData
	var vfoA string
	var power int
	var vfoB string
	var err error

	client := f.rpcClient
	defer client.Close()

	if err := client.Call("rig.get_vfo", nil, &vfoA); err != nil {
		return RigData{}, fmt.Errorf("call failed to rig.get_vfo: %w", err)
	}
	if data.FreqVFOA, err = strconv.ParseFloat(vfoA, 64); err != nil {
		return RigData{}, fmt.Errorf("Failed to parse vfo frequency %s: %w", vfoA, err)
	}

	if err := client.Call("rig.get_mode", nil, &data.Mode); err != nil {
		return RigData{}, fmt.Errorf("call failed to rig.get_mode: %w", err)
	}

	if err := client.Call("rig.get_power", nil, &power); err != nil {
		log.Debugf("call failed to rig.get_power (flrig): %v. Sending 0 power.", err)
		power = 0
	}
	data.Power = float64(power)
	data.PowerValid = true

	if err := client.Call("rig.get_split", nil, &data.Split); err != nil {
		log.Warnf("call failed to rig.get_split (flrig): %v. Sending Split=0.", err)
		data.Split = 0
	}

	if err := client.Call("rig.get_vfoB", nil, &vfoB); err != nil {
		log.Debugf("call failed to rig.get_vfoB (flrig): %v. Sending vfoA %s.", err, vfoA)
		vfoB = vfoA
	}
	if data.FreqVFOB, err = strconv.ParseFloat(vfoB, 64); err != nil {
		log.Errorf("Failed to parse vfoB frequency %s: %v", vfoB, err)
		return RigData{}, err
	}

	if err := client.Call("rig.get_modeB", nil, &data.ModeB); err != nil {
		log.Debugf("call failed to rig.get_modeB (flrig): %v. Sending ModeA.", err)
		data.ModeB = data.Mode
	}

	log.Debugf("Got data %#v", data)
	return data, nil
}
