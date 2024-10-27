package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/elastic/gosigar/psnotify"
	"github.com/pwhelan/gousb"
)

type ID gousb.ID

type Command struct {
	Command string
	Args    []string
}

func (command Command) Exec() error {
	cmd := exec.Command(command.Command, command.Args...)
	return cmd.Run()
}

type Commands []Command

type execconfig struct {
	Binary  string   `json:"bin"`
	CmdUp   Commands `json:"up"`
	CmdDown Commands `json:"down"`
}

type usbhotplugconfig struct {
	Product ID       `json:"product"`
	Vendor  ID       `json:"vendor"`
	CmdUp   Commands `json:"up"`
	CmdDown Commands `json:"down"`
}

type config struct {
	USB  []usbhotplugconfig `json:"usb"`
	Exec []execconfig       `json:"commands"`
}

func (commands Commands) Exec() []error {
	fmt.Printf("EXEC=%+v\n", commands)
	errs := make([]error, 0)
	for _, cmd := range commands {
		if err := cmd.Exec(); err != nil {
			errs = append(errs, err)
		}
	}
	return errs
}

func (commands *Commands) UnmarshalJSON(buf []byte) error {
	var data interface{}

	if err := json.Unmarshal(buf, &data); err != nil {
		return err
	}

	switch cmds := data.(type) {
	case string:
		cmdparts := strings.Split(cmds, " ")
		if len(cmdparts) >= 2 {
			*commands = Commands{{
				Command: cmdparts[0],
				Args:    cmdparts[1:],
			}}
		} else if len(cmdparts) == 1 {
			*commands = Commands{{
				Command: cmdparts[0],
				Args:    make([]string, 0),
			}}
		} else {
			return fmt.Errorf("unable to support blank command")
		}
		return nil
	case []interface{}:
		if len(cmds) <= 0 {
			return fmt.Errorf("unable to support blank command")
		}
		if _, ok := cmds[0].(string); ok {
			args := make([]string, 0)
			for _, cmdparts := range cmds[1:] {
				args = append(args, cmdparts.(string))
			}
			*commands = Commands{{
				Command: cmds[0].(string),
				Args:    args,
			}}
			return nil
		}
		*commands = make(Commands, 0)
		for _, cmd := range cmds {
			cmdparts, ok := cmd.([]interface{})
			if !ok {
				return fmt.Errorf("unable to unmarshal command")
			}
			args := make([]string, 0)
			for _, cmdparts := range cmdparts[1:] {
				args = append(args, cmdparts.(string))
			}
			*commands = append(*commands, Command{
				Command: cmdparts[0].(string),
				Args:    args,
			})
		}
		return nil
	default:
		return fmt.Errorf("unable to unmarshal command format: %+v", data)
	}
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

		fmt.Printf("%+v\n", desc)
		for _, cfg := range cfg.USB {
			if desc.Vendor == gousb.ID(cfg.Vendor) && desc.Product == gousb.ID(cfg.Product) {
				if ev.Type() == gousb.HotplugEventDeviceArrived {
					fmt.Printf("UP=%+v\n", cfg.CmdUp)
					if errs := cfg.CmdUp.Exec(); len(errs) > 0 {
						for _, err := range errs {
							fmt.Printf("ERROR: %s", err)
						}
					}
				} else if ev.Type() == gousb.HotplugEventDeviceLeft {
					fmt.Printf("DOWN=%+v\n", cfg.CmdDown)
					if errs := cfg.CmdDown.Exec(); len(errs) > 0 {
						for _, err := range errs {
							fmt.Printf("ERROR: %s", err)
						}
					}
				}
			} else {
				fmt.Printf("%v != %v\n", desc.Vendor, cfg.Vendor)
				fmt.Printf("%v != %v\n", desc.Product, cfg.Product)
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
					if errs := ex.CmdUp.Exec(); len(errs) > 0 {
						for _, err := range errs {
							fmt.Printf("ERROR: %s", err)
						}
					}
					execd[ev.Pid] = ex
				}
			case ev := <-pswatcher.Exit:
				if ex, ok := execd[ev.Pid]; ok {
					log.Printf("exit event: %d->%s (%+v)", ev.Pid, ex.Binary, ev)
					if errs := ex.CmdDown.Exec(); len(errs) > 0 {
						for _, err := range errs {
							fmt.Printf("ERROR: %s", err)
						}
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
