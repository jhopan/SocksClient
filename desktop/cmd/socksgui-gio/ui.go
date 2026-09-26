//go:build (linux || darwin || windows) && cgo

package main

import (
	"image/color"

	"gioui.org/app"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"
)

// Palet: 4 warna. Gio tidak punya dark mode otomatis, dan latar jendela harus
// dicat sendiri (paint.Fill) - kalau tidak, jendela tetap putih.
var (
	palet = material.Palette{
		Bg:         color.NRGBA{R: 0x10, G: 0x12, B: 0x16, A: 0xff},
		Fg:         color.NRGBA{R: 0xe8, G: 0xea, B: 0xed, A: 0xff},
		ContrastBg: color.NRGBA{R: 0x2d, G: 0x7d, B: 0xd2, A: 0xff},
		ContrastFg: color.NRGBA{R: 0xff, G: 0xff, B: 0xff, A: 0xff},
	}
	abu   = color.NRGBA{R: 0x8b, G: 0x93, B: 0x9e, A: 0xff}
	hijau = color.NRGBA{R: 0x3d, G: 0xc9, B: 0x7a, A: 0xff}
	merah = color.NRGBA{R: 0xe5, G: 0x6b, B: 0x6b, A: 0xff}
	pink  = color.NRGBA{R: 0xf0, G: 0x6c, B: 0xa8, A: 0xff}
)

// widget (satu instance, satu jendela)
var (
	win                            *app.Window
	ops                            op.Ops
	edHost, edPort, edUser, edPass widget.Editor
	btnTombol                      widget.Clickable
	saklarPing                     widget.Bool
)

// form menggambar isi utama: 4 field, saklar ping, tombol, status.
func form(gtx layout.Context, th *material.Theme) {
	if btnTombol.Clicked(gtx) {
		aksiTombol()
	}
	if saklarPing.Update(gtx) {
		simpanSetelan()
		terapkanPing()
	}

	mu.Lock()
	pesan, warna, cek, jalan := pesanStatus, warnaStatus, sedangCek, en.Running()
	mu.Unlock()

	// Saat tunnel hidup, status diambil dari engine (termasuk hasil ping).
	if jalan {
		pesan, warna = en.Status(), hijau
	}
	nama := "Connect"
	switch {
	case cek:
		nama = "Sedang cek..."
	case jalan:
		nama = "Disconnect"
	}

	layout.UniformInset(unit.Dp(20)).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		baris := []layout.FlexChild{
			layout.Rigid(material.Label(th, unit.Sp(15), "Socks Client").Layout),
			layout.Rigid(teks(th, 11, pink, "jhopanstore - v"+appVersion)),
			layout.Rigid(spacer(unit.Dp(14))),
			layout.Rigid(row(th, "Host", material.Editor(th, &edHost, "IP server").Layout)),
			layout.Rigid(row(th, "Port", material.Editor(th, &edPort, "1080").Layout)),
			layout.Rigid(row(th, "Username (opsional)", material.Editor(th, &edUser, "").Layout)),
			layout.Rigid(row(th, "Password (opsional)", material.Editor(th, &edPass, "").Layout)),
			layout.Rigid(teks(th, 10, abu, "Kosongkan kalau server tidak memakai login")),
			layout.Rigid(spacer(unit.Dp(8))),
			layout.Rigid(material.CheckBox(th, &saklarPing, "HTTP ping").Layout),
			layout.Rigid(spacer(unit.Dp(10))),
			layout.Rigid(material.Button(th, &btnTombol, nama).Layout),
			layout.Rigid(spacer(unit.Dp(10))),
			layout.Rigid(teks(th, 12, warna, pesan)),
		}
		return layout.Flex{Axis: layout.Vertical}.Layout(gtx, baris...)
	})
}

// row: label kecil di atas field.
func row(th *material.Theme, name string, field layout.Widget) layout.Widget {
	return func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
			layout.Rigid(teks(th, 11, abu, name)),
			layout.Rigid(field),
			layout.Rigid(spacer(unit.Dp(6))),
		)
	}
}

func teks(th *material.Theme, sp unit.Sp, c color.NRGBA, s string) layout.Widget {
	l := material.Label(th, sp, s)
	l.Color = c
	return l.Layout
}

func spacer(h unit.Dp) layout.Widget {
	return func(gtx layout.Context) layout.Dimensions { return layout.Spacer{Height: h}.Layout(gtx) }
}
