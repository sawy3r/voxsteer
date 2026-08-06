"use strict";

const SESSION_STORAGE_KEY = "voxsteer.browser.session";

let sessionToken = takeSessionToken();

function takeSessionToken() {
  let fragmentToken = null;
  const hadFragment = window.location.hash.length > 0;

  if (hadFragment) {
    try {
      const fragment = new URLSearchParams(window.location.hash.slice(1));
      if (fragment.has("session")) {
        fragmentToken = fragment.get("session") || "";
      }
    } finally {
      window.history.replaceState(
        window.history.state,
        document.title,
        `${window.location.pathname}${window.location.search}`,
      );
    }
  }

  if (fragmentToken !== null) {
    if (!isValidSessionToken(fragmentToken)) {
      removeStoredSessionToken();
      return "";
    }

    try {
      window.sessionStorage.setItem(SESSION_STORAGE_KEY, fragmentToken);
    } catch {
      // The in-memory token still supports this tab when storage is unavailable.
    }
    return fragmentToken;
  }

  try {
    const storedToken = window.sessionStorage.getItem(SESSION_STORAGE_KEY) || "";
    if (isValidSessionToken(storedToken)) {
      return storedToken;
    }
    removeStoredSessionToken();
  } catch {
    // Storage can be disabled without preventing a fragment-launched session.
  }

  return "";
}

function isValidSessionToken(value) {
  return value.length > 0
    && value.length <= 8192
    && /^[A-Za-z0-9_-]+={0,2}$/.test(value);
}

function removeStoredSessionToken() {
  try {
    window.sessionStorage.removeItem(SESSION_STORAGE_KEY);
  } catch {
    // Nothing else needs to be done when storage is unavailable.
  }
}

class ApiError extends Error {
  constructor(message, status) {
    super(message);
    this.name = "ApiError";
    this.status = status;
  }
}

const elements = {
  connectionBadge: document.querySelector("#connectionBadge"),
  connectionLabel: document.querySelector("#connectionLabel"),
  settingsButton: document.querySelector("#settingsButton"),
  sessionSummary: document.querySelector("#sessionSummary"),
  captureStep: document.querySelector("#captureStep"),
  encodeStep: document.querySelector("#encodeStep"),
  transcribeStep: document.querySelector("#transcribeStep"),
  recorderStage: document.querySelector("#recorderStage"),
  waveform: document.querySelector("#waveform"),
  startButton: document.querySelector("#startButton"),
  stopButton: document.querySelector("#stopButton"),
  discardButton: document.querySelector("#discardButton"),
  cancelButton: document.querySelector("#cancelButton"),
  elapsedTime: document.querySelector("#elapsedTime"),
  timerLimit: document.querySelector("#timerLimit"),
  inputLevel: document.querySelector("#inputLevel"),
  appStatus: document.querySelector("#appStatus"),
  statusDetail: document.querySelector("#statusDetail"),
  statusBlock: document.querySelector("#appStatus").closest(".status-block"),
  transcript: document.querySelector("#transcript"),
  characterCount: document.querySelector("#characterCount"),
  copyButton: document.querySelector("#copyButton"),
  clearButton: document.querySelector("#clearButton"),
  openLogButton: document.querySelector("#openLogButton"),
  quitButton: document.querySelector("#quitButton"),
  settingsDialog: document.querySelector("#settingsDialog"),
  settingsForm: document.querySelector("#settingsForm"),
  closeSettingsButton: document.querySelector("#closeSettingsButton"),
  cancelSettingsButton: document.querySelector("#cancelSettingsButton"),
  saveSettingsButton: document.querySelector("#saveSettingsButton"),
  settingsError: document.querySelector("#settingsError"),
  cloudEngine: document.querySelector("#cloudEngine"),
  localEngine: document.querySelector("#localEngine"),
  languageInput: document.querySelector("#languageInput"),
  cloudSettings: document.querySelector("#cloudSettings"),
  localSettings: document.querySelector("#localSettings"),
  cloudEndpointInput: document.querySelector("#cloudEndpointInput"),
  cloudModelInput: document.querySelector("#cloudModelInput"),
  cloudApiKeyInput: document.querySelector("#cloudApiKeyInput"),
  clearApiKeyInput: document.querySelector("#clearApiKeyInput"),
  apiKeyState: document.querySelector("#apiKeyState"),
  localBinaryInput: document.querySelector("#localBinaryInput"),
  localModelInput: document.querySelector("#localModelInput"),
  toast: document.querySelector("#toast"),
};

const app = {
  phase: "loading",
  sessionReady: false,
  engine: "",
  language: "",
  maxRecordingSeconds: 0,
  stream: null,
  audioContext: null,
  sourceNode: null,
  workletNode: null,
  pendingAudioContext: null,
  pendingStream: null,
  captureGeneration: 0,
  audioChunks: [],
  totalSamples: 0,
  sourceSampleRate: 0,
  acceptAudioChunks: false,
  cleanupInProgress: false,
  recordingStartedAt: 0,
  recordingTimer: 0,
  recordingLimitTimer: 0,
  workletStopResolver: null,
  processingController: null,
  activeCaptureTask: null,
  latestWaveform: new Float32Array(0),
  waveformLevel: 0,
  waveformFrame: 0,
  toastTimer: 0,
  heartbeatTimer: 0,
  pingInFlight: false,
  settingsLoadSequence: 0,
  setupPrompted: false,
  settingsSaving: false,
  shuttingDown: false,
};

bindEvents();
resizeWaveform();
updateTranscriptControls();
renderState();
void initialize();

async function initialize() {
  if (!sessionToken) {
    app.phase = "error";
    setConnection("error", "No session");
    setStatus(
      "This page does not have a valid VoxSteer session.",
      "Launch browser mode from VoxSteer again to create a new local session.",
      true,
    );
    elements.sessionSummary.textContent = "Session required";
    renderState();
    return;
  }

  await refreshSession();
  startHeartbeat();
}

