//go:build darwin && cgo

// GUI macOS: AppKit dipanggil langsung lewat cgo/ObjC, tanpa binding pihak
// ketiga. Filosofinya sama dengan walk di Windows: widget milik OS (NSButton,
// NSTextField) sehingga binary kecil (~1 MB) dan RAM rendah karena AppKit sudah
// dimuat sistem.
package main

/*
#cgo CFLAGS: -x objective-c
#cgo LDFLAGS: -framework Cocoa -framework Foundation
#import <Cocoa/Cocoa.h>

extern void goToggle(void);
extern void goPingToggled(int on);
extern void goTick(void);

@class SocksDelegate;

static NSWindow *win;
static NSTextField *fHost, *fPort, *fUser, *fPass, *lblStatus;
static NSButton *chkPing, *btnToggle;
static NSTextView *logView;
static SocksDelegate *delegate;

static NSString* str(const char* s) { return [NSString stringWithUTF8String:(s ? s : "")]; }

@interface SocksDelegate : NSObject <NSApplicationDelegate>
- (void)onToggle:(id)sender;
- (void)onPing:(id)sender;
- (void)tick:(NSTimer*)timer;
@end

@implementation SocksDelegate
- (void)onToggle:(id)sender { (void)sender; goToggle(); }
- (void)onPing:(id)sender { (void)sender; goPingToggled([chkPing state] == NSControlStateValueOn ? 1 : 0); }
- (void)tick:(NSTimer*)timer { (void)timer; goTick(); }
- (BOOL)applicationShouldTerminateAfterLastWindowClosed:(NSApplication*)app { (void)app; return YES; }
@end

static void create_delegate(void) { delegate = [[SocksDelegate alloc] init]; }

static void set_field_host(const char* s) { [fHost setStringValue:str(s)]; }
static void set_field_port(const char* s) { [fPort setStringValue:str(s)]; }
static void set_field_user(const char* s) { [fUser setStringValue:str(s)]; }
static void set_field_pass(const char* s) { [fPass setStringValue:str(s)]; }
static void set_ping_checked(int on) { [chkPing setState:(on ? NSControlStateValueOn : NSControlStateValueOff)]; }
static void set_running_ui(int running) { [btnToggle setTitle:str(running ? "Disconnect" : "Connect")]; }
static void set_status_text(const char* s) { [lblStatus setStringValue:str(s)]; }
static void set_log_text(const char* s) { [logView setString:str(s)]; }

static const char* entry_host_text(void) { return [[fHost stringValue] UTF8String]; }
static const char* entry_port_text(void) { return [[fPort stringValue] UTF8String]; }
static const char* entry_user_text(void) { return [[fUser stringValue] UTF8String]; }
static const char* entry_pass_text(void) { return [[fPass stringValue] UTF8String]; }
static int ping_checked(void) { return [chkPing state] == NSControlStateValueOn ? 1 : 0; }

static NSTextField* make_field(NSView *parent, const char *label, CGFloat y, BOOL secure) {
    NSTextField *lbl = [[NSTextField alloc] initWithFrame:NSMakeRect(16, y, 90, 24)];
    [lbl setStringValue:str(label)];
    [lbl setBezeled:NO];
    [lbl setDrawsBackground:NO];
    [lbl setEditable:NO];
    [lbl setSelectable:NO];
    [parent addSubview:lbl];

    NSTextField *field;
    if (secure) {
        field = [[NSSecureTextField alloc] initWithFrame:NSMakeRect(110, y, 320, 24)];
    } else {
        field = [[NSTextField alloc] initWithFrame:NSMakeRect(110, y, 320, 24)];
    }
    [field setBezeled:YES];
    [field setEditable:YES];
    [parent addSubview:field];
    return field;
}

static void ui_init(void) {
    [NSApplication sharedApplication];
    [NSApp setActivationPolicy:NSApplicationActivationPolicyRegular];

    win = [[NSWindow alloc] initWithContentRect:NSMakeRect(0, 0, 460, 560)
        styleMask:(NSWindowStyleMaskTitled | NSWindowStyleMaskClosable | NSWindowStyleMaskMiniaturizable)
        backing:NSBackingStoreBuffered defer:NO];
    [win setTitle:str("Socks Client")];
    NSView *view = [win contentView];

    NSTextField *title = [[NSTextField alloc] initWithFrame:NSMakeRect(16, 520, 420, 24)];
    [title setStringValue:str("Socks Client - by JhopanStore")];
    [title setBezeled:NO];
    [title setDrawsBackground:NO];
    [title setEditable:NO];
    [view addSubview:title];

    fHost = make_field(view, "Host/IP", 480, NO);
    fPort = make_field(view, "Port", 448, NO);
    fUser = make_field(view, "Username", 416, NO);
    fPass = make_field(view, "Password", 384, YES);

    chkPing = [[NSButton alloc] initWithFrame:NSMakeRect(16, 344, 320, 24)];
    [chkPing setButtonType:NSButtonTypeSwitch];
    [chkPing setTitle:str("HTTP ping (cek jalur + keep-alive)")];
    [chkPing setTarget:delegate];
    [chkPing setAction:@selector(onPing:)];
    [view addSubview:chkPing];

    btnToggle = [[NSButton alloc] initWithFrame:NSMakeRect(16, 306, 414, 32)];
    [btnToggle setTitle:str("Connect")];
    [btnToggle setBezelStyle:NSBezelStyleRounded];
    [btnToggle setTarget:delegate];
    [btnToggle setAction:@selector(onToggle:)];
    [view addSubview:btnToggle];

    lblStatus = [[NSTextField alloc] initWithFrame:NSMakeRect(16, 272, 414, 24)];
    [lblStatus setStringValue:str("Disconnected")];
    [lblStatus setBezeled:NO];
    [lblStatus setDrawsBackground:NO];
    [lblStatus setEditable:NO];
    [view addSubview:lblStatus];

    NSScrollView *scroll = [[NSScrollView alloc] initWithFrame:NSMakeRect(16, 16, 414, 244)];
    [scroll setHasVerticalScroller:YES];
    [scroll setBorderType:NSBezelBorder];
    logView = [[NSTextView alloc] initWithFrame:[scroll bounds]];
    [logView setEditable:NO];
    [logView setFont:[NSFont monospacedSystemFontOfSize:11 weight:NSFontWeightRegular]];
    [scroll setDocumentView:logView];
    [view addSubview:scroll];

    [NSTimer scheduledTimerWithTimeInterval:1.0 target:delegate selector:@selector(tick:) userInfo:nil repeats:YES];
    [win center];
    [win makeKeyAndOrderFront:nil];
    [NSApp activateIgnoringOtherApps:YES];
}

static void msg_error(const char* text) {
    NSAlert *alert = [[NSAlert alloc] init];
    [alert setMessageText:str("Socks Client")];
    [alert setInformativeText:str(text)];
    [alert addButtonWithTitle:str("OK")];
    [alert runModal];
}

static void ui_run(void) { [NSApp run]; }
*/
import "C"

import "unsafe"

// --- jembatan Go <-> C -------------------------------------------------------

func createDelegate()    { C.create_delegate() }
func uiRun()             { C.ui_run() }
func setStatus(s string) { cs := C.CString(s); C.set_status_text(cs); C.free(unsafe.Pointer(cs)) }
func setLog(s string)    { cs := C.CString(s); C.set_log_text(cs); C.free(unsafe.Pointer(cs)) }
func showError(s string) { cs := C.CString(s); C.msg_error(cs); C.free(unsafe.Pointer(cs)) }
func setRunningUI(r bool) {
	if r {
		C.set_running_ui(1)
	} else {
		C.set_running_ui(0)
	}
}
func setPingChecked(on bool) {
	if on {
		C.set_ping_checked(1)
	} else {
		C.set_ping_checked(0)
	}
}
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
