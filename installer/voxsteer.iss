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
SetupIconFile={#SourcePath}\..\cmd\voxsteer\assets\voxsteer.ico
UninstallDisplayName={#AppName}
CloseApplications=yes
CloseApplicationsFilter=voxsteer.exe

[Tasks]
Name: "local"; Description: "{#LocalDescription}"; GroupDescription: "Transcription engine:"; Flags: unchecked
Name: "startup"; Description: "Start VoxSteer when I sign in"; GroupDescription: "Windows integration:"; Flags: unchecked
Name: "reconfigure"; Description: "Reconfigure VoxSteer (replaces the existing settings)"; GroupDescription: "Existing installation:"; Flags: unchecked; Check: ExistingConfiguration

[Files]
Source: "{#BundleDir}\voxsteer.exe"; DestDir: "{app}"; Flags: ignoreversion
Source: "{#SourcePath}\..\cmd\voxsteer\assets\voxsteer.ico"; DestDir: "{app}"; Flags: ignoreversion
#if Architecture == "amd64"
Source: "{#RuntimeURL}"; DestName: "whisper-cuda.zip"; DestDir: "{app}\whisper"; Tasks: local; Hash: "{#RuntimeSHA256}"; ExternalSize: {#RuntimeSize}; Flags: external download extractarchive ignoreversion recursesubdirs createallsubdirs
#else
Source: "{#BundleDir}\whisper\*"; DestDir: "{app}\whisper"; Tasks: local; Flags: ignoreversion recursesubdirs createallsubdirs
#endif
Source: "{#ModelURL}"; DestName: "{#ModelFile}"; DestDir: "{app}\models"; Tasks: local; Hash: "{#ModelSHA256}"; ExternalSize: {#ModelSize}; Flags: external download ignoreversion

[Icons]
Name: "{group}\VoxSteer"; Filename: "{app}\voxsteer.exe"; WorkingDir: "{app}"; IconFilename: "{app}\voxsteer.ico"
Name: "{autostartup}\VoxSteer"; Filename: "{app}\voxsteer.exe"; WorkingDir: "{app}"; IconFilename: "{app}\voxsteer.ico"; Tasks: startup

[Run]
Filename: "{app}\voxsteer.exe"; Description: "Launch VoxSteer"; Flags: postinstall nowait skipifsilent runasoriginaluser

[Code]
var
  CommonPage: TInputQueryWizardPage;
  ActivationPage: TWizardPage;
  ActivationLabel: TNewStaticText;
  ActivationEdit: TNewEdit;
  ActivationCaptureButton: TNewButton;
  ActivationTrayButton: TNewButton;
  CloudPage: TInputQueryWizardPage;
  ExistingConfig: Boolean;
  CapturedBinding: String;

const
  VK_MBUTTON = $04;
  VK_XBUTTON1 = $05;
  VK_XBUTTON2 = $06;
  VK_SHIFT = $10;
  VK_CONTROL = $11;
  VK_MENU = $12;
  VK_SPACE = $20;
  VK_ESCAPE = $1B;
  VK_LWIN = $5B;
  VK_RWIN = $5C;
  VK_F1 = $70;
  VK_F12 = $7B;

function GetAsyncKeyState(VirtualKey: Integer): SmallInt;
  external 'GetAsyncKeyState@user32.dll stdcall';

function ConfigPath: String;
begin
  Result := ExpandConstant('{userappdata}\VoxSteer\config.json');
end;

function ExistingConfiguration: Boolean;
begin
  Result := ExistingConfig;
end;

function JsonEscape(Value: String): String;
begin
  Result := Value;
  StringChangeEx(Result, '\', '\\', True);
  StringChangeEx(Result, '"', '\"', True);
  StringChangeEx(Result, #13, '\r', True);
  StringChangeEx(Result, #10, '\n', True);
end;

function KeyIsDown(VirtualKey: Integer): Boolean;
begin
  Result := GetAsyncKeyState(VirtualKey) < 0;
end;

function ModifierCanonical: String;
begin
  Result := '';
  if KeyIsDown(VK_CONTROL) then
    Result := Result + 'ctrl+';
  if KeyIsDown(VK_MENU) then
    Result := Result + 'alt+';
  if KeyIsDown(VK_SHIFT) then
    Result := Result + 'shift+';
  if KeyIsDown(VK_LWIN) or KeyIsDown(VK_RWIN) then
    Result := Result + 'win+';
end;

function ModifierDisplay: String;
begin
  Result := '';
  if KeyIsDown(VK_CONTROL) then
    Result := Result + 'Ctrl+';
  if KeyIsDown(VK_MENU) then
    Result := Result + 'Alt+';
  if KeyIsDown(VK_SHIFT) then
    Result := Result + 'Shift+';
  if KeyIsDown(VK_LWIN) or KeyIsDown(VK_RWIN) then
    Result := Result + 'Win+';
end;

procedure SaveCapturedBinding(Canonical, Display: String);
begin
  CapturedBinding := Canonical;
  ActivationEdit.Text := Display;
  ActivationEdit.Update;
end;

procedure CaptureBinding(Sender: TObject);
var
  KeyCode: Integer;
  KeyName: String;
  Prefix: String;
begin
  ActivationEdit.Text := 'Press a shortcut or Mouse 3, 4, or 5...';
  ActivationEdit.Update;

  while True do
  begin
    if KeyIsDown(VK_ESCAPE) then
    begin
      ActivationEdit.Text := 'Ctrl+Alt+Space';
      CapturedBinding := 'ctrl+alt+space';
      Exit;
    end;
    if KeyIsDown(VK_MBUTTON) then
    begin
      SaveCapturedBinding('mouse3', 'Mouse 3 (middle button)');
      Exit;
    end;
    if KeyIsDown(VK_XBUTTON1) then
    begin
      SaveCapturedBinding('mouse4', 'Mouse 4');
      Exit;
    end;
    if KeyIsDown(VK_XBUTTON2) then
    begin
      SaveCapturedBinding('mouse5', 'Mouse 5');
      Exit;
    end;

    KeyCode := 0;
    KeyName := '';
    if KeyIsDown(VK_SPACE) then
    begin
      KeyCode := VK_SPACE;
      KeyName := 'Space';
    end
    else
    begin
      for KeyCode := Ord('A') to Ord('Z') do
        if KeyIsDown(KeyCode) then
        begin
          KeyName := Chr(KeyCode);
          Break;
        end;
      if KeyName = '' then
        for KeyCode := Ord('0') to Ord('9') do
          if KeyIsDown(KeyCode) then
          begin
            KeyName := Chr(KeyCode);
            Break;
          end;
      if KeyName = '' then
        for KeyCode := VK_F1 to VK_F12 do
          if KeyIsDown(KeyCode) then
          begin
            KeyName := 'F' + IntToStr(KeyCode - VK_F1 + 1);
            Break;
          end;
    end;

    if KeyName <> '' then
    begin
      Prefix := ModifierCanonical;
      if (Prefix <> '') or ((KeyCode >= VK_F1) and (KeyCode <= VK_F12)) then
      begin
        SaveCapturedBinding(
          Prefix + Lowercase(KeyName),
          ModifierDisplay + KeyName
        );
        Exit;
      end;
      ActivationEdit.Text := 'Add Ctrl, Alt, Shift, or Win...';
      ActivationEdit.Update;
    end;
    Sleep(15);
  end;
end;

procedure UseTrayOnly(Sender: TObject);
begin
  SaveCapturedBinding('disabled', 'Tray menu only');
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

  ActivationPage := CreateCustomPage(
    CommonPage.ID,
    'Activation',
    'Capture any supported keyboard shortcut or mouse button.'
  );
  ActivationLabel := TNewStaticText.Create(ActivationPage);
  ActivationLabel.Parent := ActivationPage.Surface;
  ActivationLabel.Caption :=
    'Click Capture, then hold your modifiers and press a letter, number, Space, or F1-F12.' + #13#10 +
    'You can also press the middle button, Mouse 4, or Mouse 5.';
  ActivationLabel.WordWrap := True;
  ActivationLabel.SetBounds(0, 8, ActivationPage.SurfaceWidth, 48);

  ActivationEdit := TNewEdit.Create(ActivationPage);
  ActivationEdit.Parent := ActivationPage.Surface;
  ActivationEdit.ReadOnly := True;
  ActivationEdit.Text := 'Ctrl+Alt+Space';
  ActivationEdit.SetBounds(0, 68, ScaleX(250), ScaleY(23));
  CapturedBinding := 'ctrl+alt+space';

  ActivationCaptureButton := TNewButton.Create(ActivationPage);
  ActivationCaptureButton.Parent := ActivationPage.Surface;
  ActivationCaptureButton.Caption := 'Capture...';
  ActivationCaptureButton.SetBounds(
    ActivationEdit.Left + ActivationEdit.Width + ScaleX(8),
    ActivationEdit.Top - ScaleY(1),
    ScaleX(90),
    ScaleY(25)
  );
  ActivationCaptureButton.OnClick := @CaptureBinding;

  ActivationTrayButton := TNewButton.Create(ActivationPage);
  ActivationTrayButton.Parent := ActivationPage.Surface;
  ActivationTrayButton.Caption := 'Tray menu only';
  ActivationTrayButton.SetBounds(0, ScaleY(108), ScaleX(110), ScaleY(25));
  ActivationTrayButton.OnClick := @UseTrayOnly;

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
  if ExistingConfig and not WizardIsTaskSelected('reconfigure') and
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
  ConfigJSON: String;
begin
  if ExistingConfig and not WizardIsTaskSelected('reconfigure') then
    Exit;

  if WizardIsTaskSelected('local') then
    Engine := 'local'
  else
    Engine := 'cloud';

  ConfigJSON :=
    '{' +
    '"engine":"' + Engine + '",' +
    '"language":"' + JsonEscape(CommonPage.Values[0]) + '",' +
    '"shortcut":"' + CapturedBinding + '",' +
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
