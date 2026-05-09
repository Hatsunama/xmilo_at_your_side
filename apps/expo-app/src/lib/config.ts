import Constants from "expo-constants";

const extra = (Constants.expoConfig?.extra ?? {}) as Record<string, string | undefined>;

export const SIDECAR_BASE_URL = extra.sidecarBaseUrl ?? "http://127.0.0.1:42817";
export const LOCALHOST_TOKEN = extra.localhostToken ?? "";
export const RELAY_BASE_URL = extra.relayBaseUrl ?? "http://127.0.0.1:8080";
export const WEBSITE_BASE_URL = extra.websiteBaseUrl ?? "https://sol.xmiloatyourside.com";
export const PRIVACY_POLICY_URL = `${WEBSITE_BASE_URL}/privacy`;
export const SUPPORT_URL = `${WEBSITE_BASE_URL}/support`;
export const DELETE_ACCOUNT_URL = `${WEBSITE_BASE_URL}/delete-account`;
