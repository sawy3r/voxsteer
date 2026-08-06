"use strict";

class VoxSteerRecorderProcessor extends AudioWorkletProcessor {
  constructor(options) {
    super();
    const requestedSize = Number(options.processorOptions?.chunkSize);
    this.chunkSize = Number.isInteger(requestedSize)
      ? Math.min(16384, Math.max(256, requestedSize))
      : 2048;

    const requestedTargetRate = Number(options.processorOptions?.targetSampleRate);
    this.targetSampleRate = Number.isFinite(requestedTargetRate) && requestedTargetRate > 0
      ? requestedTargetRate
      : 16000;
    const requestedInputMaximum = Number(options.processorOptions?.maxInputSamples);
    this.maxInputSamples = Number.isInteger(requestedInputMaximum) && requestedInputMaximum > 0
      ? requestedInputMaximum
      : Number.POSITIVE_INFINITY;
    const requestedOutputMaximum = Number(options.processorOptions?.maxOutputSamples);
    this.maxOutputSamples = Number.isInteger(requestedOutputMaximum) && requestedOutputMaximum > 0
      ? requestedOutputMaximum
      : Number.POSITIVE_INFINITY;

    this.resampleRatio = sampleRate / this.targetSampleRate;
    this.outputInputRemaining = this.resampleRatio;
    this.resampleSum = 0;
    this.resampleWeight = 0;
    this.buffer = new Float32Array(this.chunkSize);
    this.offset = 0;
    this.inputSamples = 0;
    this.outputSamples = 0;
    this.peak = 0;
    this.sumSquares = 0;
    this.levelSamples = 0;
    this.active = true;
    this.stopped = false;

    this.port.onmessage = (event) => {
      if (event.data?.type === "stop") {
        this.stop();
      }
    };
  }

  process(inputs) {
    if (!this.active) {
      return false;
    }

    const channels = inputs[0];
    if (!channels || channels.length === 0 || channels[0].length === 0) {
      return true;
    }

    const frameCount = channels[0].length;
    for (let frame = 0; frame < frameCount; frame += 1) {
      let mixedSample = 0;
      for (let channel = 0; channel < channels.length; channel += 1) {
        mixedSample += channels[channel][frame] || 0;
      }
      mixedSample /= channels.length;

      this.peak = Math.max(this.peak, Math.abs(mixedSample));
      this.sumSquares += mixedSample * mixedSample;
      this.levelSamples += 1;
      this.inputSamples += 1;
      this.consumeInputSample(mixedSample);

      if (this.inputSamples >= this.maxInputSamples) {
        this.finish("limit");
        break;
      }
    }

    return this.active;
  }

  consumeInputSample(sample) {
    let inputRemaining = 1;
    while (inputRemaining > 1e-9 && this.outputSamples < this.maxOutputSamples) {
      const weight = Math.min(inputRemaining, this.outputInputRemaining);
      this.resampleSum += sample * weight;
      this.resampleWeight += weight;
      inputRemaining -= weight;
      this.outputInputRemaining -= weight;

      if (this.outputInputRemaining <= 1e-9) {
        this.pushOutputSample(this.resampleSum / this.resampleWeight);
        this.resampleSum = 0;
        this.resampleWeight = 0;
        this.outputInputRemaining = this.resampleRatio;
      }
    }
  }

  pushOutputSample(sample) {
    if (this.outputSamples >= this.maxOutputSamples) {
      return;
    }
    this.buffer[this.offset] = sample;
    this.offset += 1;
    this.outputSamples += 1;
    if (this.offset === this.chunkSize) {
      this.flush();
    }
  }

  flush() {
    if (this.offset === 0) {
      return;
    }

    const samples = this.offset === this.buffer.length
      ? this.buffer
      : this.buffer.slice(0, this.offset);
    const rms = this.levelSamples > 0
      ? Math.sqrt(this.sumSquares / this.levelSamples)
      : 0;

    this.port.postMessage(
      {
        type: "chunk",
        samples: samples.buffer,
        peak: this.peak,
        rms,
      },
      [samples.buffer],
    );

    this.buffer = new Float32Array(this.chunkSize);
    this.offset = 0;
    this.peak = 0;
    this.sumSquares = 0;
    this.levelSamples = 0;
  }

  stop() {
    this.finish("stop");
  }

  finish(reason) {
    if (this.stopped) {
      return;
    }
    this.stopped = true;
    if (this.resampleWeight > 0 && this.outputSamples < this.maxOutputSamples) {
      this.pushOutputSample(this.resampleSum / this.resampleWeight);
    }
    this.flush();
    this.active = false;
    if (reason === "limit") {
      this.port.postMessage({ type: "limit" });
    }
    this.port.postMessage({ type: "stopped" });
  }
}

registerProcessor("voxsteer-recorder", VoxSteerRecorderProcessor);
