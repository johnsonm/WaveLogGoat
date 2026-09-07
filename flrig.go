package main

import (
	"fmt"
	"strconv"

	"github.com/kolo/xmlrpc"
)

// implements RadioClient for XML-RPC communication with flrig
type FlrigClient struct {
	Host string
	Port int
}

func (f *FlrigClient) SetData(freq float64, mode string) error {
	client, err := xmlrpc.NewClient(fmt.Sprintf("http://%s:%d/", f.Host, f.Port), nil)
	if err != nil {
		return err
	}
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

	client, err := xmlrpc.NewClient(fmt.Sprintf("http://%s:%d/", f.Host, f.Port), nil)
	if err != nil {
		return data, err
	}
	defer client.Close()

	if err := client.Call("rig.get_vfo", nil, &vfoA); err != nil {
		return RigData{}, fmt.Errorf("call failed to rig.get_vfo: %w", err)
	}
	if data.FreqVFOA, err = strconv.ParseFloat(vfoA, 64); err != nil {
		log.Errorf("Failed to parse vfo frequency %s: %s", vfoA, err)
		return RigData{}, err
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
		log.Errorf("Failed to parse vfoB frequency %s: %s", vfoB, err)
		return RigData{}, err
	}

	if err := client.Call("rig.get_modeB", nil, &data.ModeB); err != nil {
		log.Debugf("call failed to rig.get_modeB (flrig): %v. Sending ModeA.", err)
		data.ModeB = data.Mode
	}

	log.Debugf("Got data %#v", data)
	return data, nil
}