function bindEvents() {
  elements.startButton.addEventListener("click", () => {
    void startRecording();
  });
  elements.stopButton.addEventListener("click", () => {
    void requestCaptureFinish({ discard: false, limitReached: false });
  });
  elements.discardButton.addEventListener("click", () => {
    void requestCaptureFinish({ discard: true, limitReached: false });
  });
  elements.cancelButton.addEventListener("click", cancelProcessing);

  elements.transcript.addEventListener("input", updateTranscriptControls);
  elements.copyButton.addEventListener("click", () => {
    void copyTranscript();
  });
  elements.clearButton.addEventListener("click", clearTranscript);

  elements.settingsButton.addEventListener("click", () => {
    void openSettings();
  });
  elements.closeSettingsButton.addEventListener("click", () => {
    if (!app.settingsSaving) {
      elements.settingsDialog.close("cancel");
    }
  });
  elements.cancelSettingsButton.addEventListener("click", () => {
    if (!app.settingsSaving) {
      elements.settingsDialog.close("cancel");
    }
  });
  elements.settingsDialog.addEventListener("click", (event) => {
    if (event.target === elements.settingsDialog && !app.settingsSaving) {
      elements.settingsDialog.close("cancel");
    }
  });
  elements.settingsDialog.addEventListener("close", () => {
    clearSettingsSecrets();
    elements.settingsButton.setAttribute("aria-expanded", "false");
  });
  elements.settingsDialog.addEventListener("cancel", (event) => {
    if (app.settingsSaving) {
      event.preventDefault();
      return;
    }
    clearSettingsSecrets();
  });
  elements.settingsForm.addEventListener("submit", (event) => {
    void saveSettings(event);
  });
  elements.cloudEngine.addEventListener("change", updateEngineFields);
  elements.localEngine.addEventListener("change", updateEngineFields);
  elements.cloudApiKeyInput.addEventListener("input", () => {
    if (elements.cloudApiKeyInput.value !== "") {
      elements.clearApiKeyInput.checked = false;
    }
  });
  elements.clearApiKeyInput.addEventListener("change", () => {
    if (elements.clearApiKeyInput.checked) {
      elements.cloudApiKeyInput.value = "";
    }
    elements.cloudApiKeyInput.disabled = elements.clearApiKeyInput.checked;
  });

  elements.openLogButton.addEventListener("click", () => {
    void openDiagnosticLog();
  });
  elements.quitButton.addEventListener("click", () => {
    void quitVoxSteer();
  });

  document.addEventListener("keydown", handleKeyboardShortcut);
  document.addEventListener("visibilitychange", () => {
    if (!document.hidden) {
      void pingServer();
    }
  });
  window.addEventListener("resize", resizeWaveform);
  window.addEventListener("pagehide", releaseAudioImmediately);

  if ("ResizeObserver" in window) {
    const observer = new ResizeObserver(resizeWaveform);
    observer.observe(elements.recorderStage);
  }
}

async function apiRequest(path, options = {}) {
  if (!sessionToken) {
    throw new ApiError("The VoxSteer session is missing. Launch browser mode again.", 0);
  }
  if (!path.startsWith("/api/")) {
    throw new TypeError("API requests must use a same-origin API path.");
  }

  const headers = new Headers({
    Accept: "application/json",
    "X-VoxSteer-Token": sessionToken,
  });
  let body = options.body;

  if (Object.prototype.hasOwnProperty.call(options, "json")) {
    headers.set("Content-Type", "application/json");
    body = JSON.stringify(options.json);
  } else if (options.contentType) {
    headers.set("Content-Type", options.contentType);
  }

  const response = await window.fetch(path, {
    method: options.method || "GET",
    headers,
    body,
    signal: options.signal,
    cache: "no-store",
    credentials: "same-origin",
    redirect: "error",
    referrerPolicy: "no-referrer",
  });

  if (response.status === 204) {
    return null;
  }

  const responseText = await response.text();
  let payload = null;
  if (responseText) {
    try {
      payload = JSON.parse(responseText);
    } catch {
      if (response.ok) {
        throw new ApiError("VoxSteer returned an invalid response.", response.status);
      }
    }
  }

  if (!response.ok) {
    const message = payload && typeof payload.error === "string"
      ? payload.error
      : `VoxSteer request failed (${response.status}).`;
    throw new ApiError(message, response.status);
  }

  return payload;
}

async function refreshSession() {
  app.sessionReady = false;
  app.phase = "loading";
  setConnection("connecting", "Connecting");
  setStatus(
    "Checking the VoxSteer session...",
    "Microphone access is requested only after you press Start.",
  );
  renderState();

  try {
    const session = await apiRequest("/api/session", { method: "POST" });
    if (!session || typeof session.ready !== "boolean") {
      throw new ApiError("VoxSteer returned an invalid session response.", 0);
    }

    app.engine = typeof session.engine === "string" ? session.engine : "";
    app.language = typeof session.language === "string" ? session.language : "";

    const reportedLimit = Number(session.max_recording_seconds);
    if (Number.isFinite(reportedLimit) && reportedLimit > 0) {
      app.maxRecordingSeconds = Math.max(1, Math.floor(reportedLimit));
    } else if (session.ready) {
      throw new ApiError("VoxSteer did not provide a valid recording limit.", 0);
    }

    updateSessionSummary();

    if (session.ready) {
      app.sessionReady = true;
      app.phase = "ready";
      setConnection("ready", "Ready");
      setStatus(
        "Ready to record.",
        `Audio will be sent only to the configured ${formatEngine(app.engine)} engine after you stop.`,
      );
    } else {
      app.phase = "error";
      setConnection("connecting", "Setup needed");
      setStatus(
        typeof session.error === "string" && session.error
          ? session.error
          : "Transcription setup is incomplete.",
        "Open Settings to configure a cloud endpoint or local whisper.cpp engine.",
        true,
      );
      if (!app.setupPrompted) {
        app.setupPrompted = true;
        window.setTimeout(() => {
          if (!app.sessionReady && !elements.settingsDialog.open) {
            void openSettings();
          }
        }, 0);
      }
    }
  } catch (error) {
    app.phase = "error";
    setConnection("offline", "Unavailable");
    setStatus(
      "VoxSteer could not start this browser session.",
      errorMessage(error),
      true,
    );
    elements.sessionSummary.textContent = "Session unavailable";
    if (error instanceof ApiError && (error.status === 401 || error.status === 403)) {
      removeStoredSessionToken();
      sessionToken = "";
    }
  }

  renderState();
}

function updateSessionSummary() {
  const engine = formatEngine(app.engine);
  const language = app.language ? app.language.toUpperCase() : "Auto language";
  const limit = app.maxRecordingSeconds > 0
    ? `${formatDuration(app.maxRecordingSeconds)} max`
    : "Limit unavailable";
  elements.sessionSummary.textContent = `${engine} | ${language} | ${limit}`;
  elements.timerLimit.textContent = app.maxRecordingSeconds > 0
    ? `/ ${formatDuration(app.maxRecordingSeconds)}`
    : "/ --:--";
}

function formatEngine(engine) {
  if (engine === "cloud") {
    return "cloud";
  }
  if (engine === "local") {
    return "local";
  }
  return engine || "configured";
}

