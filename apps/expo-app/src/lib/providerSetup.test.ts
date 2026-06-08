import {
  OLLAMA_CONNECTION_MESSAGE,
  OLLAMA_MANUAL_URL_INVALID_MESSAGE,
  allowsAdvancedManualServer,
  buildProviderDisplayRows,
  findProviderOptionById,
  getProviderSaveFailureCopy,
  getProviderSetupCopy,
  getProviderSetupMode,
  getOllamaProviderChoices,
  isGroupedOllamaProviderRow,
  isGuidedLocalServerProvider,
  usesCanonicalProviderEndpoint
} from "./providerSetup";
import type { LocalProviderOption } from "./bridge";

const currentOllama = {
  id: "ollama",
  label: "Ollama Local",
  default_model: "llama3.2",
  default_base_url: "",
  key_required: false,
  custom_base_url_allowed: true,
  base_url_required: true,
  allowed_schemes: ["http", "https"],
  connection_kind: "local_server",
  base_url_entry_mode: "guided_local_server",
  advanced_manual_base_url_allowed: true,
  requires_connection_target: true,
  requires_reachability_probe: false,
  setup_guidance_code: "ollama_connection_target_required"
} satisfies LocalProviderOption;

const legacyOllama = {
  id: "ollama",
  label: "Ollama",
  default_model: "llama3.2",
  default_base_url: "",
  key_required: false,
  custom_base_url_allowed: true,
  base_url_required: true,
  allowed_schemes: ["http", "https"]
} as Partial<LocalProviderOption>;

const ollamaCloud = {
  id: "ollama_cloud",
  label: "Ollama Cloud",
  default_model: "gpt-oss:120b",
  default_base_url: "https://ollama.com",
  key_required: true,
  custom_base_url_allowed: false,
  base_url_required: true,
  allowed_schemes: ["https"],
  connection_kind: "cloud_canonical",
  base_url_entry_mode: "hidden_canonical",
  advanced_manual_base_url_allowed: false,
  requires_connection_target: false,
  requires_reachability_probe: false
} satisfies LocalProviderOption;

const openAi = {
  id: "openai",
  label: "OpenAI / GPT",
  default_model: "gpt-5.4",
  default_base_url: "https://api.openai.com/v1",
  key_required: true,
  custom_base_url_allowed: false,
  base_url_required: true,
  allowed_schemes: ["https"],
  connection_kind: "cloud_canonical",
  base_url_entry_mode: "hidden_canonical",
  advanced_manual_base_url_allowed: false,
  requires_connection_target: false,
  requires_reachability_probe: false
} satisfies LocalProviderOption;

