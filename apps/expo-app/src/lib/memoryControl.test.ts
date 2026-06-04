jest.mock("./config", () => ({
  SIDECAR_BASE_URL: "http://127.0.0.1:42817"
}));

jest.mock("./xmiloRuntimeHost", () => ({
  resolveLocalhostBearerToken: jest.fn(async () => "test-token")
}));

import {
  createMemoryControlClient,
  hasMemoryAction,
  isProtectedMemoryProjection,
  isSuppressedMemoryProjection,
  memoryMatchesFilter,
  memoryMatchesSearch,
  visibleListActions,
  type VisibleMemoryProjection
} from "./memoryControl";

function memory(overrides: Partial<VisibleMemoryProjection> = {}): VisibleMemoryProjection {
  return {
    memory_id: "memory.test",
    memory_class: "durable_user_preference",
    status: "active",
    category: "preference",
    title: "Prefers concise answers",
    summary: "You prefer concise answers.",
    content_excerpt: "Concise answers are preferred.",
    source_type: "direct_user",
    source_id: "conversation",
    freshness_state: "fresh",
    confidence: 0.9,
    contradiction_state: "none",
    quarantine_status: "clean",
    suppression_status: "active",
    retrieval_eligible: true,
    retrieval_reason: "active",
    rollback_available: false,
    user_visible: true,
    warnings: [],
    allowed_actions: ["view", "view_provenance", "correct_supersede", "suppress", "mark_stale", "delete_user_remove"],
    ...overrides
  };
}

describe("memoryControl", () => {
  it("calls local sidecar memory routes and not relay routes", async () => {
    const calls: Array<{ path: string; init?: RequestInit }> = [];
    const client = createMemoryControlClient(async (path, init) => {
      calls.push({ path, init });
      if (path === "/memory") return { ok: true, memories: [memory()] } as any;
      if (path === "/memory/candidates") return { ok: true, candidates: [] } as any;
      if (path === "/memory/memory.test") return { ok: true, memory: memory() } as any;
      if (path === "/memory/memory.test/provenance") return { ok: true, evidence: [], audit_id: "audit.test" } as any;
      return { ok: true, action: "suppress", status: "suppressed", audit_id: "audit.test" } as any;
    });

    await client.listMemory();
    await client.listMemoryCandidates();
    await client.getMemoryDetail("memory.test");
    await client.getMemoryProvenance("memory.test");
    await client.suppressMemory("memory.test");

    expect(calls.map((call) => call.path)).toEqual([
      "/memory",
      "/memory/candidates",
      "/memory/memory.test",
      "/memory/memory.test/provenance",
      "/memory/memory.test/suppress"
    ]);
    expect(calls.every((call) => call.path.startsWith("/memory"))).toBe(true);
  });

  it("renders actions only from allowed_actions and excludes approval or rollback", () => {
    const item = memory({ allowed_actions: ["view", "correct_supersede", "suppress", "approve_candidate", "rollback"] });

    expect(hasMemoryAction(item, "correct_supersede")).toBe(true);
    expect(visibleListActions(item)).toEqual(["correct_supersede", "suppress", "view"]);
    expect(visibleListActions(item)).not.toContain("reject_candidate");
    expect(visibleListActions(item as any)).not.toContain("approve_candidate");
    expect(visibleListActions(item as any)).not.toContain("rollback");
  });

  it("requires confirmation payload for delete/remove helper", async () => {
    const calls: Array<{ path: string; init?: RequestInit }> = [];
    const client = createMemoryControlClient(async (path, init) => {
      calls.push({ path, init });
      return { ok: true, action: "delete_user_remove", status: "deleted_by_user", audit_id: "audit.delete" } as any;
    });

    await client.deleteMemory("memory.test", { reason: "remove" });
    await client.deleteMemory("memory.test", { reason: "remove", confirmation: true });

    expect(JSON.parse(String(calls[0].init?.body))).toMatchObject({ reason: "remove", confirmation: false });
    expect(JSON.parse(String(calls[1].init?.body))).toMatchObject({ reason: "remove", confirmation: true });
  });

  it("identifies protected memory and avoids mutating buttons without allowed actions", () => {
    const protectedItem = memory({
      memory_class: "canon_memory",
      source_type: "canon",
      allowed_actions: ["view", "view_provenance"]
    });

    expect(isProtectedMemoryProjection(protectedItem)).toBe(true);
    expect(visibleListActions(protectedItem)).toEqual(["view"]);
    expect(hasMemoryAction(protectedItem, "suppress")).toBe(false);
    expect(hasMemoryAction(protectedItem, "correct_supersede")).toBe(false);
  });

  it("separates active, important, suppressed, and review filter behavior", () => {
    const active = memory({ confidence: 0.7 });
    const important = memory({ memory_id: "memory.important", confidence: 0.95 });
    const suppressed = memory({
      memory_id: "memory.suppressed",
      status: "suppressed",
      suppression_status: "suppressed",
      retrieval_eligible: false
    });

    expect(memoryMatchesFilter(active, "all")).toBe(true);
    expect(memoryMatchesFilter(important, "important")).toBe(true);
    expect(memoryMatchesFilter(suppressed, "all")).toBe(false);
    expect(memoryMatchesFilter(suppressed, "suppressed")).toBe(true);
    expect(memoryMatchesFilter(active, "review")).toBe(false);
    expect(isSuppressedMemoryProjection(suppressed)).toBe(true);
  });

  it("searches safe projection fields only", () => {
    expect(memoryMatchesSearch(memory(), "concise")).toBe(true);
    expect(memoryMatchesSearch(memory(), "durable_user_preference")).toBe(true);
    expect(memoryMatchesSearch(memory(), "unrelated")).toBe(false);
  });
});
