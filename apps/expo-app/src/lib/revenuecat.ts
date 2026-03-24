import Constants from "expo-constants";
import Purchases from "react-native-purchases";
import RevenueCatUI, { PAYWALL_RESULT } from "react-native-purchases-ui";

type ExtraConfig = {
  revenueCatAndroidApiKey?: string;
  revenueCatEntitlementId?: string;
};

const extra = (Constants.expoConfig?.extra ?? {}) as ExtraConfig;
const apiKey = extra.revenueCatAndroidApiKey?.trim() ?? "";
const entitlementId = extra.revenueCatEntitlementId?.trim() || "xmilo_pro";

let configuredAppUserID: string | null = null;

export { PAYWALL_RESULT };

export function isRevenueCatConfigured() {
  return Boolean(apiKey);
}

export function configureRevenueCat(appUserID: string) {
  if (!apiKey) {
    return;
  }
  const trimmed = appUserID.trim();
  if (!trimmed) {
    return;
  }
  if (configuredAppUserID === trimmed) {
    return;
  }

  Purchases.configure({ apiKey, appUserID: trimmed });
  configuredAppUserID = trimmed;
}

export async function presentSubscriptionPaywall() {
  if (!apiKey) {
    return PAYWALL_RESULT.ERROR;
  }
  return RevenueCatUI.presentPaywall({ displayCloseButton: true });
}

export async function getCustomerInfo() {
  if (!apiKey) {
    return null;
  }
  return Purchases.getCustomerInfo();
}

export function hasRequiredEntitlement(customerInfo: any) {
  const active = customerInfo?.entitlements?.active;
  return Boolean(active && entitlementId && active[entitlementId]);
}

export async function restoreRevenueCatPurchases(): Promise<{ entitled: boolean; customerInfo: any | null }> {
  if (!apiKey) {
    return { entitled: false, customerInfo: null };
  }

  const info = await Purchases.restorePurchases();
  return { entitled: hasRequiredEntitlement(info), customerInfo: info };
}

