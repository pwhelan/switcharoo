package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"

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

func (commands Commands) Exec() []error {
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

type config struct {
	Product ID       `json:"product"`
	Vendor  ID       `json:"vendor"`
	CmdUp   Commands `json:"up"`
	CmdDown Commands `json:"down"`
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
					if errs := cfg.CmdUp.Exec(); len(errs) > 0 {
						for _, err := range errs {
							fmt.Printf("ERROR: %s", err)
						}
					}
				} else if ev.Type() == gousb.HotplugEventDeviceLeft {
					if errs := cfg.CmdDown.Exec(); len(errs) > 0 {
						for _, err := range errs {
							fmt.Printf("ERROR: %s", err)
						}
					}
				}
			}
		}
	})

	for {
		time.Sleep(time.Second * 30)
	}
}
