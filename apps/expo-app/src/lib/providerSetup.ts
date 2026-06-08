import type { LocalProviderOption, LocalProviderResolvedConfig } from "./bridge";

type ProviderSetupInput = Partial<LocalProviderOption & LocalProviderResolvedConfig> | null | undefined;

export type ProviderSetupMode = "cloud_canonical" | "guided_local_server" | "api_key" | "sidecar_resolved";
export type ProviderDisplayRow =
  | { kind: "provider"; id: string; label: string; option: LocalProviderOption }
  | { kind: "grouped_ollama"; id: "ollama_group"; label: "Ollama"; localId: "ollama"; cloudId: "ollama_cloud" };

export const OLLAMA_ACCEPTANCE_TITLE = "Advanced local model setup";
export const OLLAMA_ACCEPTANCE_BODY =
  "Ollama Local uses a reachable Ollama server. It does not use Ollama Cloud or an ollama.com API key in this setup.\n\nThis setup is more advanced than OpenAI, xAI, Claude, or Ollama Cloud because xMilo has to connect to your local Ollama server before it can use the model.\n\nYou can continue if you already have Ollama Local set up. Otherwise, choose Ollama Cloud or another API-key provider for simpler setup.";
export const OLLAMA_ACCEPTANCE_PRIMARY = "Use Ollama Local";
export const OLLAMA_ACCEPTANCE_SECONDARY = "Choose another provider";
export const OLLAMA_CHOICE_TITLE = "Choose Ollama setup";
export const OLLAMA_CHOICE_BODY = "Use Ollama Cloud with an ollama.com API key, or use Ollama Local with a server running on this phone or a computer your phone can reach.";
export const OLLAMA_CHOICE_CLOUD = "Ollama Cloud";
export const OLLAMA_CHOICE_LOCAL = "Ollama Local";
export const OLLAMA_CHOICE_CANCEL = "Cancel";
export const OLLAMA_SETUP_UNAVAILABLE_TITLE = "Ollama setup unavailable";
export const OLLAMA_SETUP_UNAVAILABLE_MESSAGE = "xMilo could not load both Ollama setup options. Try again after restarting setup.";
export const OLLAMA_CONNECTION_MESSAGE = "Ollama Local must be running on this phone or on a computer your phone can reach before xMilo can use it.";
export const OLLAMA_MANUAL_URL_INVALID_MESSAGE = "Check the Ollama Local server address and try again.";

const knownCloudProviderIds = new Set(["openai", "xai", "anthropic", "ollama_cloud"]);

function providerId(provider: ProviderSetupInput) {
  return String(provider?.id ?? provider?.provider ?? "").trim().toLowerCase();
}

function hasString(value: unknown, expected: string) {
  return typeof value === "string" && value.trim() === expected;
}

export function getProviderLabel(provider: ProviderSetupInput, fallback = "Provider") {
  const id = providerId(provider);
  if (id === "ollama") return "Ollama Local";
  if (id === "ollama_cloud") return "Ollama Cloud";
  return String(provider?.label ?? provider?.provider ?? provider?.id ?? fallback);
}

export function getProviderSetupMode(provider: ProviderSetupInput): ProviderSetupMode {
  if (hasString(provider?.base_url_entry_mode, "guided_local_server")) return "guided_local_server";
  if (hasString(provider?.setup_guidance_code, "ollama_connection_target_required")) return "guided_local_server";
  if (hasString(provider?.connection_kind, "local_server") || provider?.requires_connection_target === true) return "guided_local_server";
  if (hasString(provider?.base_url_entry_mode, "hidden_canonical")) return "cloud_canonical";
  if (providerId(provider) === "ollama") return "guided_local_server";
  if (knownCloudProviderIds.has(providerId(provider))) return "cloud_canonical";
  if (provider?.key_required === true) return "api_key";
  return "sidecar_resolved";
}

export function isGuidedLocalServerProvider(provider: ProviderSetupInput) {
  return getProviderSetupMode(provider) === "guided_local_server";
}

export function usesCanonicalProviderEndpoint(provider: ProviderSetupInput) {
  return getProviderSetupMode(provider) === "cloud_canonical";
}

