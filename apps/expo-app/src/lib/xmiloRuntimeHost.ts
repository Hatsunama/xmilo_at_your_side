import { NativeModules, Platform } from "react-native";

import { LOCALHOST_TOKEN } from "./config";

export type XMiloclawRuntimeStatus = {
  notificationsGranted: boolean;
  appearOnTopGranted: boolean;
  batteryUnrestricted: boolean;
  accessibilityEnabled: boolean;
  runtimeHostStarted: boolean;
  foregroundServiceStarted?: boolean;
  runtimeFilesPrepared?: boolean;
  sidecarProcessLaunched?: boolean;
  sidecarProcessAlive?: boolean;
  bridgeConnected: boolean;
  taskRouteSurfaceReady?: boolean;
  hostReady: boolean;
  llmMode?: string;
  byokProvider?: string;
  subscriptionEntitled?: boolean;
  bringYourOwnKeyActive?: boolean;
  phase9ApiKeyAccess?: boolean;
  firstTaskEligible?: boolean;
  relayLlmTurnAllowed?: boolean;
  localLlmTurnAllowed?: boolean;
  lastRuntimeStage?: string;
  lastHealthCode?: number;
  lastReadyCode?: number;
  lastHealthCategory?: string;
  lastReadyCategory?: string;
  lastProcessExitCode?: number;
  lastProcessUptimeMillis?: number;
  firstSafeStdoutLine?: string;
  firstSafeStdoutCategory?: string;
  firstSafeStderrLine?: string;
  firstSafeStderrCategory?: string;
  lastProcessErrorSummary?: string;
  restartAttempted?: boolean;
  restartSucceeded?: boolean;
  lastError?: string;
  checkedAtMillis?: number;
};

type XMiloclawRuntimeModuleShape = {
  getStatus?: () => Promise<XMiloclawRuntimeStatus>;
  startRuntimeHost?: () => Promise<XMiloclawRuntimeStatus>;
  restartRuntimeHost?: () => Promise<XMiloclawRuntimeStatus>;
  openAccessibilitySettings?: () => Promise<boolean>;
  openNotificationSettings?: () => Promise<boolean>;
  openOverlaySettings?: () => Promise<boolean>;
  openBatteryOptimizationSettings?: () => Promise<boolean>;
  getLocalhostBearerToken?: () => Promise<string>;
  saveLocalByokApiKey?: (provider: string, apiKey: string, baseUrl: string, model: string) => Promise<string>;
  getLocalByokStatus?: () => Promise<string>;
};

const nativeModule = NativeModules.XMiloclawRuntimeModule as XMiloclawRuntimeModuleShape | undefined;
let localhostBearerTokenPromise: Promise<string> | null = null;

export function hasNativeXMiloclawRuntimeHost() {
  return Platform.OS === "android" && Boolean(nativeModule?.getStatus && nativeModule?.startRuntimeHost && nativeModule?.restartRuntimeHost);
}

export async function getXMiloclawRuntimeStatus(): Promise<XMiloclawRuntimeStatus> {
  if (!nativeModule?.getStatus) {
    return {
      notificationsGranted: false,
      appearOnTopGranted: false,
      batteryUnrestricted: false,
      accessibilityEnabled: false,
      runtimeHostStarted: false,
      foregroundServiceStarted: false,
      runtimeFilesPrepared: false,
      sidecarProcessLaunched: false,
      sidecarProcessAlive: false,
      bridgeConnected: false,
      taskRouteSurfaceReady: false,
      hostReady: false,
      lastRuntimeStage: "native_module_missing",
      lastHealthCategory: "unknown",
      lastReadyCategory: "unknown",
      firstSafeStdoutCategory: "none",
      firstSafeStderrCategory: "none",
    };
  }
  return nativeModule.getStatus();
}

export async function startXMiloclawRuntimeHost(): Promise<XMiloclawRuntimeStatus> {
  if (!nativeModule?.startRuntimeHost) {
    throw new Error("Native xMilo runtime host is not available in this build yet.");
  }
  return nativeModule.startRuntimeHost();
}

