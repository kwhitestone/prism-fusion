import { validateMessageCapabilities, validateRemoteApplication } from "./remote-registry.js";
import type { RemoteApplicationManifest, RemoteMessageCapabilities } from "./remote-registry.js";

export const REMOTE_PROTOCOL = "prism-fusion/remote";
export const REMOTE_PROTOCOL_VERSION = 1;
export type RemoteChannelState = "idle" | "connecting" | "ready" | "failed" | "disposed";
export interface RemoteEnvelope {
  protocol: typeof REMOTE_PROTOCOL;
  version: number;
  appId: string;
  instanceId: string;
  type: string;
  payload?: unknown;
}
export interface RemoteMessageTarget { postMessage(message: unknown, targetOrigin: string): void; }
export interface RemoteMessageEvent { source: unknown; origin: string; data: unknown; }
type Receiver = (type: string, payload: unknown) => void;

function envelope(data: unknown): data is RemoteEnvelope {
  if (!data || typeof data !== "object" || Array.isArray(data)) return false;
  const message = data as RemoteEnvelope;
  return message.protocol === REMOTE_PROTOCOL && Number.isSafeInteger(message.version) &&
    typeof message.appId === "string" && typeof message.type === "string" &&
    typeof message.instanceId === "string" && /^[a-zA-Z0-9-]{16,128}$/.test(message.instanceId);
}

const message = (appId: string, instanceId: string, type: string, payload?: unknown): RemoteEnvelope => ({
  protocol: REMOTE_PROTOCOL, version: REMOTE_PROTOCOL_VERSION, appId, instanceId, type,
  ...(payload === undefined ? {} : { payload })
});

export interface RemoteHostChannelOptions {
  application: RemoteApplicationManifest;
  target: RemoteMessageTarget;
  onMessage?: Receiver;
  onStateChange?: (state: RemoteChannelState) => void;
  timeoutMs?: number;
}

/** One channel per iframe document. Re-start on every load to rotate its identity. */
export function createRemoteHostChannel(options: RemoteHostChannelOptions) {
  validateRemoteApplication(options.application);
  const application = structuredClone(options.application);
  const origin = new URL(application.entryUrl).origin;
  const timeoutMs = options.timeoutMs ?? 15_000;
  if (!Number.isFinite(timeoutMs) || timeoutMs <= 0 || timeoutMs > 120_000) throw new Error("Invalid remote readiness timeout");
  let state: RemoteChannelState = "idle";
  let instanceId = "";
  let timer: ReturnType<typeof setTimeout> | undefined;
  const clearTimer = () => { if (timer !== undefined) clearTimeout(timer); timer = undefined; };
  const transition = (next: RemoteChannelState) => { state = next; options.onStateChange?.(next); };
  return {
    get state(): RemoteChannelState { return state; },
    get instanceId(): string { return instanceId; },
    start(): void {
      if (state === "disposed") throw new Error("Remote channel is disposed");
      clearTimer();
      instanceId = globalThis.crypto.randomUUID();
      transition("connecting");
      timer = setTimeout(() => { timer = undefined; transition("failed"); }, timeoutMs);
      (timer as unknown as { unref?: () => void }).unref?.();
      try { options.target.postMessage(message(application.id, instanceId, "host:init"), origin); }
      catch { clearTimer(); transition("failed"); }
    },
    receive(event: RemoteMessageEvent): boolean {
      if (state === "disposed" || event.source !== options.target || event.origin !== origin || !envelope(event.data)) return false;
      const data = event.data;
      if (data.appId !== application.id || data.instanceId !== instanceId) return false;
      if (data.version !== REMOTE_PROTOCOL_VERSION) { clearTimer(); transition("failed"); return false; }
      if (data.type === "child:ready" && state === "connecting") {
        clearTimer(); transition("ready"); return true;
      }
      if (state !== "ready" || !application.messages.fromChild.includes(data.type)) return false;
      options.onMessage?.(data.type, data.payload);
      return true;
    },
    send(type: string, payload?: unknown): boolean {
      if (state !== "ready" || !application.messages.toChild.includes(type)) return false;
      try { options.target.postMessage(message(application.id, instanceId, type, payload), origin); return true; }
      catch { clearTimer(); transition("failed"); return false; }
    },
    dispose(): void { if (state !== "disposed") { clearTimer(); transition("disposed"); } }
  };
}

export interface RemoteChildChannelOptions {
  appId: string;
  hostOrigin: string;
  parent: RemoteMessageTarget;
  messages: RemoteMessageCapabilities;
  onMessage?: Receiver;
  onConnected?: () => void;
}

/** Caller owns the DOM listener and invokes ready only after plugin installation. */
export function createRemoteChildChannel(options: RemoteChildChannelOptions) {
  if (!/^[a-z0-9](?:[a-z0-9._-]*[a-z0-9])?$/.test(options.appId)) throw new Error("Invalid remote child ID");
  if (new URL(options.hostOrigin).origin !== options.hostOrigin || !/^https?:\/\//.test(options.hostOrigin)) throw new Error("Invalid remote host origin");
  validateMessageCapabilities(options.messages);
  const capabilities = structuredClone(options.messages);
  let instanceId: string | undefined;
  let mounted = false;
  let connected = false;
  let disposed = false;
  const acknowledge = () => {
    if (!mounted || !instanceId || disposed) return;
    connected = true;
    options.parent.postMessage(message(options.appId, instanceId, "child:ready"), options.hostOrigin);
    options.onConnected?.();
  };
  return {
    get connected(): boolean { return connected && !disposed; },
    ready(): void { if (disposed) throw new Error("Remote child channel is disposed"); mounted = true; acknowledge(); },
    receive(event: RemoteMessageEvent): boolean {
      if (disposed || event.source !== options.parent || event.origin !== options.hostOrigin || !envelope(event.data)) return false;
      const data = event.data;
      if (data.appId !== options.appId || data.version !== REMOTE_PROTOCOL_VERSION) return false;
      if (data.type === "host:init") {
        connected = false;
        instanceId = data.instanceId;
        acknowledge();
        return true;
      }
      if (!connected || data.instanceId !== instanceId || !capabilities.toChild.includes(data.type)) return false;
      options.onMessage?.(data.type, data.payload);
      return true;
    },
    send(type: string, payload?: unknown): boolean {
      if (!connected || disposed || !instanceId || !capabilities.fromChild.includes(type)) return false;
      options.parent.postMessage(message(options.appId, instanceId, type, payload), options.hostOrigin);
      return true;
    },
    dispose(): void { disposed = true; connected = false; instanceId = undefined; }
  };
}