export function allowsAdvancedManualServer(provider: ProviderSetupInput) {
  if (typeof provider?.advanced_manual_base_url_allowed === "boolean") return provider.advanced_manual_base_url_allowed;
  return isGuidedLocalServerProvider(provider) && providerId(provider) === "ollama" && provider?.custom_base_url_allowed !== false;
}

export function isProviderApiKeyRequired(provider: ProviderSetupInput) {
  return provider?.key_required === true;
}

export function findProviderOptionById(options: LocalProviderOption[], id: string) {
  return options.find((option) => providerId(option) === id);
}

export function getOllamaProviderChoices(options: LocalProviderOption[]) {
  return {
    local: findProviderOptionById(options, "ollama"),
    cloud: findProviderOptionById(options, "ollama_cloud")
  };
}

export function isGroupedOllamaProviderRow(row: ProviderDisplayRow): row is Extract<ProviderDisplayRow, { kind: "grouped_ollama" }> {
  return row.kind === "grouped_ollama";
}

export function buildProviderDisplayRows(options: LocalProviderOption[]): ProviderDisplayRow[] {
  const choices = getOllamaProviderChoices(options);
  const shouldGroupOllama = Boolean(choices.local && choices.cloud);
  const rows: ProviderDisplayRow[] = [];
  for (const option of options) {
    const id = providerId(option);
    if (id === "ollama" || id === "ollama_cloud") {
      if (shouldGroupOllama && id === "ollama") {
        rows.push({ kind: "grouped_ollama", id: "ollama_group", label: "Ollama", localId: "ollama", cloudId: "ollama_cloud" });
      } else if (!shouldGroupOllama) {
        rows.push({ kind: "provider", id: option.id, label: getProviderLabel(option), option });
      }
      continue;
    }
    rows.push({ kind: "provider", id: option.id, label: getProviderLabel(option), option });
  }
  return rows;
}

export function getProviderSetupCopy(provider: ProviderSetupInput, fallbackLabel = "Provider") {
  const label = getProviderLabel(provider, fallbackLabel);
  const mode = getProviderSetupMode(provider);
  if (mode === "guided_local_server") {
    return {
      title: `Connect to ${label}`,
      body: OLLAMA_CONNECTION_MESSAGE,
      primary: `Set up ${label} connection`
    };
  }
  const keyCopy = providerId(provider) === "ollama_cloud" ? "Paste your Ollama API key to continue." : "Paste your API key to continue.";
  return {
    title: "Enter your API key",
    body: `${usesCanonicalProviderEndpoint(provider) ? `xMilo will use the standard ${label} endpoint automatically. ` : ""}${isProviderApiKeyRequired(provider) ? keyCopy : "Provider setup is checked before saving."}`,
    primary: "Save & continue"
  };
}

export function getProviderSaveFailureCopy(reasonOrMessage: string, provider: ProviderSetupInput) {
  const label = getProviderLabel(provider);
  if (reasonOrMessage === "local_provider_manual_url_invalid" || reasonOrMessage.includes("Ollama Local server address is not valid") || reasonOrMessage.includes(OLLAMA_MANUAL_URL_INVALID_MESSAGE)) {
    return { title: "Ollama Local server address is not valid", message: OLLAMA_MANUAL_URL_INVALID_MESSAGE };
  }
  if (reasonOrMessage === "local_provider_connection_target_required" || reasonOrMessage.includes("Ollama Local must be running")) {
    return { title: "Ollama Local connection needed", message: OLLAMA_CONNECTION_MESSAGE };
  }
  if ((reasonOrMessage === "missing_base_url" || reasonOrMessage === "local_provider_base_url_required") && isGuidedLocalServerProvider(provider)) {
    return { title: "Ollama Local connection needed", message: OLLAMA_CONNECTION_MESSAGE };
  }
  if (reasonOrMessage === "missing_key" || (isProviderApiKeyRequired(provider) && reasonOrMessage.includes("API key"))) {
    return { title: `${label} key not saved`, message: `${label} needs an API key before it can handle new tasks.` };
  }
  return { title: isProviderApiKeyRequired(provider) ? `${label} key not saved` : `${label} setup not ready`, message: reasonOrMessage };
}
