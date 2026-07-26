#ifndef Architecture
  #define Architecture "amd64"
#endif

#define AppName "VoxSteer"
#ifndef AppVersion
  #define AppVersion "0.1.0"
#endif
#define BundleDir AddBackslash(SourcePath) + "..\dist\windows-" + Architecture

#if Architecture == "amd64"
  #define ArchitectureAllowed "x64os"
  #define ArchitectureLabel "x64"
  #define LocalDescription "Install local NVIDIA CUDA transcription for an RTX-class GPU (downloads 547 MiB model)"
  #define ModelFile "ggml-large-v3-turbo-q5_0.bin"
  #define ModelURL "https://huggingface.co/ggerganov/whisper.cpp/resolve/main/ggml-large-v3-turbo-q5_0.bin"
  #define ModelSHA256 "394221709cd5ad1f40c46e6031ca61bce88931e6e088c188294c6d5a55ffa7e2"
  #define ModelSize "574041195"
  #define RuntimeURL "https://github.com/ggml-org/whisper.cpp/releases/download/v1.9.1/whisper-cublas-12.4.0-bin-x64.zip"
  #define RuntimeSHA256 "106a2030eff8998e4ef320fe72e263a78449e9040386ee27c41ea80b001b601b"
  #define RuntimeSize "1209487872"
#elif Architecture == "arm64"
  #define ArchitectureAllowed "arm64"
  #define ArchitectureLabel "ARM64"
  #define LocalDescription "Install local ARM64 CPU transcription (downloads 142 MiB English model)"
  #define ModelFile "ggml-base.en.bin"
  #define ModelURL "https://huggingface.co/ggerganov/whisper.cpp/resolve/main/ggml-base.en.bin"
  #define ModelSHA256 "a03779c86df3323075f5e796cb2ce5029f00ec8869eee3fdfb897afe36c6d002"
  #define ModelSize "147964211"
#else
  #error Unsupported Architecture
#endif

