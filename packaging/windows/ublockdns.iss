#define MyAppName "uBlockDNS"
#define MyAppPublisher "uBlockDNS"
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
DefaultGroupName=uBlockDNS
DisableDirPage=yes
DisableProgramGroupPage=yes
OutputDir=.
OutputBaseFilename={#OutputBaseFilename}
Compression=lzma
SolidCompression=yes
WizardStyle=modern
ArchitecturesAllowed=x64compatible
ArchitecturesInstallIn64BitMode=x64compatible
PrivilegesRequired=admin
UninstallDisplayIcon={app}\\{#MyAppExeName}

[Languages]
Name: "english"; MessagesFile: "compiler:Default.isl"

[Files]
Source: "{#SourceExe}"; DestDir: "{app}"; DestName: "{#MyAppExeName}"; Flags: ignoreversion
Source: "{#SourceInstallPs1}"; DestDir: "{app}"; Flags: ignoreversion
Source: "{#SourceSetupPs1}"; DestDir: "{app}"; Flags: ignoreversion
Source: "..\\..\\scripts\\windows\\common.ps1"; DestDir: "{app}\\scripts\\windows"; Flags: ignoreversion

[Icons]
Name: "{autoprograms}\\uBlockDNS\\uBlockDNS Status"; Filename: "{app}\\{#MyAppExeName}"; Parameters: "status"
Name: "{autoprograms}\\uBlockDNS\\uBlockDNS Guided Setup"; Filename: "powershell.exe"; Parameters: "-ExecutionPolicy Bypass -File ""{app}\\setup.ps1"" -Version ""{#MyAppVersion}"""
Name: "{autoprograms}\\uBlockDNS\\Uninstall uBlockDNS"; Filename: "{uninstallexe}"

[Run]
Filename: "powershell.exe"; Parameters: "-ExecutionPolicy Bypass -File ""{app}\\setup.ps1"" -Version ""{#MyAppVersion}"""; Description: "Run guided setup now (recommended)"; Flags: postinstall skipifsilent