describe("provider setup normalizer", () => {
  it("groups Ollama Local and Ollama Cloud into one visible Ollama row", () => {
    const rows = buildProviderDisplayRows([openAi, currentOllama, ollamaCloud]);
    expect(rows.map((row) => row.label)).toEqual(["OpenAI / GPT", "Ollama"]);
    const ollamaRow = rows.find(isGroupedOllamaProviderRow);
    expect(ollamaRow).toEqual({
      kind: "grouped_ollama",
      id: "ollama_group",
      label: "Ollama",
      localId: "ollama",
      cloudId: "ollama_cloud"
    });
  });

  it("resolves grouped Ollama choices to exact sidecar provider ids", () => {
    const choices = getOllamaProviderChoices([openAi, currentOllama, ollamaCloud]);
    expect(choices.cloud?.id).toBe("ollama_cloud");
    expect(choices.local?.id).toBe("ollama");
    expect(findProviderOptionById([openAi, currentOllama, ollamaCloud], "ollama_cloud")?.label).toBe("Ollama Cloud");
    expect(findProviderOptionById([openAi, currentOllama, ollamaCloud], "ollama")?.label).toBe("Ollama Local");
  });

  it("does not create an ambiguous Ollama group when one variant is missing", () => {
    expect(buildProviderDisplayRows([currentOllama]).map((row) => row.label)).toEqual(["Ollama Local"]);
    expect(buildProviderDisplayRows([ollamaCloud]).map((row) => row.label)).toEqual(["Ollama Cloud"]);
  });

  it("does not group unknown providers as Ollama", () => {
    const unknown = {
      ...openAi,
      id: "ollama-compatible",
      label: "Ollama Compatible"
    };
    const rows = buildProviderDisplayRows([unknown]);
    expect(rows).toEqual([{ kind: "provider", id: "ollama-compatible", label: "Ollama Compatible", option: unknown }]);
  });

  it("uses current M20 Ollama fields for guided local-server setup", () => {
    expect(getProviderSetupMode(currentOllama)).toBe("guided_local_server");
    expect(isGuidedLocalServerProvider(currentOllama)).toBe(true);
    expect(allowsAdvancedManualServer(currentOllama)).toBe(true);
    expect(getProviderSetupCopy(currentOllama).title).toBe("Connect to Ollama Local");
    expect(getProviderSetupCopy(currentOllama).body).toBe(OLLAMA_CONNECTION_MESSAGE);
  });

  it("keeps legacy Ollama payloads out of generic API-key setup", () => {
    expect(getProviderSetupMode(legacyOllama)).toBe("guided_local_server");
    expect(isGuidedLocalServerProvider(legacyOllama)).toBe(true);
    expect(allowsAdvancedManualServer(legacyOllama)).toBe(true);
    expect(getProviderSetupCopy(legacyOllama).title).toBe("Connect to Ollama Local");
    expect(getProviderSetupCopy(legacyOllama).primary).toBe("Set up Ollama Local connection");
  });

  it("keeps Ollama Cloud on canonical API-key setup without local-server treatment", () => {
    expect(getProviderSetupMode(ollamaCloud)).toBe("cloud_canonical");
    expect(isGuidedLocalServerProvider(ollamaCloud)).toBe(false);
    expect(usesCanonicalProviderEndpoint(ollamaCloud)).toBe(true);
    expect(allowsAdvancedManualServer(ollamaCloud)).toBe(false);
    expect(getProviderSetupCopy(ollamaCloud).title).toBe("Enter your API key");
    expect(getProviderSetupCopy(ollamaCloud).body).toContain("xMilo will use the standard Ollama Cloud endpoint automatically.");
    expect(getProviderSetupCopy(ollamaCloud).body).toContain("Paste your Ollama API key to continue.");
  });

  it("keeps cloud providers on API-key setup without Ollama treatment", () => {
    expect(getProviderSetupMode(openAi)).toBe("cloud_canonical");
    expect(isGuidedLocalServerProvider(openAi)).toBe(false);
    expect(usesCanonicalProviderEndpoint(openAi)).toBe(true);
    expect(allowsAdvancedManualServer(openAi)).toBe(false);
    expect(getProviderSetupCopy(openAi).title).toBe("Enter your API key");
    expect(getProviderSetupCopy(openAi).body).toContain("xMilo will use the standard OpenAI / GPT endpoint automatically.");
    expect(getProviderSetupCopy(openAi).body).toContain("Paste your API key to continue.");
  });

  it("does not give unknown missing-field providers Ollama compatibility", () => {
    const unknown = { id: "local-other", label: "Local Other", key_required: false } as Partial<LocalProviderOption>;
    expect(getProviderSetupMode(unknown)).toBe("sidecar_resolved");
    expect(isGuidedLocalServerProvider(unknown)).toBe(false);
    expect(allowsAdvancedManualServer(unknown)).toBe(false);
    expect(getProviderSetupCopy(unknown).title).toBe("Enter your API key");
  });

  it("returns commercial Ollama failure title and message pairs", () => {
    expect(getProviderSaveFailureCopy("local_provider_connection_target_required", currentOllama)).toEqual({
      title: "Ollama Local connection needed",
      message: OLLAMA_CONNECTION_MESSAGE
    });
    expect(getProviderSaveFailureCopy("local_provider_manual_url_invalid", currentOllama)).toEqual({
      title: "Ollama Local server address is not valid",
      message: OLLAMA_MANUAL_URL_INVALID_MESSAGE
    });
    expect(getProviderSaveFailureCopy("missing_key", ollamaCloud)).toEqual({
      title: "Ollama Cloud key not saved",
      message: "Ollama Cloud needs an API key before it can handle new tasks."
    });
  });
});
