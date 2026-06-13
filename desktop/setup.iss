; Socks Client Desktop — Inno Setup Script
; Build: Compile this script with Inno Setup Compiler (free)
; Download: https://jrsoftware.org/isdl.php

#define MyAppName "Socks Client Desktop"
#define MyAppVersion "1.0.0"
#define MyAppPublisher "JhopanStore"
#define MyAppExeName "socks-client.exe"
#define MyAppURL "https://github.com/jhopan/SocksClientByJhopanStore"

[Setup]
AppId={{A1B2C3D4-E5F6-7890-ABCD-EF1234567890}
AppName={#MyAppName}
AppVersion={#MyAppVersion}
AppPublisher={#MyAppPublisher}
AppPublisherURL={#MyAppURL}
AppSupportURL={#MyAppURL}
AppUpdatesURL={#MyAppURL}
DefaultDirName={autopf}\SocksClientDesktop
DefaultGroupName={#MyAppName}
AllowNoIcons=yes
LicenseFile=
OutputDir=installer_output
OutputBaseFilename=SocksClientDesktop_Setup_v{#MyAppVersion}
SetupIconFile=app.ico
Compression=lzma2/ultra64
SolidCompression=yes
WizardStyle=modern
PrivilegesRequired=admin
ArchitecturesInstallIn64BitMode=x64
UninstallDisplayIcon={app}\{#MyAppExeName}

; Dark theme colors
WizardSmallImageFile=
WizardImageFile=

[Languages]
Name: "english"; MessagesFile: "compiler:Default.isl"
Name: "indonesian"; MessagesFile: "compiler:Languages\Indonesian.isl"

[Tasks]
Name: "desktopicon"; Description: "Create a &desktop icon"; GroupDescription: "Additional icons:"
Name: "autostart"; Description: "Start with &Windows"; GroupDescription: "Startup:"

[Files]
Source: "socks-client.exe"; DestDir: "{app}"; Flags: ignoreversion

[Icons]
Name: "{group}\{#MyAppName}"; Filename: "{app}\{#MyAppExeName}"
Name: "{group}\Uninstall {#MyAppName}"; Filename: "{uninstallexe}"
Name: "{autodesktop}\{#MyAppName}"; Filename: "{app}\{#MyAppExeName}"; Tasks: desktopicon
Name: "{commonstartup}\{#MyAppName}"; Filename: "{app}\{#MyAppExeName}"; Tasks: autostart

[Run]
Filename: "{app}\{#MyAppExeName}"; Description: "Launch {#MyAppName}"; Flags: nowait postinstall skipifsilent runasoriginaluser

[UninstallDelete]
Type: files; Name: "{app}\socks_client_config.json"
Type: files; Name: "{app}\settings.json"
Type: files; Name: "{app}\sing-box.exe"

[Code]
// Check if WebView2 runtime is installed
function IsWebView2Installed(): Boolean;
var
  Installed: Cardinal;
begin
  Result := RegQueryDWordValue(HKLM, 'SOFTWARE\WOW6432Node\Microsoft\EdgeUpdate\Clients\{F3017226-FE2A-4295-8BDF-00C3A9A7E4C5}', 'pv', Installed) or
            RegQueryDWordValue(HKCU, 'SOFTWARE\Microsoft\EdgeUpdate\Clients\{F3017226-FE2A-4295-8BDF-00C3A9A7E4C5}', 'pv', Installed);
end;

function InitializeSetup(): Boolean;
begin
  Result := True;
  if not IsWebView2Installed() then
  begin
    MsgBox('WebView2 Runtime belum terinstall.' + #13#10 +
           'Download dari: https://go.microsoft.com/fwlink/p/?LinkId=2124703' + #13#10#13#10 +
           'Install WebView2 dulu, lalu jalankan Setup ini lagi.',
           mbCriticalError, MB_OK);
    Result := False;
  end;
end;
