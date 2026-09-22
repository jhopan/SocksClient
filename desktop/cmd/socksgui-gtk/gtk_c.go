//go:build linux && cgo

// GUI Linux: GTK3 dipanggil langsung lewat cgo, tanpa toolkit Go.
// Sama filosofinya dengan aplikasi Windows (walk memakai user32/comctl32):
// widget milik OS, binary kecil (~1 MB), RAM rendah karena library GTK sudah
// dimuat sesi desktop.
package main

/*
#cgo pkg-config: gtk+-3.0
#include <gtk/gtk.h>
#include <stdlib.h>
#include <string.h>

// Fungsi Go yang dipanggil dari callback GTK (semuanya di main thread).
extern void goToggle(void);
extern void goPingToggled(int on);
extern void goTick(void);

static GtkWidget *win, *entryHost, *entryPort, *entryUser, *entryPass,
                 *chkPing, *btnToggle, *lblStatus, *logView;

static const char* entry_host_text(void)  { return gtk_entry_get_text(GTK_ENTRY(entryHost)); }
static const char* entry_port_text(void)  { return gtk_entry_get_text(GTK_ENTRY(entryPort)); }
static const char* entry_user_text(void)  { return gtk_entry_get_text(GTK_ENTRY(entryUser)); }
static const char* entry_pass_text(void)  { return gtk_entry_get_text(GTK_ENTRY(entryPass)); }
static int ping_checked(void)             { return gtk_toggle_button_get_active(GTK_TOGGLE_BUTTON(chkPing)) ? 1 : 0; }

static void set_field_host(const char* s) { gtk_entry_set_text(GTK_ENTRY(entryHost), s ? s : ""); }
static void set_field_port(const char* s) { gtk_entry_set_text(GTK_ENTRY(entryPort), s ? s : ""); }
static void set_field_user(const char* s) { gtk_entry_set_text(GTK_ENTRY(entryUser), s ? s : ""); }
static void set_field_pass(const char* s) { gtk_entry_set_text(GTK_ENTRY(entryPass), s ? s : ""); }
static void set_ping_checked(int on)      { gtk_toggle_button_set_active(GTK_TOGGLE_BUTTON(chkPing), on ? TRUE : FALSE); }
static void set_running_ui(int running) {
    gtk_button_set_label(GTK_BUTTON(btnToggle), running ? "Disconnect" : "Connect");
}
static void set_status_text(const char* s) { gtk_label_set_text(GTK_LABEL(lblStatus), s ? s : ""); }
static void set_log_text(const char* s) {
    GtkTextBuffer *buf = gtk_text_view_get_buffer(GTK_TEXT_VIEW(logView));
    gtk_text_buffer_set_text(buf, s ? s : "", -1);
}

static void on_toggle(GtkWidget *w, gpointer d) { (void)w; (void)d; goToggle(); }
static void on_ping(GtkWidget *w, gpointer d) {
    (void)d;
    goPingToggled(gtk_toggle_button_get_active(GTK_TOGGLE_BUTTON(w)) ? 1 : 0);
}
static gboolean on_tick(gpointer d) { (void)d; goTick(); return TRUE; }

static GtkWidget* make_grid(void) {
    GtkWidget *grid = gtk_grid_new();
    gtk_grid_set_row_spacing(GTK_GRID(grid), 6);
    gtk_grid_set_column_spacing(GTK_GRID(grid), 10);
    gtk_widget_set_margin_start(grid, 14);
    gtk_widget_set_margin_end(grid, 14);
    gtk_widget_set_margin_top(grid, 12);

    const char *labels[4] = {"Host/IP", "Port", "Username", "Password"};
    GtkWidget **entries[4] = {&entryHost, &entryPort, &entryUser, &entryPass};
    for (int i = 0; i < 4; i++) {
        GtkWidget *lbl = gtk_label_new(labels[i]);
        gtk_widget_set_halign(lbl, GTK_ALIGN_START);
        *entries[i] = gtk_entry_new();
        gtk_widget_set_hexpand(*entries[i], TRUE);
        gtk_widget_set_size_request(*entries[i], 260, -1);
        gtk_grid_attach(GTK_GRID(grid), lbl, 0, i, 1, 1);
        gtk_grid_attach(GTK_GRID(grid), *entries[i], 1, i, 1, 1);
    }
    gtk_entry_set_placeholder_text(GTK_ENTRY(entryHost), "10.0.0.1");
    gtk_entry_set_visibility(GTK_ENTRY(entryPass), FALSE);
    return grid;
}

static void ui_init(void) {
    gtk_init(NULL, NULL);

    win = gtk_window_new(GTK_WINDOW_TOPLEVEL);
    gtk_window_set_title(GTK_WINDOW(win), "Socks Client");
    gtk_window_set_default_size(GTK_WINDOW(win), 460, 560);
    gtk_window_set_position(GTK_WINDOW(win), GTK_WIN_POS_CENTER);
    g_signal_connect(win, "destroy", G_CALLBACK(gtk_main_quit), NULL);

    GtkWidget *box = gtk_box_new(GTK_ORIENTATION_VERTICAL, 8);
    gtk_container_add(GTK_CONTAINER(win), box);

    GtkWidget *title = gtk_label_new(NULL);
    gtk_label_set_markup(GTK_LABEL(title), "<b>Socks Client</b>  <small>by JhopanStore</small>");
    gtk_widget_set_margin_top(title, 10);
    gtk_box_pack_start(GTK_BOX(box), title, FALSE, FALSE, 0);
    gtk_box_pack_start(GTK_BOX(box), make_grid(), FALSE, FALSE, 0);

    chkPing = gtk_check_button_new_with_label("HTTP ping (cek jalur + keep-alive)");
    gtk_widget_set_margin_start(chkPing, 14);
    g_signal_connect(chkPing, "toggled", G_CALLBACK(on_ping), NULL);
    gtk_box_pack_start(GTK_BOX(box), chkPing, FALSE, FALSE, 0);

    btnToggle = gtk_button_new_with_label("Connect");
    gtk_widget_set_margin_start(btnToggle, 14);
    gtk_widget_set_margin_end(btnToggle, 14);
    g_signal_connect(btnToggle, "clicked", G_CALLBACK(on_toggle), NULL);
    gtk_box_pack_start(GTK_BOX(box), btnToggle, FALSE, FALSE, 0);

    lblStatus = gtk_label_new("Disconnected");
    gtk_widget_set_halign(lblStatus, GTK_ALIGN_START);
    gtk_widget_set_margin_start(lblStatus, 14);
    gtk_box_pack_start(GTK_BOX(box), lblStatus, FALSE, FALSE, 0);

    GtkWidget *logLabel = gtk_label_new("Log core");
    gtk_widget_set_halign(logLabel, GTK_ALIGN_START);
    gtk_widget_set_margin_start(logLabel, 14);
    gtk_box_pack_start(GTK_BOX(box), logLabel, FALSE, FALSE, 0);

    logView = gtk_text_view_new();
    gtk_text_view_set_editable(GTK_TEXT_VIEW(logView), FALSE);
    gtk_text_view_set_monospace(GTK_TEXT_VIEW(logView), TRUE);
    GtkWidget *scroll = gtk_scrolled_window_new(NULL, NULL);
    gtk_widget_set_size_request(scroll, -1, 200);
    gtk_widget_set_margin_start(scroll, 14);
    gtk_widget_set_margin_end(scroll, 14);
    gtk_widget_set_margin_bottom(scroll, 12);
    gtk_container_add(GTK_CONTAINER(scroll), logView);
    gtk_box_pack_start(GTK_BOX(box), scroll, TRUE, TRUE, 0);

    g_timeout_add(1000, on_tick, NULL);
    gtk_widget_show_all(win);
}

static void msg_error(const char* text) {
    GtkWidget *d = gtk_message_dialog_new(GTK_WINDOW(win), GTK_DIALOG_MODAL,
        GTK_MESSAGE_ERROR, GTK_BUTTONS_OK, "%s", text);
    gtk_dialog_run(GTK_DIALOG(d));
    gtk_widget_destroy(d);
}

static void ui_run(void) { gtk_main(); }
*/
import "C"

