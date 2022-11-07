package perf

import (
	"log"

	giDevice "github.com/electricbubble/gidevice"
	"github.com/electricbubble/gidevice/pkg/libimobiledevice"
)

type PerfMonitor struct {
	usbmux            giDevice.Usbmux
	device            *giDevice.Device
	instrumentsClient *libimobiledevice.InstrumentsClient
}

func NewMonitor(udid string) PerfMonitor {
	monitor := PerfMonitor{}
	monitor.usbmux, _ = giDevice.NewUsbmux()
	devices, err := monitor.usbmux.Devices()
	var device giDevice.Device
	if err != nil {
		log.Fatalln(err)
	}
	if len(devices) < 1 {
		log.Fatalln("No iOS devices connected!")
	}
	if udid == "" {
		device = devices[0]
	} else {
		for _, dev := range devices {
			properties := dev.Properties()
			if properties.SerialNumber == udid {
				device = dev
			}
		}
	}
	monitor.device = &device
	monitor.instrumentsClient = device.GetInstrumentsClient()
	return monitor
}

func (pm *PerfMonitor) DumpPerformance(indexes []string) {
	pm.dumpCPU()
}

func (pm *PerfMonitor) dumpCPU() {
	var id uint32
	id, _ = pm.instrumentsClient.RequestChannel("com.apple.instruments.server.services.sysmontap")

	selector := "setConfig:"
	args := libimobiledevice.NewAuxBuffer()

	var config map[string]interface{}
	config = make(map[string]interface{})
	{
		config["bm"] = 0
		config["cpuUsage"] = true

		config["procAttrs"] = []string{
			"memVirtualSize", "cpuUsage", "ctxSwitch", "intWakeups",
			"physFootprint", "memResidentSize", "memAnon", "pid"}

		config["sampleInterval"] = 1000000000
		// 系统信息字段
		config["sysAttrs"] = []string{
			"vmExtPageCount", "vmFreeCount", "vmPurgeableCount",
			"vmSpeculativeCount", "physMemSize"}
		// 刷新频率
		config["ur"] = 1000
	}

	args.AppendObject(config)
	pm.instrumentsClient.Invoke(selector, args, id, true)
	selector = "start"
	args = libimobiledevice.NewAuxBuffer()

	pm.instrumentsClient.Invoke(selector, args, id, true)

	pm.instrumentsClient.RegisterCallback("", func(m libimobiledevice.DTXMessageResult) {
		println(123)
		// select {
		// // case <-ctx.Done():
		// // 	return
		// default:
		// 	mess := m.Obj
		// 	chanCPUAndMEMData(mess, _outMEM, _outCPU, pid)
		// }
	})
}
