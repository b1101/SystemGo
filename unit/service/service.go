// Package service defines a service unit type
package service

import (
	"io"
	"os/exec"
	"strings"

	"github.com/rvolosatovs/systemgo/unit"
)

const DEFAULT_TYPE = "simple"

const (
	dead         = "dead"
	startPre     = "startPre"
	start        = "start"
	startPost    = "startPost"
	running      = "running"
	exited       = "exited" // not running anymore, but RemainAfterExit true for this unit
	reload       = "reload"
	stop         = "stop"
	stopSigabrt  = "stopSigabrt" // watchdog timeout
	stopSigterm  = "stopSigterm"
	stopSigkill  = "stopSigkill"
	stopPost     = "stopPost"
	finalSigterm = "finalSigterm"
	finalSigkill = "finalSigkill"
	failed       = "failed"
	autoRestart  = "autoRestart"
)

var supported = map[string]bool{
	"oneshot": true,
	"simple":  true,
	"forking": false,
	"dbus":    false,
	"notify":  false,
	"idle":    false,
}

// Service unit
type Unit struct {
	Definition
	*exec.Cmd
}

// Service unit definition
type Definition struct {
	unit.Definition
	Service struct {
		Type                            string
		ExecStart, ExecStop, ExecReload string
		PIDFile                         string
		Restart                         string
		RemainAfterExit                 bool
	}
}

func Supported(typ string) (is bool) {
	return supported[typ]
}

// Define attempts to fill the sv definition by parsing r
func (sv *Unit) Define(r io.Reader /*, errch chan<- error*/) (err error) {
	def := Definition{}
	def.Service.Type = DEFAULT_TYPE

	if err = unit.ParseDefinition(r, &def); err != nil {
		return
	}

	merr := unit.MultiError{}

	// Check definition for errors
	switch {
	case def.Service.ExecStart == "":
		merr = append(merr, unit.ParseErr("ExecStart", unit.ErrNotSet))

	case !Supported(def.Service.Type):
		merr = append(merr, unit.ParseErr("Type", unit.ParseErr(def.Service.Type, unit.ErrNotSupported)))
	}

	if len(merr) > 0 {
		return merr
	}

	sv.Definition = def

	cmd := strings.Fields(def.Service.ExecStart)
	sv.Cmd = exec.Command(cmd[0], cmd[1:]...)

	return nil
}

// Start executes the command specified in service definition
func (sv *Unit) Start() (err error) {
	switch sv.Definition.Service.Type {
	case "simple":
		if err = sv.Cmd.Start(); err == nil {
			go sv.Cmd.Wait()
		}
		return
	case "oneshot":
		return sv.Cmd.Run()
	default:
		panic("Unknown service type")
	}
}

// Stop stops execution of the command specified in service definition
func (sv *Unit) Stop() (err error) {
	if sv.Cmd.Process == nil {
		return unit.ErrNotStarted
	}

	return sv.Process.Kill()
}

// Sub reports the sub status of a service
func (sv *Unit) Sub() string {
	if sv.Cmd == nil {
		panic(unit.ErrNotParsed)
	}

	switch {
	case sv.Cmd.Process == nil:
		// Service has not been started yet
		return dead

	case sv.Cmd.ProcessState == nil:
		// Wait has not returned yet
		return running

	case sv.ProcessState.Exited(), sv.ProcessState.Success():
		if sv.Definition.Service.RemainAfterExit {
			return exited
		}
		return dead

	default:
		// Service process has finished, but did not return a 0 exit code
		return failed
	}
}

// Active reports activation status of a service
func (sv *Unit) Active() unit.Activation {
	// based of Systemd transtition table found in https://goo.gl/oEjikJ
	switch sv.Sub() {
	case dead:
		return unit.Inactive
	case failed:
		return unit.Failed
	case reload:
		return unit.Reloading
	case running, exited:
		return unit.Active
	case start, startPre, startPost, autoRestart:
		return unit.Activating
	case stop, stopSigabrt, stopPost, stopSigkill, stopSigterm, finalSigkill, finalSigterm:
		return unit.Deactivating
	default:
		panic("Unknown service sub state")
	}
}