export async function restartXMiloclawRuntimeHost(): Promise<XMiloclawRuntimeStatus> {
  if (!nativeModule?.restartRuntimeHost) {
    throw new Error("Native xMilo runtime host restart is not available in this build yet.");
  }
  return nativeModule.restartRuntimeHost();
}

export async function openXMiloclawAccessibilitySettings() {
  if (!nativeModule?.openAccessibilitySettings) {
    throw new Error("Native accessibility settings launch is not available in this build yet.");
  }
  return nativeModule.openAccessibilitySettings();
}

export async function openXMiloclawNotificationSettings() {
  if (!nativeModule?.openNotificationSettings) {
    throw new Error("Native notification settings launch is not available in this build yet.");
  }
  return nativeModule.openNotificationSettings();
}

export async function openXMiloclawOverlaySettings() {
  if (!nativeModule?.openOverlaySettings) {
    throw new Error("Native overlay settings launch is not available in this build yet.");
  }
  return nativeModule.openOverlaySettings();
}

export async function openXMiloclawBatteryOptimizationSettings() {
  if (!nativeModule?.openBatteryOptimizationSettings) {
    throw new Error("Native battery optimization settings launch is not available in this build yet.");
  }
  return nativeModule.openBatteryOptimizationSettings();
}

export async function getXMiloclawLocalhostBearerToken() {
  if (!nativeModule?.getLocalhostBearerToken) {
    throw new Error("Native localhost bearer token source is not available in this build yet.");
  }
  const token = await nativeModule.getLocalhostBearerToken();
  if (!token) {
    throw new Error("Localhost bearer token was empty.");
  }
  return token;
}

export type LocalByokStatus = {
  provider: ByokProvider;
  keyFileReady: boolean;
  keyFilePath: string;
  baseUrlReady: boolean;
  model: string;
  byokReady: boolean;
};

export type ByokProvider = "xai" | "openai" | "anthropic" | "ollama";

function parseLocalByokStatus(raw: string): LocalByokStatus {
  const parsed = JSON.parse(raw) as Partial<LocalByokStatus>;
  return {
    provider: parseByokProvider(parsed.provider),
    keyFileReady: Boolean(parsed.keyFileReady),
    keyFilePath: String(parsed.keyFilePath ?? ""),
    baseUrlReady: Boolean(parsed.baseUrlReady),
    model: String(parsed.model ?? ""),
    byokReady: Boolean(parsed.byokReady),
  };
}

function parseByokProvider(provider: unknown): ByokProvider {
  if (provider === "openai" || provider === "anthropic" || provider === "ollama") return provider;
  return "xai";
}

export async function saveXMiloclawLocalByokApiKey(provider: ByokProvider, apiKey: string, baseUrl = "", model = "") {
  if (!nativeModule?.saveLocalByokApiKey) {
    throw new Error("Native local API key storage is not available in this build yet.");
  }
  return parseLocalByokStatus(await nativeModule.saveLocalByokApiKey(provider, apiKey, baseUrl, model));
}

export async function getXMiloclawLocalByokStatus() {
  if (!nativeModule?.getLocalByokStatus) {
    return { provider: "xai", keyFileReady: false, keyFilePath: "", baseUrlReady: false, model: "", byokReady: false } satisfies LocalByokStatus;
  }
  return parseLocalByokStatus(await nativeModule.getLocalByokStatus());
}

export async function resolveLocalhostBearerToken() {
  if (!localhostBearerTokenPromise) {
    localhostBearerTokenPromise = (async () => {
      if (hasNativeXMiloclawRuntimeHost()) {
        return getXMiloclawLocalhostBearerToken();
      }

      if (!LOCALHOST_TOKEN) {
        throw new Error("Native localhost bearer source unavailable, and non-native EXPO_PUBLIC_LOCALHOST_TOKEN fallback is unset.");
      }

      return LOCALHOST_TOKEN;
    })();
  }

  return localhostBearerTokenPromise;
}
