#define MyAppName "uBlock DNS"
#define MyAppPublisher "uBlock DNS"
#define MyAppExeName "ublockdns.exe"

#ifndef MyAppVersion
  #define MyAppVersion "dev"
#endif

#ifndef Arch
  #define Arch "amd64"
#endif

#ifndef SourceExe
  #define SourceExe "ublockdns.exe"
#endif

#ifndef SourceInstallPs1
  #define SourceInstallPs1 "install.ps1"
#endif

#ifndef SourceSetupPs1
  #define SourceSetupPs1 "setup.ps1"
#endif

#ifndef OutputBaseFilename
  #define OutputBaseFilename "uBlockDNS-Setup"
#endif

[Setup]
AppId={{4D1E6B6B-20D2-4D0E-9E77-D06EA5C60CD3}
AppName={#MyAppName}
AppVersion={#MyAppVersion}
AppPublisher={#MyAppPublisher}
DefaultDirName={autopf}\\uBlockDNS
DefaultGroupName=uBlock DNS
DisableDirPage=yes
DisableProgramGroupPage=yes
OutputDir=.
OutputBaseFilename={#OutputBaseFilename}
Compression=lzma
SolidCompression=yes
WizardStyle=modern
ArchitecturesAllowed=x64 arm64
ArchitecturesInstallIn64BitMode=x64 arm64
PrivilegesRequired=admin
UninstallDisplayIcon={app}\\{#MyAppExeName}

[Languages]
Name: "english"; MessagesFile: "compiler:Default.isl"

[Files]
Source: "{#SourceExe}"; DestDir: "{app}"; DestName: "{#MyAppExeName}"; Flags: ignoreversion
Source: "{#SourceInstallPs1}"; DestDir: "{app}"; Flags: ignoreversion
Source: "{#SourceSetupPs1}"; DestDir: "{app}"; Flags: ignoreversion

[Icons]
Name: "{autoprograms}\\uBlock DNS\\uBlock DNS Status"; Filename: "{app}\\{#MyAppExeName}"; Parameters: "status"
Name: "{autoprograms}\\uBlock DNS\\uBlock DNS Guided Setup"; Filename: "powershell.exe"; Parameters: "-ExecutionPolicy Bypass -File ""{app}\\setup.ps1"""
Name: "{autoprograms}\\uBlock DNS\\Uninstall uBlock DNS"; Filename: "{uninstallexe}"

[Run]
Filename: "powershell.exe"; Parameters: "-ExecutionPolicy Bypass -File ""{app}\\setup.ps1"""; Description: "Run guided setup now"; Flags: postinstall skipifsilent unchecked