function setConnection(state, label) {
  elements.connectionBadge.dataset.state = state;
  elements.connectionBadge.setAttribute("aria-label", `VoxSteer connection: ${label}`);
  elements.connectionLabel.textContent = label;
}

function setStatus(message, detail, isError = false) {
  elements.appStatus.textContent = message;
  elements.statusDetail.textContent = detail || "";
  elements.statusBlock.classList.toggle("is-error", isError);
}

function setPhase(phase) {
  app.phase = phase;
  renderState();
}

function renderState() {
  const phase = app.phase;
  const canOfferStart = phase === "ready" || phase === "result" || phase === "error";
  const canStart = canOfferStart && app.sessionReady && !app.shuttingDown;
  const isRecording = phase === "recording";
  const isProcessing = phase === "encoding" || phase === "transcribing" || phase === "cancelling";
  const canCancel = phase === "requesting" || isProcessing;

  elements.startButton.hidden = !canOfferStart || app.shuttingDown;
  elements.startButton.disabled = !canStart;
  elements.stopButton.hidden = !isRecording || app.shuttingDown;
  elements.discardButton.hidden = !isRecording || app.shuttingDown;
  elements.cancelButton.hidden = !canCancel || app.shuttingDown;
  elements.cancelButton.disabled = phase === "cancelling";

  const blocksSettings = isRecording
    || phase === "requesting"
    || phase === "discarding"
    || phase === "loading"
    || isProcessing;
  elements.settingsButton.disabled = !sessionToken || blocksSettings || app.shuttingDown;
  elements.openLogButton.disabled = !sessionToken || app.shuttingDown;
  elements.quitButton.disabled = !sessionToken || app.shuttingDown;
  const transcriptLocked = phase === "requesting"
    || isRecording
    || phase === "discarding"
    || isProcessing;
  elements.transcript.readOnly = transcriptLocked;
  elements.transcript.setAttribute("aria-readonly", String(transcriptLocked));
  updateTranscriptControls();

  let stageState = phase;
  if (canOfferStart || phase === "error" || phase === "result") {
    stageState = "ready";
  } else if (phase === "discarding") {
    stageState = "cancelling";
  }
  elements.recorderStage.dataset.state = stageState;

  updatePipeline(phase);
  scheduleWaveformDraw();
}

function updatePipeline(phase) {
  const steps = [elements.captureStep, elements.encodeStep, elements.transcribeStep];
  for (const step of steps) {
    step.classList.remove("is-active", "is-complete");
    step.removeAttribute("aria-current");
  }

  if (phase === "result") {
    for (const step of steps) {
      step.classList.add("is-complete");
    }
    return;
  }

  let activeIndex = -1;
  if (phase === "ready" || phase === "error" || phase === "requesting" || phase === "recording") {
    activeIndex = 0;
  } else if (phase === "encoding" || phase === "cancelling") {
    activeIndex = 1;
  } else if (phase === "transcribing") {
    activeIndex = 2;
  }

  for (let index = 0; index < activeIndex; index += 1) {
    steps[index].classList.add("is-complete");
  }
  if (activeIndex >= 0) {
    steps[activeIndex].classList.add("is-active");
    steps[activeIndex].setAttribute("aria-current", "step");
  }
}

async function startRecording() {
  if (!app.sessionReady || app.shuttingDown || !["ready", "result", "error"].includes(app.phase)) {
    return;
  }

  if (!navigator.mediaDevices || typeof navigator.mediaDevices.getUserMedia !== "function") {
    setPhase("error");
    setStatus(
      "This browser cannot capture microphone audio.",
      "Use a current browser and open this page from VoxSteer on 127.0.0.1.",
      true,
    );
    return;
  }

  const AudioContextClass = window.AudioContext || window.webkitAudioContext;
  if (!AudioContextClass || typeof window.AudioWorkletNode !== "function") {
    setPhase("error");
    setStatus(
      "This browser does not support the required audio worklet.",
      "Update your browser, then launch VoxSteer browser mode again.",
      true,
    );
    return;
  }

  const captureGeneration = ++app.captureGeneration;
  setPhase("requesting");
  setStatus(
    "Waiting for browser microphone permission...",
    "Use the site control beside the address bar to allow Microphone for 127.0.0.1.",
  );

  app.audioChunks = [];
  app.totalSamples = 0;
  app.latestWaveform = new Float32Array(0);
  app.waveformLevel = 0;
  updateInputLevel(0);

  let audioContext = null;
  let stream = null;
  let attached = false;
  try {
    try {
      audioContext = new AudioContextClass({ latencyHint: "interactive", sampleRate: 16000 });
    } catch {
      audioContext = new AudioContextClass({ latencyHint: "interactive" });
    }
    app.pendingAudioContext = audioContext;

    stream = await navigator.mediaDevices.getUserMedia({
      audio: {
        channelCount: { ideal: 1 },
        echoCancellation: true,
        noiseSuppression: true,
        autoGainControl: true,
      },
      video: false,
    });

    if (
      captureGeneration !== app.captureGeneration
      || app.shuttingDown
      || app.phase !== "requesting"
    ) {
      if (app.pendingAudioContext === audioContext) {
        app.pendingAudioContext = null;
      }
      await releaseCaptureResources(audioContext, stream);
      return;
    }
    app.pendingStream = stream;

    if (audioContext.state === "suspended") {
      await audioContext.resume();
    }
    if (!audioContext.audioWorklet) {
      throw new Error("AudioWorklet is unavailable in this browser.");
    }

    await audioContext.audioWorklet.addModule("/recorder-worklet.js");
    if (audioContext.state === "suspended") {
      await audioContext.resume();
    }

    const sourceNode = audioContext.createMediaStreamSource(stream);
    const workletNode = new AudioWorkletNode(audioContext, "voxsteer-recorder", {
      numberOfInputs: 1,
      numberOfOutputs: 0,
      channelCountMode: "max",
      channelInterpretation: "speakers",
      processorOptions: {
        chunkSize: 2048,
        targetSampleRate: 16000,
        maxInputSamples: Math.floor(audioContext.sampleRate * app.maxRecordingSeconds),
        maxOutputSamples: Math.floor(16000 * app.maxRecordingSeconds),
      },
    });
    workletNode.port.onmessage = handleWorkletMessage;
    workletNode.addEventListener("processorerror", handleProcessorError);

    if (
      captureGeneration !== app.captureGeneration
      || app.shuttingDown
      || app.phase !== "requesting"
    ) {
      try {
        sourceNode.disconnect();
      } catch {
        // The source has not necessarily joined the active graph yet.
      }
      workletNode.port.close();
      if (app.pendingAudioContext === audioContext) {
        app.pendingAudioContext = null;
      }
      if (app.pendingStream === stream) {
        app.pendingStream = null;
      }
      await releaseCaptureResources(audioContext, stream);
      return;
    }

    app.pendingAudioContext = null;
    app.pendingStream = null;
    app.audioContext = audioContext;
    app.stream = stream;
    app.sourceNode = sourceNode;
    app.workletNode = workletNode;
    app.sourceSampleRate = 16000;
    app.acceptAudioChunks = true;
    app.cleanupInProgress = false;
    attached = true;

    for (const track of stream.getAudioTracks()) {
      track.addEventListener("ended", handleMicrophoneEnded, { once: true });
    }

    sourceNode.connect(workletNode);
    app.recordingStartedAt = performance.now();
    setPhase("recording");
    elements.stopButton.focus();
    setStatus(
      "Listening in this browser...",
      "Press Stop to transcribe, or Discard to delete this capture without sending it.",
    );
    elements.elapsedTime.textContent = "00:00";
    startCaptureTimers();
  } catch (error) {
    const ownsCapture = captureGeneration === app.captureGeneration;
    if (app.pendingAudioContext === audioContext) {
      app.pendingAudioContext = null;
    }
    if (app.pendingStream === stream) {
      app.pendingStream = null;
    }
    if (attached && app.audioContext === audioContext) {
      await teardownAudio();
    } else {
      await releaseCaptureResources(audioContext, stream);
    }
    if (!ownsCapture || app.shuttingDown) {
      return;
    }
    app.acceptAudioChunks = false;
    app.audioChunks = [];
    app.totalSamples = 0;
    setPhase("error");
    const permissionMessage = describeMicrophoneError(error);
    setStatus(permissionMessage.title, permissionMessage.detail, true);
  }
}

