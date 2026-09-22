//go:build darwin && cgo

// Jembatan AppKit -> guicore. Preamble hanya deklarasi (syarat cgo saat ada
// //export); definisi C/ObjC-nya di cocoa_c.go.
package main

/*
extern void goToggle(void);
extern void goPingToggled(int on);
extern void goTick(void);
*/
import "C"

import "socks-client-desktop/internal/guicore"

var state *guicore.State

//export goToggle
func goToggle() { state.Toggle() }

//export goPingToggled
func goPingToggled(on C.int) { state.PingToggled(on == 1) }

//export goTick
func goTick() { state.Tick() }

// macUI memenuhi guicore.UI dengan widget AppKit.
type macUI struct{}

func (macUI) FieldHost() string { return fieldHost() }
func (macUI) FieldPort() string { return fieldPort() }
func (macUI) FieldUser() string { return fieldUser() }
func (macUI) FieldPass() string { return fieldPass() }
func (macUI) PingChecked() bool { return pingChecked() }

func (macUI) SetStatus(s string) { setStatus(s) }
func (macUI) SetLog(s string)    { setLog(s) }
func (macUI) SetRunning(r bool)  { setRunningUI(r) }
func (macUI) ShowError(s string) { showError(s) }

func (macUI) SetFields(host, port, user, pass string) { setFields(host, port, user, pass) }
func (macUI) SetPingChecked(on bool)                  { setPingChecked(on) }