import "unsafe"

// --- jembatan Go <-> C -------------------------------------------------------

func uiInit()            { C.ui_init() }
func uiRun()             { C.ui_run() }
func setStatus(s string) { cs := C.CString(s); C.set_status_text(cs); C.free(unsafe.Pointer(cs)) }
func setLog(s string)    { cs := C.CString(s); C.set_log_text(cs); C.free(unsafe.Pointer(cs)) }
func setRunningUI(r bool) {
	if r {
		C.set_running_ui(1)
	} else {
		C.set_running_ui(0)
	}
}
func showError(s string) { cs := C.CString(s); C.msg_error(cs); C.free(unsafe.Pointer(cs)) }

func fieldHost() string { return C.GoString(C.entry_host_text()) }
func fieldPort() string { return C.GoString(C.entry_port_text()) }
func fieldUser() string { return C.GoString(C.entry_user_text()) }
func fieldPass() string { return C.GoString(C.entry_pass_text()) }
func pingChecked() bool { return C.ping_checked() == 1 }

func setFields(host, port, user, pass string) {
	h, p := C.CString(host), C.CString(port)
	u, s := C.CString(user), C.CString(pass)
	C.set_field_host(h)
	C.set_field_port(p)
	C.set_field_user(u)
	C.set_field_pass(s)
	C.free(unsafe.Pointer(h))
	C.free(unsafe.Pointer(p))
	C.free(unsafe.Pointer(u))
	C.free(unsafe.Pointer(s))
}

func setPingChecked(on bool) {
	if on {
		C.set_ping_checked(1)
	} else {
		C.set_ping_checked(0)
	}
}