function handleWorkletMessage(event) {
  const message = event.data;
  if (!message || typeof message.type !== "string") {
    return;
  }

  if (message.type === "stopped") {
    if (app.workletStopResolver) {
      const resolve = app.workletStopResolver;
      app.workletStopResolver = null;
      resolve();
    }
    return;
  }

  if (message.type === "limit") {
    if (app.phase === "recording") {
      void requestCaptureFinish({ discard: false, limitReached: true });
    }
    return;
  }

  if (message.type !== "chunk" || !app.acceptAudioChunks || !(message.samples instanceof ArrayBuffer)) {
    return;
  }

  let samples = new Float32Array(message.samples);
  const maximumSamples = Math.max(
    1,
    Math.floor(16000 * app.maxRecordingSeconds),
  );
  const remainingSamples = maximumSamples - app.totalSamples;

  if (remainingSamples <= 0) {
    if (app.phase === "recording") {
      void requestCaptureFinish({ discard: false, limitReached: true });
    }
    return;
  }
  if (samples.length > remainingSamples) {
    samples = samples.slice(0, remainingSamples);
  }

  app.audioChunks.push(samples);
  app.totalSamples += samples.length;
  app.latestWaveform = samples;

  const peak = clampNumber(Number(message.peak), 0, 1);
  const rms = clampNumber(Number(message.rms), 0, 1);
  app.waveformLevel = Math.min(1, Math.max(peak * 0.7, rms * 3.2));
  updateInputLevel(app.waveformLevel);
  scheduleWaveformDraw();

  if (app.totalSamples >= maximumSamples && app.phase === "recording") {
    void requestCaptureFinish({ discard: false, limitReached: true });
  }
}

function handleProcessorError() {
  if (app.phase === "recording" || app.phase === "requesting") {
    void failCapture(
      "Browser audio processing stopped.",
      "No audio was sent. Check the microphone in this browser's site settings, then try again.",
    );
  }
}

function handleMicrophoneEnded() {
  if (!app.cleanupInProgress && (app.phase === "recording" || app.phase === "requesting")) {
    void failCapture(
      "The microphone input ended unexpectedly.",
      "No audio was sent. Reconnect or reselect the microphone in this browser's site settings.",
    );
  }
}

async function failCapture(title, detail) {
  if (!["recording", "requesting"].includes(app.phase)) {
    return;
  }
  clearCaptureTimers();
  app.acceptAudioChunks = false;
  app.audioChunks = [];
  app.totalSamples = 0;
  setPhase("error");
  await teardownAudio();
  resetCaptureReadout();
  setStatus(title, detail, true);
}

function requestCaptureFinish(options) {
  if (app.activeCaptureTask) {
    return app.activeCaptureTask;
  }
  if (app.phase !== "recording") {
    return Promise.resolve();
  }

  const task = finishCapture(options).catch(async (error) => {
    app.processingController?.abort();
    app.processingController = null;
    app.acceptAudioChunks = false;
    app.audioChunks = [];
    app.totalSamples = 0;
    await teardownAudio();
    if (!app.shuttingDown) {
      setPhase("error");
      setStatus(
        "Recording could not be completed.",
        `${errorMessage(error)} Captured audio was discarded.`,
        true,
      );
    }
  });
  app.activeCaptureTask = task;
  void task.then(() => {
    if (app.activeCaptureTask === task) {
      app.activeCaptureTask = null;
    }
  }, () => {
    if (app.activeCaptureTask === task) {
      app.activeCaptureTask = null;
    }
  });
  return task;
}

