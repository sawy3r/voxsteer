import assert from "node:assert/strict";

globalThis.AudioWorkletProcessor = class {
  constructor() {
    this.port = {
      messages: [],
      onmessage: null,
      postMessage: (message) => this.port.messages.push(message),
    };
  }
};
globalThis.registerProcessor = (_name, processor) => {
  globalThis.RecorderProcessor = processor;
};
globalThis.sampleRate = 48000;

await import("../internal/webapp/assets/recorder-worklet.js");

for (const rate of [16000, 44100, 48000]) {
  globalThis.sampleRate = rate;
  const seconds = 2;
  const processor = new globalThis.RecorderProcessor({
    processorOptions: {
      chunkSize: 256,
      targetSampleRate: 16000,
      maxInputSamples: rate * seconds,
      maxOutputSamples: 16000 * seconds,
    },
  });
  const left = new Float32Array(rate * seconds).fill(0.5);
  const right = new Float32Array(rate * seconds).fill(-0.25);
  processor.process([[left, right]]);

  const chunks = processor.port.messages.filter((message) => message.type === "chunk");
  const sampleCount = chunks.reduce(
    (count, message) => count + message.samples.byteLength / Float32Array.BYTES_PER_ELEMENT,
    0,
  );
  assert.equal(sampleCount, 16000 * seconds, `${rate} Hz output length`);
  for (const message of chunks) {
    for (const sample of new Float32Array(message.samples)) {
      assert.ok(Math.abs(sample - 0.125) < 1e-5, `${rate} Hz downmix value`);
    }
  }
  assert.ok(processor.port.messages.some((message) => message.type === "limit"));
  assert.ok(processor.port.messages.some((message) => message.type === "stopped"));
}

console.log("recorder worklet tests passed");
