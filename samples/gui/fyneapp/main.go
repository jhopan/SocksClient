// Aplikasi contoh dengan Fyne - bentuk yang sama dengan versi Gio.
// Jalankan: go mod tidy && go run .
package main

import (
	"fmt"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"
)

func main() {
	a := app.New()
	w := a.NewWindow("Socks Client - Fyne")
	w.Resize(fyne.NewSize(420, 520))

	host := widget.NewEntry()
	host.SetPlaceHolder("10.0.0.1")
	port := widget.NewEntry()
	port.SetText("1080")
	user := widget.NewEntry()
	pass := widget.NewPasswordEntry()

	status := widget.NewLabel("Status: Disconnected")
	form := widget.NewForm(
		widget.NewFormItem("Host", host),
		widget.NewFormItem("Port", port),
		widget.NewFormItem("Username", user),
		widget.NewFormItem("Password", pass),
	)

	connected := false
	var btn *widget.Button
	btn = widget.NewButton("Connect", func() {
		connected = !connected
		if connected {
			btn.SetText("Disconnect")
			status.SetText(fmt.Sprintf("Status: Connected (TUN) %s:%s", host.Text, port.Text))
		} else {
			btn.SetText("Connect")
			status.SetText("Status: Disconnected")
		}
	})

	w.SetContent(container.NewVBox(form, btn, status))
	w.ShowAndRun()
}
