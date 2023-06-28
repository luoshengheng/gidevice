package giDevice

import (
	"bytes"

	"github.com/electricbubble/gidevice/pkg/libimobiledevice"
	"howett.net/plist"
)

func newDiagnosticsRelay(client *libimobiledevice.DiagnosticsRelayClient) *diagnostics {
	return &diagnostics{
		client: client,
	}
}

type diagnostics struct {
	client *libimobiledevice.DiagnosticsRelayClient
}

func (d *diagnostics) Reboot() (err error) {
	var pkt libimobiledevice.Packet
	if pkt, err = d.client.NewXmlPacket(
		d.client.NewBasicRequest("Restart"),
	); err != nil {
		return
	}
	if err = d.client.SendPacket(pkt); err != nil {
		return err
	}
	return
}

func (d *diagnostics) Shutdown() (err error) {
	var pkt libimobiledevice.Packet
	if pkt, err = d.client.NewXmlPacket(
		d.client.NewBasicRequest("Shutdown"),
	); err != nil {
		return
	}
	if err = d.client.SendPacket(pkt); err != nil {
		return err
	}
	return
}

func (d *diagnostics) DumpBattery() (result interface{}, err error) { //add
	var pkt libimobiledevice.Packet
	if pkt, err = d.client.NewXmlPacket(
		map[string]string{"Request": "IORegistry", "EntryClass": "IOPMPowerSource"},
	); err != nil {
		return
	}
	if err = d.client.SendPacket(pkt); err != nil {
		return
	}
	var response []byte
	if response, err = d.client.ReceiveBytes(); err != nil {
		return
	}

	buf := bytes.NewReader(response)
	decoder := plist.NewDecoder(buf)
	decoder.Decode(&result)

	return
}

func (d *diagnostics) Close() {
	connc := d.client.InnerConn()
	if connc != nil {
		connc.Close()
	}
}
