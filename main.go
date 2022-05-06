package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"strconv"
	"time"

	"github.com/pwhelan/gousb"
)

type ID gousb.ID

type config struct {
	Product ID       `json:"product"`
	Vendor  ID       `json:"vendor"`
	CmdUp   []string `json:"up"`
	CmdDown []string `json:"down"`
}

func (id *ID) UnmarshalJSON(data []byte) error {
	var num string
	if err := json.Unmarshal(data, &num); err != nil {
		return err
	}
	val, err := strconv.ParseUint(num[2:], 16, 32)
	if err != nil {
		return err
	}
	*id = ID(val)
	return nil
}

type configs []config

func main() {
	cfgs := make(configs, 0)
	usb := gousb.NewContext()

	cfgdata, err := ioutil.ReadFile(os.Args[1])
	if err != nil {
		panic(err)
	}
	if err := json.Unmarshal(cfgdata, &cfgs); err != nil {
		panic(err)
	}

	usb.RegisterHotplug(func(ev gousb.HotplugEvent) {
		desc, err := ev.DeviceDesc()
		if err != nil {
			panic(err)
		}

		for _, cfg := range cfgs {
			if desc.Vendor == gousb.ID(cfg.Vendor) && desc.Product == gousb.ID(cfg.Product) {
				if ev.Type() == gousb.HotplugEventDeviceArrived {
					cmd := exec.Command(cfg.CmdUp[0], cfg.CmdUp[1:]...)
					if err := cmd.Run(); err != nil {
						fmt.Printf("ERROR: %s", err)
					}
				} else if ev.Type() == gousb.HotplugEventDeviceLeft {
					cmd := exec.Command(cfg.CmdUp[0], cfg.CmdDown[1:]...)
					if err := cmd.Run(); err != nil {
						fmt.Printf("ERROR: %s", err)
					}
				}
			}
		}
	})

	for {
		time.Sleep(time.Second * 30)
	}
}