async function finishCapture({ discard, limitReached, quiet = false }) {
  clearCaptureTimers();

  if (discard) {
    setPhase("discarding");
    if (!quiet) {
      setStatus("Discarding this recording...", "Captured audio will not be sent.");
    }
  } else {
    app.processingController = new AbortController();
    setPhase("encoding");
    setStatus(
      limitReached ? "Recording limit reached. Shaping audio..." : "Shaping audio for transcription...",
      "Converting the browser capture to 16 kHz mono PCM audio.",
    );
  }

  try {
    await stopWorklet();
  } finally {
    app.acceptAudioChunks = false;
    await teardownAudio();
  }

  updateInputLevel(0);
  const capturedChunks = app.audioChunks;
  const capturedSamples = app.totalSamples;
  const sourceRate = app.sourceSampleRate;
  app.audioChunks = [];
  app.totalSamples = 0;

  if (sourceRate > 0 && capturedSamples > 0) {
    elements.elapsedTime.textContent = formatDuration(capturedSamples / sourceRate);
  }

  if (discard) {
    app.latestWaveform = new Float32Array(0);
    app.waveformLevel = 0;
    if (!app.shuttingDown) {
      setPhase("ready");
      if (!quiet) {
        setStatus(
          "Recording discarded.",
          "Nothing was sent. Press Start whenever you are ready to record again.",
        );
      }
      resetCaptureReadout();
    }
    return;
  }

  const controller = app.processingController;
  try {
    await nextPaint();
    throwIfAborted(controller.signal);

    if (capturedSamples === 0 || sourceRate <= 0) {
      throw new Error("No microphone audio was captured.");
    }
    if (sourceRate !== 16000) {
      throw new Error("Browser audio was not converted to the required sample rate.");
    }

    const mergedAudio = mergeAudioChunks(capturedChunks, capturedSamples);
    throwIfAborted(controller.signal);
    const waveFile = encodePcm16Wave(mergedAudio, 16000);
    throwIfAborted(controller.signal);

    setPhase("transcribing");
    setStatus(
      `Transcribing with the ${formatEngine(app.engine)} engine...`,
      "You can cancel while the request is in progress.",
    );

    const response = await apiRequest("/api/transcribe", {
      method: "POST",
      body: waveFile,
      contentType: "audio/wav",
      signal: controller.signal,
    });
    if (!response || typeof response.text !== "string") {
      throw new ApiError("VoxSteer returned an invalid transcript response.", 0);
    }

    app.latestWaveform = new Float32Array(0);
    app.waveformLevel = 0;
    setPhase("result");

    if (response.text) {
      const existing = elements.transcript.value;
      const needsSeparator = existing.length > 0 && !/\s$/.test(existing);
      elements.transcript.value = existing
        ? `${existing}${needsSeparator ? " " : ""}${response.text}`
        : response.text;
      updateTranscriptControls();
      setStatus(
        "Transcript ready for review.",
        "Edit the text if needed, then use Copy transcript when you choose.",
      );
      if (document.activeElement === document.body) {
        elements.transcript.focus();
        elements.transcript.setSelectionRange(
          elements.transcript.value.length,
          elements.transcript.value.length,
        );
      }
    } else {
      setStatus(
        "No speech was returned.",
        "Nothing was added. Your existing transcript, if any, is unchanged. Try again closer to the microphone or check the selected input in browser site settings.",
      );
    }
  } catch (error) {
    app.latestWaveform = new Float32Array(0);
    app.waveformLevel = 0;
    if (isAbortError(error) || controller.signal.aborted) {
      if (!app.shuttingDown) {
        setPhase("ready");
        setStatus(
          "Transcription cancelled.",
          "Captured audio was discarded and no transcript was added.",
        );
        resetCaptureReadout();
      }
    } else if (!app.shuttingDown) {
      setPhase("error");
      setStatus(
        "Transcription failed.",
        `${errorMessage(error)} The captured audio was discarded.`,
        true,
      );
    }
  } finally {
    if (app.processingController === controller) {
      app.processingController = null;
    }
    updateInputLevel(0);
    scheduleWaveformDraw();
  }
}

function stopWorklet() {
  if (!app.workletNode) {
    return Promise.resolve();
  }

  return new Promise((resolve, reject) => {
    let settled = false;
    const finish = (error) => {
      if (settled) {
        return;
      }
      settled = true;
      window.clearTimeout(timeout);
      app.workletStopResolver = null;
      if (error) {
        reject(error);
      } else {
        resolve();
      }
    };
    const timeout = window.setTimeout(() => {
      finish(new Error("Browser audio did not finish cleanly; the recording was discarded."));
    }, 1000);
    app.workletStopResolver = () => finish(null);

    try {
      app.workletNode.port.postMessage({ type: "stop" });
    } catch (error) {
      finish(error);
    }
  });
}

async function releaseCaptureResources(audioContext, stream) {
  if (stream) {
    for (const track of stream.getTracks()) {
      track.stop();
    }
  }
  if (audioContext && audioContext.state !== "closed") {
    try {
      await audioContext.close();
    } catch {
      // The browser can close a context while a permission request is pending.
    }
  }
}

async function teardownAudio() {
  app.cleanupInProgress = true;
  const pendingAudioContext = app.pendingAudioContext;
  const pendingStream = app.pendingStream;
  const sourceNode = app.sourceNode;
  const workletNode = app.workletNode;
  const stream = app.stream;
  const audioContext = app.audioContext;

  app.sourceNode = null;
  app.workletNode = null;
  app.stream = null;
  app.audioContext = null;
  app.pendingAudioContext = null;
  app.pendingStream = null;

  if (app.workletStopResolver) {
    const resolve = app.workletStopResolver;
    app.workletStopResolver = null;
    resolve();
  }

  try {
    sourceNode?.disconnect();
  } catch {
    // The node may already have been disconnected by the browser.
  }
  try {
    workletNode?.disconnect();
  } catch {
    // A zero-output worklet can reject disconnect in some browsers.
  }
  if (workletNode) {
    workletNode.port.onmessage = null;
    try {
      workletNode.port.close();
    } catch {
      // Closing an already stopped port is harmless.
    }
  }

  if (stream) {
    for (const track of stream.getTracks()) {
      track.stop();
    }
  }
  if (pendingStream && pendingStream !== stream) {
    for (const track of pendingStream.getTracks()) {
      track.stop();
    }
  }

  if (audioContext && audioContext.state !== "closed") {
    try {
      await audioContext.close();
    } catch {
      // Tracks and nodes are already released even if close is rejected.
    }
  }
  if (
    pendingAudioContext
    && pendingAudioContext !== audioContext
    && pendingAudioContext.state !== "closed"
  ) {
    try {
      await pendingAudioContext.close();
    } catch {
      // Closing a context with a pending permission request is best effort.
    }
  }
  app.cleanupInProgress = false;
}

function releaseAudioImmediately() {
  app.captureGeneration += 1;
  clearCaptureTimers();
  app.processingController?.abort();
  app.acceptAudioChunks = false;
  app.cleanupInProgress = true;

  try {
    app.sourceNode?.disconnect();
  } catch {
    // The page is leaving; best-effort cleanup is sufficient here.
  }
  try {
    app.workletNode?.disconnect();
    app.workletNode?.port.close();
  } catch {
    // The worklet may already be stopped.
  }
  if (app.stream) {
    for (const track of app.stream.getTracks()) {
      track.stop();
    }
  }
  if (app.pendingStream) {
    for (const track of app.pendingStream.getTracks()) {
      track.stop();
    }
  }
  if (app.audioContext && app.audioContext.state !== "closed") {
    void app.audioContext.close();
  }
  if (app.pendingAudioContext && app.pendingAudioContext.state !== "closed") {
    void app.pendingAudioContext.close();
  }
  app.pendingAudioContext = null;
  app.pendingStream = null;
}

function startCaptureTimers() {
  clearCaptureTimers();
  app.recordingTimer = window.setInterval(updateElapsedTime, 200);
  const limitMilliseconds = Math.min(app.maxRecordingSeconds * 1000, 2147483647);
  app.recordingLimitTimer = window.setTimeout(() => {
    if (app.phase === "recording") {
      void requestCaptureFinish({ discard: false, limitReached: true });
    }
  }, limitMilliseconds);
}

