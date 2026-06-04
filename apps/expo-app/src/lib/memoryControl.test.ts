import { readFileSync } from "fs";
import { join } from "path";

jest.mock("./config", () => ({
  SIDECAR_BASE_URL: "http://127.0.0.1:42817"
}));

jest.mock("./xmiloRuntimeHost", () => ({
  resolveLocalhostBearerToken: jest.fn(async () => "test-token")
}));

import {
  createMemoryControlClient,
  candidateMatchesSearch,
  hasMemoryAction,
  isProtectedMemoryProjection,
  isSuppressedMemoryProjection,
  memoryMutationConfirmed,
  memoryMatchesFilter,
  memoryMatchesSearch,
  visibleListActions,
  type MemoryCandidateProjection,
  type VisibleMemoryProjection
} from "./memoryControl";

const originalFetch = globalThis.fetch;

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

function candidate(overrides: Partial<MemoryCandidateProjection> = {}): MemoryCandidateProjection {
  return {
    candidate_id: "candidate.test",
    candidate_type: "memory_candidate",
    status: "needs_review",
    title: "Possible preference",
    summary: "You may prefer quiet summaries.",
    source_type: "model_output",
    source_id: "turn.test",
    trust_tier: 6,
    authority_rank: "rank_700_model_output",
    freshness_state: "fresh",
    confidence: 0.62,
    contradiction_state: "none",
    quarantine_status: "clean",
    suppression_status: "active",
    warnings: ["candidate approval and promotion are deferred in Phase 18E2A-V1"],
    allowed_actions: ["reject_candidate"],
    ...overrides
  };
}

function appSource(fileName: string) {
  return readFileSync(join(__dirname, "../../app/memory", fileName), "utf8");
}

afterEach(() => {
  jest.restoreAllMocks();
  (globalThis as any).fetch = originalFetch;
});

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

  it("consumes representative Phase 18E2A response shapes without last_updated", async () => {
    const projection = memory();
    const reviewCandidate = candidate();
    const client = createMemoryControlClient(async (path) => {
      if (path === "/memory") return { ok: true, memories: [projection] } as any;
      if (path === "/memory/memory.test") return { ok: true, memory: projection } as any;
      if (path === "/memory/memory.test/provenance") {
        return {
          ok: true,
          memory_id: "memory.test",
          evidence: [
            {
              evidence_id: "evidence.test",
              source_type: "conversation",
              source_id: "turn.test",
              source_ref: "Directly told Milo",
              evidence_kind: "user_statement",
              trust_tier: 2,
              authority_rank: "rank_300_direct_user",
              timestamp: "2026-06-04T10:00:00Z",
              content_hash: "hash.test",
              redaction_status: "safe_projection",
              display_allowed: true,
              promotion_allowed: false
            }
          ],
          audit_id: "audit.provenance"
        } as any;
      }
      if (path === "/memory/candidates") return { ok: true, candidates: [reviewCandidate] } as any;
      return { ok: true, action: "mark_stale", status: "stale", audit_id: "audit.action" } as any;
    });

    await expect(client.listMemory()).resolves.toEqual([projection]);
    await expect(client.getMemoryDetail("memory.test")).resolves.toEqual(projection);
    await expect(client.getMemoryProvenance("memory.test")).resolves.toMatchObject({
      audit_id: "audit.provenance",
      evidence: [{ display_allowed: true, promotion_allowed: false }]
    });
    await expect(client.listMemoryCandidates()).resolves.toEqual([reviewCandidate]);
    await expect(client.markMemoryStale("memory.test")).resolves.toMatchObject({ ok: true, audit_id: "audit.action" });
    expect(projection).not.toHaveProperty("last_updated");
  });

  it("renders actions only from allowed_actions and excludes approval or rollback", () => {
    const item = memory({ allowed_actions: ["view", "correct_supersede", "suppress", "approve_candidate", "rollback"] });

    expect(hasMemoryAction(item, "correct_supersede")).toBe(true);
    expect(visibleListActions(item)).toEqual(["correct_supersede", "suppress", "view"]);
    expect(visibleListActions(item)).not.toContain("reject_candidate");
    expect(visibleListActions(item as any)).not.toContain("approve_candidate");
    expect(visibleListActions(item as any)).not.toContain("rollback");
  });

  it("shows candidate rejection only when allowed and never exposes promotion actions", () => {
    const rejectable = candidate();
    const inert = candidate({ allowed_actions: [] });

    expect(hasMemoryAction(rejectable, "reject_candidate")).toBe(true);
    expect(hasMemoryAction(inert, "reject_candidate")).toBe(false);
    expect(candidateMatchesSearch(rejectable, "quiet summaries")).toBe(true);
    expect(rejectable.allowed_actions).not.toContain("approve_candidate");
    expect(rejectable.allowed_actions).not.toContain("promote_candidate");
    expect(rejectable.allowed_actions).not.toContain("rollback");
  });

  it("requires ok true and audit_id before a mutation is confirmed", () => {
    expect(memoryMutationConfirmed({ ok: true, action: "suppress", status: "suppressed", audit_id: "audit.ok" })).toBe(true);
    expect(memoryMutationConfirmed({ ok: true, action: "suppress", status: "suppressed" })).toBe(false);
    expect(memoryMutationConfirmed({ ok: false, action: "suppress", status: "active", audit_id: "audit.no" })).toBe(false);
    expect(memoryMutationConfirmed({ ok: true, action: "suppress", status: "suppressed", audit_id: "   " })).toBe(false);
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

  it("maps stable sidecar memory errors to safe user messages", async () => {
    (globalThis as any).fetch = jest.fn(async () => ({
      ok: false,
      status: 403,
      text: async () => JSON.stringify({ ok: false, error_code: "memory_runtime_truth_cannot_modify" })
    }));
    const client = createMemoryControlClient();

    await expect(client.suppressMemory("runtime.memory")).rejects.toMatchObject({
      message: "That memory can't be changed here.",
      errorCode: "memory_runtime_truth_cannot_modify",
      path: "/memory/runtime.memory/suppress",
      status: 403
    });
  });

  it("keeps secret-like transport errors out of user-facing messages", async () => {
    (globalThis as any).fetch = jest.fn(async () => ({
      ok: false,
      status: 500,
      text: async () => JSON.stringify({ ok: false, reason: "authorization: bearer abcdefghijklmnopqrstuvwxyz1234567890 failed" })
    }));
    const client = createMemoryControlClient();

    await expect(client.listMemory()).rejects.toThrow("authorization: bearer <redacted> failed");
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

  it("keeps Memory UI source on local filters, safe missing date copy, and guarded actions", () => {
    const listSource = appSource("index.tsx");
    const detailSource = appSource("[memoryId].tsx");

    expect(listSource).toContain('router.push({ pathname: "/memory/[memoryId]"');
    expect(listSource).toContain('filter === "review"');
    expect(listSource).toContain('hasMemoryAction(candidate, "reject_candidate")');
    expect(listSource).toContain("memoryMutationConfirmed(result)");
    expect(listSource).not.toContain("approve_candidate");
    expect(listSource).not.toContain("promote_candidate");
    expect(detailSource).toContain('InfoRow label="Last updated" value="Not available"');
    expect(detailSource).toContain("Protected runtime, policy, provider, and canon truth can't be changed here.");
    expect(detailSource).toContain("confirmation: true");
    expect(detailSource).toContain("memoryMutationConfirmed(result)");
    expect(detailSource).not.toContain("approve_candidate");
    expect(detailSource).not.toContain("rollback");
  });
});
