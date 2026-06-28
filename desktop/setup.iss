; Socks Client Desktop — Inno Setup Script
; Build: Compile with Inno Setup Compiler (free)
; Download: https://jrsoftware.org/isdl.php

#define MyAppName "Socks Client Desktop"
#define MyAppVersion "1.1.0"
#define MyAppPublisher "JhopanStore"
#define MyAppExeName "socks-client.exe"
#define MyAppURL "https://github.com/jhopan/SocksClientByJhopanStore"

[Setup]
AppId={{A1B2C3D4-E5F6-7890-ABCD-EF1234567890}
AppName={#MyAppName}
AppVersion={#MyAppVersion}
AppVerName={#MyAppName} {#MyAppVersion}
AppPublisher={#MyAppPublisher}
AppPublisherURL={#MyAppURL}
AppSupportURL={#MyAppURL}
AppUpdatesURL={#MyAppURL}
DefaultDirName={autopf}\SocksClientDesktop
DefaultGroupName={#MyAppName}
AllowNoIcons=yes
OutputDir=installer_output
OutputBaseFilename=SocksClientDesktop_Setup_v{#MyAppVersion}
VersionInfoVersion={#MyAppVersion}.0
SetupIconFile=app.ico
Compression=lzma2/ultra64
SolidCompression=yes
WizardStyle=modern
PrivilegesRequired=admin
UninstallDisplayIcon={app}\{#MyAppExeName}
UsePreviousAppDir=yes
DisableDirPage=auto
CloseApplications=force
AppMutex=SocksClientDesktopMutex

[Languages]
Name: "english"; MessagesFile: "compiler:Default.isl"

[Tasks]
Name: "desktopicon"; Description: "Create a &desktop icon"; GroupDescription: "Additional icons:"
Name: "autostart"; Description: "Start with &Windows"; GroupDescription: "Startup:"

[Files]
Source: "socks-client.exe"; DestDir: "{app}"; Flags: ignoreversion
Source: "embed/sing-box.exe"; DestDir: "{app}"; Flags: ignoreversion

[Icons]
Name: "{group}\{#MyAppName}"; Filename: "{app}\{#MyAppExeName}"
Name: "{group}\Uninstall {#MyAppName}"; Filename: "{uninstallexe}"
Name: "{autodesktop}\{#MyAppName}"; Filename: "{app}\{#MyAppExeName}"; Tasks: desktopicon
Name: "{commonstartup}\{#MyAppName}"; Filename: "{app}\{#MyAppExeName}"; Tasks: autostart

[Run]
Filename: "{app}\{#MyAppExeName}"; Description: "Launch {#MyAppName}"; Verb: runas; Flags: nowait postinstall skipifsilent shellexec

[UninstallDelete]
Type: files; Name: "{app}\config.json"
Type: files; Name: "{app}\settings.json"
Type: files; Name: "{app}\sing-box.exe"
Type: files; Name: "{app}\sing-box.log"
Type: files; Name: "{app}\.lock"
