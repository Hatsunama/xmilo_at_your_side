import { NativeModules, Platform } from "react-native";

import type { ArtDegradationState } from "../types/contracts";

export type CastleRendererStatus = {
  mode: "native" | "expo_fallback";
  available: boolean;
  degradation: ArtDegradationState | null;
};

type CastleNativeModuleShape = {
  start?: (wsURL: string) => void;
};

export function getCastleRendererStatus(): CastleRendererStatus {
  if (Platform.OS !== "android") {
    return {
      mode: "expo_fallback",
      available: false,
      degradation: {
        reason: "platform_not_android",
        surface: "lair",
        message: "Native castle rendering is currently Android-only, so xMilo is using the Expo fallback surface."
      }
    };
  }

  const nativeModule = NativeModules.CastleModule as CastleNativeModuleShape | undefined;
  if (!nativeModule?.start) {
    return {
      mode: "expo_fallback",
      available: false,
      degradation: {
        reason: "native_module_unavailable",
        surface: "lair",
        message: "The native castle renderer is not active in this build yet, so xMilo is using the lighter Expo fallback surface."
      }
    };
  }

  return {
    mode: "native",
    available: true,
    degradation: null
  };
}
