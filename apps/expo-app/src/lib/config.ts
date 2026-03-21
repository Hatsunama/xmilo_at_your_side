import Constants from "expo-constants";

const extra = (Constants.expoConfig?.extra ?? {}) as Record<string, string | undefined>;

export const SIDECAR_BASE_URL = extra.sidecarBaseUrl ?? "http://127.0.0.1:42817";
export const LOCALHOST_TOKEN = extra.localhostToken ?? "";
