import type { ExpoConfig } from "expo/config";

const appVersion = "0.1.5";
const androidPackage = process.env.XMILO_ANDROID_PACKAGE ?? "com.hatsunama.xmilo";
const publicRelayBaseUrl = process.env.EXPO_PUBLIC_RELAY_BASE_URL || "https://xmiloatyourside.com";

const config: ExpoConfig = {
  name: "xMilo",
  slug: "xmilo",
  version: appVersion,
  scheme: "xmilo",
  orientation: "portrait",
  userInterfaceStyle: "dark",
  experiments: {
    typedRoutes: true
  },
  plugins: [
    "expo-router",
    "expo-notifications",
    "expo-splash-screen"
  ],
  android: {
    package: androidPackage,
    permissions: [
      "INTERNET",
      "POST_NOTIFICATIONS",
      "CAMERA",
      "RECORD_AUDIO",
      "ACCESS_FINE_LOCATION",
      "READ_MEDIA_IMAGES",
      "READ_EXTERNAL_STORAGE"
    ],
    adaptiveIcon: {
      backgroundColor: "#0B1020"
    }
  },
  extra: {
    // Treat empty-string env vars as "unset" so builds don't accidentally ship with missing config.
    sidecarBaseUrl: process.env.EXPO_PUBLIC_SIDECAR_BASE_URL || "http://localhost:42817",
    localhostToken: process.env.EXPO_PUBLIC_LOCALHOST_TOKEN || "",
    relayBaseUrl: publicRelayBaseUrl,
    websiteBaseUrl: process.env.EXPO_PUBLIC_WEBSITE_BASE_URL || "https://sol.xmiloatyourside.com"
  }
};

export default config;