function clearCaptureTimers() {
  window.clearInterval(app.recordingTimer);
  window.clearTimeout(app.recordingLimitTimer);
  app.recordingTimer = 0;
  app.recordingLimitTimer = 0;
}

function updateElapsedTime() {
  if (app.phase !== "recording") {
    return;
  }
  const elapsed = Math.min(
    app.maxRecordingSeconds,
    (performance.now() - app.recordingStartedAt) / 1000,
  );
  elements.elapsedTime.textContent = formatDuration(elapsed);
  if (elapsed >= app.maxRecordingSeconds) {
    void requestCaptureFinish({ discard: false, limitReached: true });
  }
}

function resetCaptureReadout() {
  elements.elapsedTime.textContent = "00:00";
  updateInputLevel(0);
  app.latestWaveform = new Float32Array(0);
  app.waveformLevel = 0;
  scheduleWaveformDraw();
}

function cancelProcessing() {
  if (app.phase === "requesting") {
    app.captureGeneration += 1;
    const pendingContext = app.pendingAudioContext;
    const pendingStream = app.pendingStream;
    app.pendingAudioContext = null;
    app.pendingStream = null;
    if (pendingStream) {
      for (const track of pendingStream.getTracks()) {
        track.stop();
      }
    }
    if (pendingContext && pendingContext.state !== "closed") {
      void pendingContext.close().catch(() => {});
    }
    setPhase("ready");
    setStatus(
      "Microphone request cancelled.",
      "No audio was captured or sent. Press Start whenever you are ready to try again.",
    );
    return;
  }
  if (!app.processingController || app.processingController.signal.aborted) {
    return;
  }
  app.processingController.abort();
  setPhase("cancelling");
  setStatus(
    "Cancelling transcription...",
    "Captured audio will be discarded when cancellation completes.",
  );
}

function describeMicrophoneError(error) {
  const browserPermissionPath = "Use the site control beside the address bar, open Site settings, and set Microphone to Allow for 127.0.0.1.";
  const nativePermissionNote = "VoxSteer itself does not request native macOS permissions, but the browser may require macOS microphone access as well as site permission.";

  if (error && error.name === "NotAllowedError") {
    return {
      title: "Microphone access was blocked in this browser.",
      detail: `${browserPermissionPath} ${nativePermissionNote}`,
    };
  }
  if (error && error.name === "NotFoundError") {
    return {
      title: "No microphone was found.",
      detail: `Connect or enable an input, then check the browser's Microphone site setting for 127.0.0.1. ${nativePermissionNote}`,
    };
  }
  if (error && error.name === "NotReadableError") {
    return {
      title: "The browser found a microphone but could not read it.",
      detail: `Close other apps or tabs using the input, then reselect it in this browser's site settings. ${nativePermissionNote}`,
    };
  }
  if (error && error.name === "SecurityError") {
    return {
      title: "The browser blocked microphone capture for this site.",
      detail: `${browserPermissionPath} ${nativePermissionNote}`,
    };
  }
  return {
    title: "Microphone capture could not start.",
    detail: `${errorMessage(error)} Check the browser's Microphone site setting for 127.0.0.1.`,
  };
}

function mergeAudioChunks(chunks, totalSamples) {
  const merged = new Float32Array(totalSamples);
  let offset = 0;
  for (const chunk of chunks) {
    const remaining = totalSamples - offset;
    if (remaining <= 0) {
      break;
    }
    const portion = chunk.length > remaining ? chunk.subarray(0, remaining) : chunk;
    merged.set(portion, offset);
    offset += portion.length;
  }
  return offset === totalSamples ? merged : merged.slice(0, offset);
}

function encodePcm16Wave(samples, sampleRate) {
  const bytesPerSample = 2;
  const dataLength = samples.length * bytesPerSample;
  const buffer = new ArrayBuffer(44 + dataLength);
  const view = new DataView(buffer);

  writeAscii(view, 0, "RIFF");
  view.setUint32(4, 36 + dataLength, true);
  writeAscii(view, 8, "WAVE");
  writeAscii(view, 12, "fmt ");
  view.setUint32(16, 16, true);
  view.setUint16(20, 1, true);
  view.setUint16(22, 1, true);
  view.setUint32(24, sampleRate, true);
  view.setUint32(28, sampleRate * bytesPerSample, true);
  view.setUint16(32, bytesPerSample, true);
  view.setUint16(34, 16, true);
  writeAscii(view, 36, "data");
  view.setUint32(40, dataLength, true);

  let byteOffset = 44;
  for (let index = 0; index < samples.length; index += 1) {
    const sample = Math.max(-1, Math.min(1, samples[index]));
    const pcm = sample < 0 ? sample * 0x8000 : sample * 0x7fff;
    view.setInt16(byteOffset, Math.round(pcm), true);
    byteOffset += bytesPerSample;
  }
  return buffer;
}

function writeAscii(view, offset, value) {
  for (let index = 0; index < value.length; index += 1) {
    view.setUint8(offset + index, value.charCodeAt(index));
  }
}

function throwIfAborted(signal) {
  if (signal.aborted) {
    throw new DOMException("The operation was aborted.", "AbortError");
  }
}

function nextPaint() {
  return new Promise((resolve) => {
    window.requestAnimationFrame(() => resolve());
  });
}

function updateInputLevel(level) {
  const normalized = clampNumber(level, 0, 1);
  elements.inputLevel.value = normalized;
  elements.inputLevel.textContent = `${Math.round(normalized * 100)}%`;
}

function resizeWaveform() {
  const rectangle = elements.waveform.getBoundingClientRect();
  if (rectangle.width <= 0 || rectangle.height <= 0) {
    return;
  }
  const pixelRatio = Math.min(window.devicePixelRatio || 1, 2);
  const width = Math.max(1, Math.round(rectangle.width * pixelRatio));
  const height = Math.max(1, Math.round(rectangle.height * pixelRatio));
  if (elements.waveform.width !== width || elements.waveform.height !== height) {
    elements.waveform.width = width;
    elements.waveform.height = height;
  }
  drawWaveform();
}

function scheduleWaveformDraw() {
  if (app.waveformFrame) {
    return;
  }
  app.waveformFrame = window.requestAnimationFrame(() => {
    app.waveformFrame = 0;
    drawWaveform();
  });
}

