package main

import (
	"fmt"
	"os/exec"
	"github.com/pwhelan/gousb"
)

func main() {
	usb := gousb.NewContext()
	usb.RegisterHotplug(func(ev gousb.HotplugEvent) {
		desc, err := ev.DeviceDesc()
		if err != nil {
			panic(err)
		}
		// 445a:1015
		// ./m1ddc display 1 set input 17 ; sleep 5 ; ./m1ddc display 1 set input 6
		if desc.Vendor == 0x05e3 && desc.Product == 0x0610 {
			fmt.Println("PWNEDDD!")
			if ev.Type() == gousb.HotplugEventDeviceArrived {
				fmt.Println("ADDED")
				cmd := exec.Command("/Users/pwhelan/bin/m1ddc",
					"display", "1", "set", "input", "6")
				if err := cmd.Run(); err != nil {
					fmt.Printf("ERROR: %w", err)
				}
			} else if ev.Type() == gousb.HotplugEventDeviceLeft {
				fmt.Println("HAS LEFT")
				cmd := exec.Command("/Users/pwhelan/bin/m1ddc",
					"display", "1", "set", "input", "15")
				if err := cmd.Run(); err != nil {
					fmt.Printf("ERROR: %w", err)
				}
			}
		}
	})
	for {
		
	}
}
