package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"os/exec"
	"strconv"
	"time"

	"github.com/elastic/gosigar/psnotify"
	"github.com/pwhelan/gousb"
)

type ID gousb.ID

type usbhotplugconfig struct {
	Product ID       `json:"product"`
	Vendor  ID       `json:"vendor"`
	CmdUp   []string `json:"up"`
	CmdDown []string `json:"down"`
}

type execconfig struct {
	Binary  string   `json:"bin"`
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

type config struct {
	USB  []usbhotplugconfig `json:"usb"`
	Exec []execconfig       `json:"exec"`
}

func isNumeric(s string) bool {
	_, err := strconv.ParseInt(s, 10, 64)
	return err == nil
}

func main() {
	var cfg config
	execs := make(map[string]execconfig)
	execd := make(map[int]execconfig)

	usb := gousb.NewContext()

	cfgdata, err := ioutil.ReadFile(os.Args[1])
	if err != nil {
		panic(err)
	}
	if err := json.Unmarshal(cfgdata, &cfg); err != nil {
		panic(err)
	}

	usb.RegisterHotplug(func(ev gousb.HotplugEvent) {
		desc, err := ev.DeviceDesc()
		if err != nil {
			panic(err)
		}

		for _, cfg := range cfg.USB {
			if desc.Vendor == gousb.ID(cfg.Vendor) && desc.Product == gousb.ID(cfg.Product) {
				if ev.Type() == gousb.HotplugEventDeviceArrived {
					cmd := exec.Command(cfg.CmdUp[0], cfg.CmdUp[1:]...)
					if err := cmd.Run(); err != nil {
						fmt.Printf("ERROR: %s", err)
					}
				} else if ev.Type() == gousb.HotplugEventDeviceLeft {
					cmd := exec.Command(cfg.CmdDown[0], cfg.CmdDown[1:]...)
					if err := cmd.Run(); err != nil {
						fmt.Printf("ERROR: %s", err)
					}
				}
			}
		}
	})

	pswatcher, err := psnotify.NewWatcher()
	if err != nil {
		log.Fatal(err)
	}

	for _, exec := range cfg.Exec {
		execs[exec.Binary] = exec
	}

	go func() {
		for {
			select {
			case ev := <-pswatcher.Exec:
				bin, _ := os.Readlink(fmt.Sprintf("/proc/%d/exe", ev.Pid))
				if ex, ok := execs[bin]; ok {
					log.Printf("exec event: %d->%s", ev.Pid, bin)
					cmd := exec.Command(ex.CmdUp[0], ex.CmdUp[1:]...)
					if err := cmd.Run(); err != nil {
						fmt.Printf("ERROR: %s\n", err)
					}
					execd[ev.Pid] = ex
				}
			case ev := <-pswatcher.Exit:
				if ex, ok := execd[ev.Pid]; ok {
					log.Printf("exit event: %d->%s (%+v)", ev.Pid, ex.Binary, ev)
					cmd := exec.Command(ex.CmdDown[0], ex.CmdDown[1:]...)
					if err := cmd.Run(); err != nil {
						fmt.Printf("ERROR: %s\n", err)
					}
					delete(execd, ev.Pid)
				}
			}
		}
	}()

	files, err := ioutil.ReadDir("/proc")
	if err != nil {
		log.Fatal(err)
	}

	for _, f := range files {
		if f.IsDir() && isNumeric(f.Name()) {
			pid, _ := strconv.ParseInt(f.Name(), 10, 64)
			err = pswatcher.Watch(int(pid), psnotify.PROC_EVENT_EXIT|psnotify.PROC_EVENT_EXEC)
			if err != nil {
				log.Fatal(err)
			}
		}
	}

	defer pswatcher.Close()

	for {
		time.Sleep(time.Second * 30)
	}
}