function drawWaveform() {
  const canvas = elements.waveform;
  const context = canvas.getContext("2d");
  if (!context) {
    return;
  }

  const pixelRatio = Math.min(window.devicePixelRatio || 1, 2);
  const width = canvas.width / pixelRatio;
  const height = canvas.height / pixelRatio;
  const styles = window.getComputedStyle(document.documentElement);
  const cyan = styles.getPropertyValue("--cyan-400").trim() || "#35e8f4";
  const isActive = app.phase === "recording";
  const samples = app.latestWaveform;

  context.setTransform(pixelRatio, 0, 0, pixelRatio, 0, 0);
  context.clearRect(0, 0, width, height);
  context.lineCap = "round";

  context.beginPath();
  context.moveTo(0, height / 2);
  context.lineTo(width, height / 2);
  context.strokeStyle = "rgba(143, 165, 195, 0.14)";
  context.lineWidth = 1;
  context.stroke();

  const barCount = Math.max(42, Math.floor(width / 8));
  const gap = width / barCount;
  const maximumBarHeight = height * 0.36;
  context.strokeStyle = isActive ? cyan : "rgba(53, 232, 244, 0.2)";
  context.lineWidth = Math.max(1.5, Math.min(3, gap * 0.34));
  context.beginPath();

  for (let bar = 0; bar < barCount; bar += 1) {
    let amplitude = 0;
    if (samples.length > 0) {
      const start = Math.floor(bar * samples.length / barCount);
      const end = Math.max(start + 1, Math.floor((bar + 1) * samples.length / barCount));
      for (let index = start; index < end && index < samples.length; index += 1) {
        amplitude = Math.max(amplitude, Math.abs(samples[index]));
      }
    } else {
      const distanceFromCenter = Math.abs((bar / Math.max(1, barCount - 1)) * 2 - 1);
      amplitude = Math.max(0, 0.035 - distanceFromCenter * 0.03);
    }

    const visibleAmplitude = Math.min(1, amplitude * 2.4);
    const barHeight = Math.max(2, visibleAmplitude * maximumBarHeight);
    const x = (bar + 0.5) * gap;
    context.moveTo(x, height / 2 - barHeight);
    context.lineTo(x, height / 2 + barHeight);
  }
  context.stroke();
}

function updateTranscriptControls() {
  const count = Array.from(elements.transcript.value).length;
  elements.characterCount.textContent = `${count} ${count === 1 ? "character" : "characters"}`;
  const hasText = elements.transcript.value.length > 0;
  elements.copyButton.disabled = !hasText;
  elements.clearButton.disabled = !hasText || elements.transcript.readOnly;
}

async function copyTranscript() {
  const text = elements.transcript.value;
  if (!text) {
    return;
  }

  if (navigator.clipboard && typeof navigator.clipboard.writeText === "function") {
    try {
      await navigator.clipboard.writeText(text);
      showToast("Transcript copied to the clipboard.");
      return;
    } catch {
      // Selection below gives the user a manual path when clipboard access fails.
    }
  }

  elements.transcript.focus();
  elements.transcript.select();
  elements.transcript.setSelectionRange(0, text.length);
  showToast("Transcript selected. Press Command+C or Ctrl+C to copy it.");
}

function clearTranscript() {
  elements.transcript.value = "";
  updateTranscriptControls();
  elements.transcript.focus();
  showToast("Transcript cleared from this page.");
}

function showToast(message) {
  window.clearTimeout(app.toastTimer);
  elements.toast.textContent = message;
  elements.toast.hidden = false;
  app.toastTimer = window.setTimeout(() => {
    elements.toast.hidden = true;
  }, 4200);
}

async function openSettings() {
  if (!sessionToken || elements.settingsDialog.open) {
    return;
  }

  elements.settingsError.hidden = true;
  elements.settingsError.textContent = "";
  elements.saveSettingsButton.disabled = true;
  elements.saveSettingsButton.textContent = "Loading...";
  elements.settingsForm.setAttribute("aria-busy", "true");
  clearSettingsSecrets();
  elements.settingsDialog.showModal();
  elements.settingsButton.setAttribute("aria-expanded", "true");

  const loadSequence = ++app.settingsLoadSequence;
  try {
    const settings = await apiRequest("/api/settings");
    if (loadSequence !== app.settingsLoadSequence || !elements.settingsDialog.open) {
      return;
    }
    populateSettings(settings);
    elements.saveSettingsButton.disabled = false;
  } catch (error) {
    if (loadSequence !== app.settingsLoadSequence || !elements.settingsDialog.open) {
      return;
    }
    showSettingsError(`Settings could not be loaded. ${errorMessage(error)}`);
  } finally {
    if (loadSequence === app.settingsLoadSequence) {
      elements.settingsForm.removeAttribute("aria-busy");
      elements.saveSettingsButton.textContent = "Save settings";
    }
  }
}

function populateSettings(settings) {
  if (!settings || typeof settings !== "object") {
    throw new ApiError("VoxSteer returned invalid settings.", 0);
  }

  const engine = settings.engine === "cloud" || settings.engine === "local"
    ? settings.engine
    : "cloud";
  elements.cloudEngine.checked = engine === "cloud";
  elements.localEngine.checked = engine === "local";
  elements.languageInput.value = stringValue(settings.language);
  elements.cloudEndpointInput.value = stringValue(settings.cloud_endpoint);
  elements.cloudModelInput.value = stringValue(settings.cloud_model);
  elements.localBinaryInput.value = stringValue(settings.local_binary);
  elements.localModelInput.value = stringValue(settings.local_model);
  elements.cloudApiKeyInput.value = "";
  elements.cloudApiKeyInput.disabled = false;
  elements.clearApiKeyInput.checked = false;
  elements.apiKeyState.textContent = settings.has_cloud_api_key
    ? "A saved key is configured"
    : "No saved key is configured";
  updateEngineFields();
  if (settings.engine !== "cloud" && settings.engine !== "local") {
    showSettingsError("The saved transcription engine is unsupported. Choose Cloud or Local, then save.");
  }
}

function updateEngineFields() {
  const cloudSelected = elements.cloudEngine.checked;
  elements.cloudSettings.hidden = !cloudSelected;
  elements.cloudSettings.disabled = !cloudSelected;
  elements.localSettings.hidden = cloudSelected;
  elements.localSettings.disabled = cloudSelected;

  elements.cloudEndpointInput.required = cloudSelected;
  elements.cloudModelInput.required = cloudSelected;
  elements.localBinaryInput.required = false;
  elements.localModelInput.required = !cloudSelected;
}

