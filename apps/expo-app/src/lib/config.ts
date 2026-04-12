import Constants from "expo-constants";
import { NativeModules, Platform } from "react-native";

const extra = (Constants.expoConfig?.extra ?? {}) as Record<string, string | undefined>;

function defaultSidecarBaseUrl() {
  return "http://127.0.0.1:42817";
}

export const SIDECAR_BASE_URL = extra.sidecarBaseUrl ?? defaultSidecarBaseUrl();
export const LOCALHOST_TOKEN = extra.localhostToken ?? "";
export const RELAY_BASE_URL = extra.relayBaseUrl ?? "http://127.0.0.1:8080";
export const WEBSITE_BASE_URL = extra.websiteBaseUrl ?? "https://sol.xmiloatyourside.com";
export const GITHUB_REPO_URL = "https://github.com/Hatsunama/xmilo_at_your_side";
export const SIDECAR_RELEASES_URL = `${GITHUB_REPO_URL}/releases`;
export const PRIVACY_POLICY_URL = `${WEBSITE_BASE_URL}/privacy`;
export const SUPPORT_URL = `${WEBSITE_BASE_URL}/support`;
export const DELETE_ACCOUNT_URL = `${WEBSITE_BASE_URL}/delete-account`;

type SidecarAssetName = "xmilo-sidecar-arm64" | "xmilo-sidecar-arm" | "xmilo-sidecar-amd64";
type SidecarArchResolution = {
  assetName: SidecarAssetName;
  checksumName: `${SidecarAssetName}.sha256`;
  abiLabel: string;
  detectedFrom: "android_abis" | "platform_fallback";
};

type PlatformConstantsShape = {
  SupportedAbis?: string[];
  supportedAbis?: string[];
};

function getSupportedAbis(): string[] {
  if (Platform.OS !== "android") {
    return [];
  }

  const platformConstants = Platform.constants as PlatformConstantsShape | undefined;
  const nativePlatformConstants = NativeModules.PlatformConstants as PlatformConstantsShape | undefined;
  return (
    platformConstants?.SupportedAbis ??
    platformConstants?.supportedAbis ??
    nativePlatformConstants?.SupportedAbis ??
    nativePlatformConstants?.supportedAbis ??
    []
  );
}

export function resolveSidecarReleaseAsset(): SidecarArchResolution {
  const abis = getSupportedAbis().map((abi) => abi.toLowerCase());

  if (abis.some((abi) => abi.includes("arm64") || abi.includes("aarch64"))) {
    return {
      assetName: "xmilo-sidecar-arm64",
      checksumName: "xmilo-sidecar-arm64.sha256",
      abiLabel: "arm64-v8a",
      detectedFrom: "android_abis"
    };
  }

  if (abis.some((abi) => abi.includes("armeabi") || abi === "arm")) {
    return {
      assetName: "xmilo-sidecar-arm",
      checksumName: "xmilo-sidecar-arm.sha256",
      abiLabel: "armeabi-v7a",
      detectedFrom: "android_abis"
    };
  }

  if (abis.some((abi) => abi.includes("x86_64") || abi.includes("amd64"))) {
    return {
      assetName: "xmilo-sidecar-amd64",
      checksumName: "xmilo-sidecar-amd64.sha256",
      abiLabel: "x86_64",
      detectedFrom: "android_abis"
    };
  }

  return {
    assetName: "xmilo-sidecar-arm64",
    checksumName: "xmilo-sidecar-arm64.sha256",
    abiLabel: "arm64-v8a",
    detectedFrom: "platform_fallback"
  };
}