[Setup]
AppId={{D187A695-A6A8-49C3-BDBA-CB13855AF45A}
AppName={#AppName}
AppVersion={#AppVersion}
AppVerName={#AppName} {#AppVersion}
AppPublisher=VoxSteer
ArchitecturesAllowed={#ArchitectureAllowed}
DefaultDirName={localappdata}\Programs\VoxSteer
DefaultGroupName={#AppName}
DisableProgramGroupPage=yes
OutputDir={#SourcePath}\..\dist\installers
OutputBaseFilename=voxsteer-{#AppVersion}-windows-{#Architecture}
PrivilegesRequired=lowest
Compression=lzma2/max
SolidCompression=yes
WizardStyle=modern
ArchiveExtraction=full
UninstallDisplayName={#AppName}
CloseApplications=yes
CloseApplicationsFilter=voxsteer.exe

[Tasks]
Name: "local"; Description: "{#LocalDescription}"; GroupDescription: "Transcription engine:"; Flags: unchecked
Name: "startup"; Description: "Start VoxSteer when I sign in"; GroupDescription: "Windows integration:"; Flags: unchecked

[Files]
Source: "{#BundleDir}\voxsteer.exe"; DestDir: "{app}"; Flags: ignoreversion
#if Architecture == "amd64"
Source: "{#RuntimeURL}"; DestName: "whisper-cuda.zip"; DestDir: "{app}\whisper"; Tasks: local; Hash: "{#RuntimeSHA256}"; ExternalSize: {#RuntimeSize}; Flags: external download extractarchive ignoreversion recursesubdirs createallsubdirs
#else
Source: "{#BundleDir}\whisper\*"; DestDir: "{app}\whisper"; Tasks: local; Flags: ignoreversion recursesubdirs createallsubdirs
#endif
Source: "{#ModelURL}"; DestName: "{#ModelFile}"; DestDir: "{app}\models"; Tasks: local; Hash: "{#ModelSHA256}"; ExternalSize: {#ModelSize}; Flags: external download ignoreversion

[Icons]
Name: "{group}\VoxSteer"; Filename: "{app}\voxsteer.exe"; WorkingDir: "{app}"
Name: "{autostartup}\VoxSteer"; Filename: "{app}\voxsteer.exe"; WorkingDir: "{app}"; Tasks: startup

[Run]
Filename: "{app}\voxsteer.exe"; Description: "Launch VoxSteer"; Flags: postinstall nowait skipifsilent runasoriginaluser

[Code]
var
  CommonPage: TInputQueryWizardPage;
  ActivationPage: TInputOptionWizardPage;
  CloudPage: TInputQueryWizardPage;
  ExistingConfig: Boolean;

function ConfigPath: String;
begin
  Result := ExpandConstant('{userappdata}\VoxSteer\config.json');
end;

function JsonEscape(Value: String): String;
begin
  Result := Value;
  StringChangeEx(Result, '\', '\\', True);
  StringChangeEx(Result, '"', '\"', True);
  StringChangeEx(Result, #13, '\r', True);
  StringChangeEx(Result, #10, '\n', True);
end;

procedure InitializeWizard;
begin
  ExistingConfig := FileExists(ConfigPath);

  CommonPage := CreateInputQueryPage(
    wpSelectTasks,
    'Dictation settings',
    'Choose the transcription language.',
    'Use an ISO language code such as en, fr, de, or leave it blank for automatic detection.'
  );
  CommonPage.Add('Language:', False);
  CommonPage.Values[0] := 'en';

  ActivationPage := CreateInputOptionPage(
    CommonPage.ID,
    'Activation',
    'How would you like to start and stop dictation?',
    'Choose a global keyboard shortcut, or use the VoxSteer tray menu with your mouse.',
    True,
    False
  );
  ActivationPage.Add('Ctrl+Alt+Space (recommended)');
  ActivationPage.Add('Ctrl+Shift+D');
  ActivationPage.Add('F8');
  ActivationPage.Add('Mouse / tray only');
  ActivationPage.SelectedValueIndex := 0;

  CloudPage := CreateInputQueryPage(
    ActivationPage.ID,
    'Cloud transcription',
    'Configure an OpenAI-compatible transcription endpoint.',
    'These settings are stored only in your Windows profile.'
  );
  CloudPage.Add('Endpoint:', False);
  CloudPage.Add('API key (optional for endpoints without authentication):', True);
  CloudPage.Add('Model:', False);
  CloudPage.Values[0] := 'https://api.openai.com/v1/audio/transcriptions';
  CloudPage.Values[2] := 'whisper-1';
end;

function ShouldSkipPage(PageID: Integer): Boolean;
begin
  Result := False;
  if ExistingConfig and
     ((PageID = CommonPage.ID) or
      (PageID = ActivationPage.ID) or
      (PageID = CloudPage.ID)) then
    Result := True
  else if (PageID = CloudPage.ID) and WizardIsTaskSelected('local') then
    Result := True;
end;

function NextButtonClick(CurPageID: Integer): Boolean;
begin
  Result := True;
  if (CurPageID = CloudPage.ID) and (Trim(CloudPage.Values[0]) = '') then
  begin
    MsgBox('Enter a cloud transcription endpoint or go back and select local transcription.', mbError, MB_OK);
    Result := False;
  end;
end;

procedure WriteConfiguration;
var
  Lines: TArrayOfString;
  Engine: String;
  Shortcut: String;
  ConfigJSON: String;
begin
  if ExistingConfig then
    Exit;

  if WizardIsTaskSelected('local') then
    Engine := 'local'
  else
    Engine := 'cloud';

  case ActivationPage.SelectedValueIndex of
    0: Shortcut := 'ctrl+alt+space';
    1: Shortcut := 'ctrl+shift+d';
    2: Shortcut := 'f8';
    3: Shortcut := 'disabled';
  end;

  ConfigJSON :=
    '{' +
    '"engine":"' + Engine + '",' +
    '"language":"' + JsonEscape(CommonPage.Values[0]) + '",' +
    '"shortcut":"' + Shortcut + '",' +
    '"local":{' +
      '"binary":"' + JsonEscape(ExpandConstant('{app}\whisper\Release\whisper-cli.exe')) + '",' +
      '"model":"' + JsonEscape(ExpandConstant('{app}\models\{#ModelFile}')) + '"' +
    '},' +
    '"cloud":{' +
      '"endpoint":"' + JsonEscape(CloudPage.Values[0]) + '",' +
      '"api_key":"' + JsonEscape(CloudPage.Values[1]) + '",' +
      '"model":"' + JsonEscape(CloudPage.Values[2]) + '"' +
    '}' +
    '}';

  ForceDirectories(ExtractFileDir(ConfigPath));
  SetArrayLength(Lines, 1);
  Lines[0] := ConfigJSON;
  if not SaveStringsToUTF8FileWithoutBOM(ConfigPath, Lines, False) then
    RaiseException('Could not save VoxSteer configuration.');
end;

procedure CurStepChanged(CurStep: TSetupStep);
begin
  if CurStep = ssPostInstall then
    WriteConfiguration;
end;
