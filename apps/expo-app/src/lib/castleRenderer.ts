import { Platform } from "react-native";

import type { ArtDegradationState } from "../types/contracts";

export type CastleRendererStatus = {
  degradation: ArtDegradationState | null;
};

export function getCastleRendererStatus(): CastleRendererStatus {
  if (Platform.OS !== "android") {
    return {
      degradation: {
        reason: "platform_not_android",
        surface: "lair",
        message: "Native castle rendering is currently Android-only, so xMilo is using the Expo fallback surface."
      }
    };
  }

  return {
    degradation: null
  };
}
