//go:build linux && cgo

// Jembatan GTK -> guicore. Preamble hanya berisi deklarasi (syarat cgo saat ada
// //export); definisi C-nya di gtk_c.go.
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

// gtkUI memenuhi guicore.UI dengan widget GTK.
type gtkUI struct{}

func (gtkUI) FieldHost() string { return fieldHost() }
func (gtkUI) FieldPort() string { return fieldPort() }
func (gtkUI) FieldUser() string { return fieldUser() }
func (gtkUI) FieldPass() string { return fieldPass() }
func (gtkUI) PingChecked() bool { return pingChecked() }

func (gtkUI) SetStatus(s string) { setStatus(s) }
func (gtkUI) SetLog(s string)    { setLog(s) }
func (gtkUI) SetRunning(r bool)  { setRunningUI(r) }
func (gtkUI) ShowError(s string) { showError(s) }

func (gtkUI) SetFields(host, port, user, pass string) { setFields(host, port, user, pass) }
func (gtkUI) SetPingChecked(on bool)                  { setPingChecked(on) }
