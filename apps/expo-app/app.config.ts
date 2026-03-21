import type { ExpoConfig } from "expo/config";

const config: ExpoConfig = {
  name: "xMilo",
  slug: "xmilo",
  version: "0.1.0",
  scheme: "xmilo",
  orientation: "portrait",
  userInterfaceStyle: "dark",
  experiments: {
    typedRoutes: true
  },
  plugins: [
    "expo-router"
  ],
  android: {
    package: process.env.XMILO_ANDROID_PACKAGE ?? "com.hatsunama.xmilo.dev",
    permissions: ["INTERNET", "com.android.vending.BILLING"],
    adaptiveIcon: {
      backgroundColor: "#0B1020"
    }
  },
  extra: {
    sidecarBaseUrl: process.env.EXPO_PUBLIC_SIDECAR_BASE_URL ?? "http://127.0.0.1:42817",
    localhostToken: process.env.EXPO_PUBLIC_LOCALHOST_TOKEN ?? "",
    relayBaseUrl: process.env.EXPO_PUBLIC_RELAY_BASE_URL ?? "http://127.0.0.1:8080",
    revenueCatAndroidApiKey: process.env.EXPO_PUBLIC_RC_ANDROID_API_KEY ?? "",
    revenueCatEntitlementId: process.env.EXPO_PUBLIC_RC_ENTITLEMENT_ID ?? "xmilo_pro"
  }
};

export default config;
