package perf

import (
	"fmt"
	"github.com/electricbubble/gidevice/pkg/nskeyedarchiver"
	uuid "github.com/satori/go.uuid"
	"log"
	"strings"
	"testing"
	"time"

	giDevice "github.com/electricbubble/gidevice"
	"github.com/electricbubble/gidevice/pkg/libimobiledevice"
	biPack "github.com/roman-kachanovsky/go-binary-pack/binary-pack"
)

type Any = interface{}

func Test_Performance(t *testing.T) {
	usbmux, _ := giDevice.NewUsbmux()
	devices, err := usbmux.Devices()
	var device giDevice.Device
	if err != nil {
		log.Fatalln(err)
	}
	if len(devices) < 1 {
		log.Fatalln("No iOS devices connected!")
	}
	device = devices[0]
	instrumentsClient := device.GetInstrumentsClient()
	channelName := "com.apple.instruments.server.services.coreprofilesessiontap"
	id, _ := instrumentsClient.RequestChannel(channelName)
	instrumentsClient.RegisterCallbackArgs(channelName, coreProfileSessionTapCallback)
	args := libimobiledevice.NewAuxBuffer()
	uuid4 := strings.ToUpper(uuid.NewV4().String())
	tc := map[string]Any{
		"kdf2": nskeyedarchiver.NewNSSet([]interface{}{630784000, 833617920, 830472456}),
		"tk":   3,
		"uuid": uuid4,
	}
	config := map[string]Any{
		"rp": 10,
		"tc": []map[string]Any{tc},
		"ur": 500,
	}
	instrumentsClient.RegisterCallback("", func(messageResult libimobiledevice.DTXMessageResult) {

	})
	args.AppendObject(config)
	instrumentsClient.Invoke("setConfig:", args, id, true)
	args = libimobiledevice.NewAuxBuffer()
	instrumentsClient.Invoke("start", args, id, true)
	time.Sleep(time.Minute * 9999)
	instrumentsClient.Invoke("stop", args, id, false)
}

func coreProfileSessionTapCallback(messageResult libimobiledevice.DTXMessageResult, args ...Any) {
	switch result := messageResult.Obj.(type) {
	case libimobiledevice.UnmarshalError:
		format := []string{"Q", "L", "L", "Q", "Q", "Q", "Q", "L", "L", "Q"} //<QLLQQQQLLQ
		bp := new(biPack.BinaryPack)
		if len(result.Data)%64 != 0 {
			fmt.Println("illeagal data size: ", len(result.Data))
		}
		p := 0
		for {
			if p+64 < len(result.Data) {
				unpacked_values, _ := bp.UnPack(format, result.Data[p:p+64])
				switch unpacked_values[7].(int) {
				case 830472268:
					log.Println(unpacked_values)
				case 830473056:
					log.Println(unpacked_values)
				case 830472984:
					log.Println(unpacked_values)
				default:
					fmt.Printf("illeagl frame debug code: %d\n", unpacked_values[7])
				}
				p += 64
			} else {
				break
			}
		}
	}
}
