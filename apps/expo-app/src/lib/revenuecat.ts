import { Platform } from "react-native";
import Purchases, { LOG_LEVEL } from "react-native-purchases";
import RevenueCatUI, { PAYWALL_RESULT } from "react-native-purchases-ui";
import Constants from "expo-constants";

const extra = (Constants.expoConfig?.extra ?? {}) as Record<string, string | undefined>;

const RC_ANDROID_API_KEY = extra.revenueCatAndroidApiKey ?? "";
const ENTITLEMENT_ID = extra.revenueCatEntitlementId ?? "xmilo_pro";

let configuredAppUserID = "";

export function getRevenueCatEntitlementId() {
  return ENTITLEMENT_ID;
}

export function isRevenueCatConfigured() {
  return Boolean(RC_ANDROID_API_KEY);
}

export function configureRevenueCat(appUserID: string) {
  if (Platform.OS !== "android") return;
  if (!RC_ANDROID_API_KEY) {
    throw new Error("Missing EXPO_PUBLIC_RC_ANDROID_API_KEY");
  }
  if (!appUserID) {
    throw new Error("Missing RevenueCat app user id");
  }
  if (configuredAppUserID === appUserID) {
    return;
  }

  Purchases.setLogLevel(__DEV__ ? LOG_LEVEL.VERBOSE : LOG_LEVEL.WARN);
  Purchases.configure({ apiKey: RC_ANDROID_API_KEY, appUserID });
  configuredAppUserID = appUserID;
}

export async function getCustomerInfo() {
  return Purchases.getCustomerInfo();
}

export function hasRequiredEntitlement(customerInfo: any) {
  return typeof customerInfo?.entitlements?.active?.[ENTITLEMENT_ID] !== "undefined";
}

export async function presentSubscriptionPaywall() {
  const result = await RevenueCatUI.presentPaywall();
  return result as PAYWALL_RESULT;
}

export async function restoreRevenueCatPurchases() {
  const customerInfo = await Purchases.restorePurchases();
  return {
    customerInfo,
    entitled: hasRequiredEntitlement(customerInfo)
  };
}

export { PAYWALL_RESULT };
