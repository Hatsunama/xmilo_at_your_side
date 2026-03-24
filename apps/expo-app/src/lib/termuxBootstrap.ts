import { NativeModules, Platform } from "react-native";

import { TERMUX_BOOTSTRAP_COMMAND } from "./config";

type TermuxBootstrapStatus = {
  termuxInstalled: boolean;
  runCommandPermissionGranted: boolean;
};

type TermuxBootstrapModuleShape = {
  getStatus?: () => Promise<TermuxBootstrapStatus>;
  openAppPermissionSettings?: () => Promise<boolean>;
  openTermux?: () => Promise<boolean>;
  runBootstrap?: (command: string) => Promise<boolean>;
};

const nativeModule = NativeModules.TermuxBootstrapModule as TermuxBootstrapModuleShape | undefined;

function shellEscape(value: string) {
  return `'${value.replace(/'/g, `'\"'\"'`)}'`;
}

export function hasNativeTermuxBootstrap() {
  return Platform.OS === "android" && Boolean(nativeModule?.runBootstrap);
}

export function buildTermuxBootstrapCommand({
  bearerToken,
  relayBaseUrl,
  command = TERMUX_BOOTSTRAP_COMMAND,
}: {
  bearerToken: string;
  relayBaseUrl: string;
  command?: string;
}) {
  const commandParts: string[] = [];

  if (bearerToken.trim()) {
    commandParts.push(`export XMILO_BEARER_TOKEN=${shellEscape(bearerToken.trim())}`);
  }

  if (relayBaseUrl.trim()) {
    commandParts.push(`export XMILO_RELAY_URL=${shellEscape(relayBaseUrl.trim())}`);
  }

  commandParts.push(command);
  return commandParts.join("; ");
}

export async function getTermuxBootstrapStatus(): Promise<TermuxBootstrapStatus> {
  if (!nativeModule?.getStatus) {
    return {
      termuxInstalled: false,
      runCommandPermissionGranted: false
    };
  }
  return nativeModule.getStatus();
}

export async function openTermuxBootstrapPermissionSettings() {
  if (!nativeModule?.openAppPermissionSettings) {
    throw new Error("Native Termux permission settings are not available in this build yet.");
  }
  return nativeModule.openAppPermissionSettings();
}

export async function openTermuxApp() {
  if (!nativeModule?.openTermux) {
    throw new Error("Native Termux launch is not available in this build yet.");
  }
  return nativeModule.openTermux();
}

export async function runTermuxBootstrap(command = TERMUX_BOOTSTRAP_COMMAND) {
  if (!nativeModule?.runBootstrap) {
    throw new Error("Native Termux bootstrap is not available in this build yet.");
  }
  return nativeModule.runBootstrap(command);
}
