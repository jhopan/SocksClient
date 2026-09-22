// Aplikasi contoh dengan Gio - bentuk yang sama dengan versi Fyne.
// Jalankan: go mod tidy && go run .
package main

import (
	"fmt"
	"os"

	"gioui.org/app"
	"gioui.org/font/gofont"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/text"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"
)

func main() {
	go func() {
		w := new(app.Window)
		w.Option(app.Title("Socks Client - Gio"), app.Size(unit.Dp(420), unit.Dp(520)))

		th := material.NewTheme()
		th.Shaper = text.NewShaper(text.WithCollection(gofont.Collection()))

		var (
			ops                    op.Ops
			host, port, user, pass widget.Editor
			btn                    widget.Clickable
		)
		port.SetText("1080")

		connected := false
		status := "Status: Disconnected"

		for {
			e := w.Event()
			switch e := e.(type) {
			case app.DestroyEvent:
				os.Exit(0)
			case app.FrameEvent:
				gtx := app.NewContext(&ops, e)

				if btn.Clicked(gtx) {
					connected = !connected
					if connected {
						status = fmt.Sprintf("Status: Connected (TUN) %s:%s", host.Text(), port.Text())
					} else {
						status = "Status: Disconnected"
					}
				}
				label := "Connect"
				if connected {
					label = "Disconnect"
				}

				// Tata letak digambar manual: tidak ada NewForm, tidak ada VBox.
				layout.UniformInset(unit.Dp(16)).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
						layout.Rigid(row(gtx, th, "Host", material.Editor(th, &host, "10.0.0.1").Layout)),
						layout.Rigid(row(gtx, th, "Port", material.Editor(th, &port, "1080").Layout)),
						layout.Rigid(row(gtx, th, "Username", material.Editor(th, &user, "").Layout)),
						layout.Rigid(row(gtx, th, "Password", material.Editor(th, &pass, "").Layout)),
						layout.Rigid(spacer(unit.Dp(12))),
						layout.Rigid(material.Button(th, &btn, label).Layout),
						layout.Rigid(spacer(unit.Dp(8))),
						layout.Rigid(material.Label(th, unit.Sp(13), status).Layout),
					)
				})

				e.Frame(gtx.Ops)
			}
		}
	}()
	app.Main()
}

// row: satu label kecil di atas field-nya.
func row(gtx layout.Context, th *material.Theme, name string, field layout.Widget) layout.Widget {
	return func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
			layout.Rigid(material.Label(th, unit.Sp(11), name).Layout),
			layout.Rigid(field),
			layout.Rigid(spacer(unit.Dp(8))),
		)
	}
}

func spacer(h unit.Dp) layout.Widget {
	return func(gtx layout.Context) layout.Dimensions {
		return layout.Spacer{Height: h}.Layout(gtx)
	}
}