async function saveSettings(event) {
  event.preventDefault();
  if (app.settingsSaving || !elements.settingsForm.reportValidity()) {
    return;
  }

  const engine = elements.cloudEngine.checked ? "cloud" : "local";
  const cloudApiKey = elements.cloudApiKeyInput.value;
  elements.cloudApiKeyInput.value = "";

  const payload = {
    engine,
    language: elements.languageInput.value.trim(),
    cloud_endpoint: elements.cloudEndpointInput.value.trim(),
    cloud_model: elements.cloudModelInput.value.trim(),
    local_binary: elements.localBinaryInput.value.trim(),
    local_model: elements.localModelInput.value.trim(),
  };
  if (engine === "cloud" && elements.clearApiKeyInput.checked) {
    payload.cloud_api_key = "";
  } else if (engine === "cloud" && cloudApiKey !== "") {
    payload.cloud_api_key = cloudApiKey;
  }

  app.settingsSaving = true;
  setSettingsDismissEnabled(false);
  elements.settingsError.hidden = true;
  elements.saveSettingsButton.disabled = true;
  elements.saveSettingsButton.textContent = "Saving...";
  elements.settingsForm.setAttribute("aria-busy", "true");

  try {
    await apiRequest("/api/settings", { method: "PUT", json: payload });
    delete payload.cloud_api_key;
    app.settingsSaving = false;
    setSettingsDismissEnabled(true);
    elements.settingsDialog.close("saved");
    showToast("Settings saved.");
    await refreshSession();
  } catch (error) {
    delete payload.cloud_api_key;
    showSettingsError(`Settings were not saved. ${errorMessage(error)}`);
  } finally {
    delete payload.cloud_api_key;
    app.settingsSaving = false;
    setSettingsDismissEnabled(true);
    elements.cloudApiKeyInput.value = "";
    elements.cloudApiKeyInput.disabled = false;
    elements.clearApiKeyInput.checked = false;
    elements.settingsForm.removeAttribute("aria-busy");
    elements.saveSettingsButton.disabled = false;
    elements.saveSettingsButton.textContent = "Save settings";
  }
}

function showSettingsError(message) {
  elements.settingsError.textContent = message;
  elements.settingsError.hidden = false;
}

function clearSettingsSecrets() {
  elements.cloudApiKeyInput.value = "";
  elements.cloudApiKeyInput.disabled = false;
  elements.clearApiKeyInput.checked = false;
}

function setSettingsDismissEnabled(enabled) {
  elements.closeSettingsButton.disabled = !enabled;
  elements.cancelSettingsButton.disabled = !enabled;
}

async function openDiagnosticLog() {
  elements.openLogButton.disabled = true;
  try {
    await apiRequest("/api/open-log", { method: "POST" });
    showToast("Diagnostic log opened by VoxSteer.");
  } catch (error) {
    showToast(`Could not open the diagnostic log. ${errorMessage(error)}`);
  } finally {
    elements.openLogButton.disabled = !sessionToken || app.shuttingDown;
  }
}

async function quitVoxSteer() {
  if (!sessionToken || app.shuttingDown) {
    return;
  }
  if (!window.confirm("Quit VoxSteer and end this local browser session?")) {
    return;
  }

  app.shuttingDown = true;
  renderState();
  setStatus("Quitting VoxSteer...", "Ending the local browser session.");

  try {
    if (app.phase === "recording") {
      await requestCaptureFinish({ discard: true, limitReached: false, quiet: true });
    } else if (app.activeCaptureTask) {
      app.processingController?.abort();
      await app.activeCaptureTask;
    } else {
      await teardownAudio();
    }

    await apiRequest("/api/quit", { method: "POST" });
    window.clearInterval(app.heartbeatTimer);
    removeStoredSessionToken();
    sessionToken = "";
    app.sessionReady = false;
    app.phase = "quit";
    setConnection("offline", "Stopped");
    setStatus(
      "VoxSteer has quit.",
      "You can close this tab when you are finished with any transcript text still on the page.",
    );
  } catch (error) {
    app.shuttingDown = false;
    app.phase = "error";
    setConnection("offline", "Unavailable");
    setStatus("VoxSteer could not confirm quit.", errorMessage(error), true);
  }
  renderState();
}

function startHeartbeat() {
  if (app.heartbeatTimer || !sessionToken) {
    return;
  }
  app.heartbeatTimer = window.setInterval(() => {
    void pingServer();
  }, 30000);
}

async function pingServer() {
  if (!sessionToken || document.hidden || app.pingInFlight || app.shuttingDown) {
    return;
  }
  app.pingInFlight = true;
  try {
    await apiRequest("/api/ping", { method: "POST" });
    setConnection(app.sessionReady ? "ready" : "connecting", app.sessionReady ? "Ready" : "Setup needed");
  } catch {
    setConnection("offline", "Connection lost");
  } finally {
    app.pingInFlight = false;
  }
}

function handleKeyboardShortcut(event) {
  if (
    event.defaultPrevented
    || event.repeat
    || event.code !== "Space"
    || event.altKey
    || event.ctrlKey
    || event.metaKey
    || event.shiftKey
    || elements.settingsDialog.open
    || isInteractiveOrEditable(event.target)
  ) {
    return;
  }

  if (app.phase === "recording") {
    event.preventDefault();
    void requestCaptureFinish({ discard: false, limitReached: false });
  } else if (
    app.sessionReady
    && !app.shuttingDown
    && ["ready", "result", "error"].includes(app.phase)
  ) {
    event.preventDefault();
    void startRecording();
  }
}

function isInteractiveOrEditable(target) {
  if (!(target instanceof Element)) {
    return false;
  }
  return Boolean(target.closest(
    "button, a, input, textarea, select, summary, [contenteditable]:not([contenteditable='false']), [role='button'], [role='textbox']",
  ));
}

function formatDuration(seconds) {
  const wholeSeconds = Math.max(0, Math.floor(Number(seconds) || 0));
  const hours = Math.floor(wholeSeconds / 3600);
  const minutes = Math.floor((wholeSeconds % 3600) / 60);
  const remainder = wholeSeconds % 60;

  if (hours > 0) {
    return `${hours}:${String(minutes).padStart(2, "0")}:${String(remainder).padStart(2, "0")}`;
  }
  return `${String(minutes).padStart(2, "0")}:${String(remainder).padStart(2, "0")}`;
}

function stringValue(value) {
  return typeof value === "string" ? value : "";
}

function clampNumber(value, minimum, maximum) {
  if (!Number.isFinite(value)) {
    return minimum;
  }
  return Math.min(maximum, Math.max(minimum, value));
}

function errorMessage(error) {
  if (error && typeof error.message === "string" && error.message) {
    return error.message;
  }
  return "An unexpected error occurred.";
}

function isAbortError(error) {
  return Boolean(error && error.name === "AbortError");
}
