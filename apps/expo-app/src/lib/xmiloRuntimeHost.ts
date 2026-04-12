import { NativeModules, Platform } from "react-native";

import { LOCALHOST_TOKEN } from "./config";

export type XMiloclawRuntimeStatus = {
  notificationsGranted: boolean;
  appearOnTopGranted: boolean;
  batteryUnrestricted: boolean;
  accessibilityEnabled: boolean;
  runtimeHostStarted: boolean;
  bridgeConnected: boolean;
  hostReady: boolean;
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
      bridgeConnected: false,
      hostReady: false,
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

export async function resolveLocalhostBearerToken() {
  if (!localhostBearerTokenPromise) {
    localhostBearerTokenPromise = (async () => {
      if (hasNativeXMiloclawRuntimeHost()) {
        return getXMiloclawLocalhostBearerToken();
      }

      if (!LOCALHOST_TOKEN) {
        throw new Error("Missing EXPO_PUBLIC_LOCALHOST_TOKEN");
      }

      return LOCALHOST_TOKEN;
    })();
  }

  return localhostBearerTokenPromise;
}
