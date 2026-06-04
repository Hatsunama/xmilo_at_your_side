import { useLocalSearchParams, useRouter } from "expo-router";
import { useCallback, useEffect, useMemo, useState } from "react";
import { ActivityIndicator, Alert, Pressable, ScrollView, StyleSheet, Text, TextInput, View } from "react-native";

import {
  createMemoryControlClient,
  hasMemoryAction,
  isProtectedMemoryProjection,
  type MemoryEvidenceProjection,
  type VisibleMemoryProjection
} from "../../src/lib/memoryControl";

const memoryControl = createMemoryControlClient();

export default function MemoryDetailScreen() {
  const router = useRouter();
  const params = useLocalSearchParams<{ memoryId?: string }>();
  const memoryId = String(params.memoryId ?? "");
  const [memory, setMemory] = useState<VisibleMemoryProjection | null>(null);
  const [evidence, setEvidence] = useState<MemoryEvidenceProjection[]>([]);
  const [evidenceLoaded, setEvidenceLoaded] = useState(false);
  const [auditId, setAuditId] = useState("");
  const [loading, setLoading] = useState(true);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const [actionStatus, setActionStatus] = useState("");
  const [correctionOpen, setCorrectionOpen] = useState(false);
  const [correctionSummary, setCorrectionSummary] = useState("");

  const protectedTruth = useMemo(() => (memory ? isProtectedMemoryProjection(memory) : false), [memory]);

  const loadDetail = useCallback(async () => {
    if (!memoryId) {
      setError("Memory detail is unavailable.");
      setLoading(false);
      return;
    }
    try {
      const nextMemory = await memoryControl.getMemoryDetail(memoryId);
      setMemory(nextMemory);
      setError("");
    } catch (loadError: any) {
      setError(loadError?.message ?? "Memory detail is unavailable.");
    } finally {
      setLoading(false);
    }
  }, [memoryId]);

  useEffect(() => {
    void loadDetail();
  }, [loadDetail]);

  async function loadEvidence() {
    if (!memory || !hasMemoryAction(memory, "view_provenance")) return;
    setBusy(true);
    try {
      const result = await memoryControl.getMemoryProvenance(memory.memory_id);
      setEvidence(result.evidence ?? []);
      setAuditId(result.audit_id ?? "");
      setEvidenceLoaded(true);
      setActionStatus("");
    } catch (evidenceError: any) {
      setEvidence([]);
      setEvidenceLoaded(true);
      setActionStatus(evidenceError?.message ?? "Evidence is unavailable for this memory.");
    } finally {
      setBusy(false);
    }
  }

  function confirmAction(label: string, body: string, run: () => Promise<void>, destructive = false) {
    Alert.alert(label, body, [
      { text: "Cancel", style: "cancel" },
      {
        text: label,
        style: destructive ? "destructive" : "default",
        onPress: () => {
          void runMemoryAction(run);
        }
      }
    ]);
  }

  async function runMemoryAction(run: () => Promise<void>) {
    setBusy(true);
    try {
      await run();
      await loadDetail();
    } catch (actionError: any) {
      Alert.alert("Memory action failed", actionError?.message ?? "Could not update this memory.");
    } finally {
      setBusy(false);
    }
  }

  async function submitCorrection() {
    if (!memory) return;
    const clean = correctionSummary.trim();
    if (!clean) {
      Alert.alert("Correction needed", "Enter the corrected memory summary first.");
      return;
    }
    await runMemoryAction(async () => {
      const result = await memoryControl.correctMemory(memory.memory_id, {
        reason: "user correction from memory detail",
        summary: clean,
        content_excerpt: clean
      });
      if (!result.ok || !result.audit_id) throw new Error(result.message || "Correction was not confirmed by the local sidecar.");
      setActionStatus("Correction saved.");
      setCorrectionOpen(false);
      setCorrectionSummary("");
    });
  }

  if (loading) {
    return (
      <View style={styles.screen}>
        <View style={styles.centerCard}>
          <ActivityIndicator color="#93C5FD" />
          <Text style={styles.mutedText}>Loading memory detail...</Text>
        </View>
      </View>
    );
  }

  if (error || !memory) {
    return (
      <View style={styles.screen}>
        <View style={styles.content}>
          <Pressable style={styles.backButton} onPress={() => router.back()}>
            <Text style={styles.backButtonText}>‹ Back</Text>
          </Pressable>
          <View style={styles.errorCard}>
            <Text style={styles.errorText}>{error || "Memory detail is unavailable."}</Text>
          </View>
        </View>
      </View>
    );
  }

  return (
    <ScrollView style={styles.screen} contentContainerStyle={styles.content}>
      <View style={styles.headerRow}>
        <Pressable style={styles.backIconButton} onPress={() => router.back()} accessibilityRole="button" accessibilityLabel="Back">
          <Text style={styles.backIconText}>‹</Text>
        </Pressable>
        <Text style={styles.headerTitle}>Memory detail</Text>
      </View>

      <View style={styles.heroCard}>
        <View style={styles.heroTop}>
          <View style={[styles.iconBubble, protectedTruth && styles.iconBubbleProtected]}>
            <Text style={styles.iconText}>{protectedTruth ? "⌂" : "●"}</Text>
          </View>
          <View style={styles.heroTitleBlock}>
            <Text style={styles.title}>{memory.title || "Untitled memory"}</Text>
            <Text style={[styles.statusText, protectedTruth && styles.protectedText]}>{statusLabel(memory)}</Text>
          </View>
        </View>
        <Text style={styles.typeBadge}>{classLabel(memory.memory_class)}</Text>
        <InfoRow label="Type" value={classLabel(memory.memory_class)} />
        <InfoRow label="Source" value={sourceLabel(memory)} />
        <InfoRow label="Last updated" value="Not available" />
        <InfoRow label="Freshness" value={memory.freshness_state ? classLabel(memory.freshness_state) : "Unknown"} />
        <InfoRow label="Confidence" value={confidenceLabel(memory.confidence)} />
      </View>

      <View style={styles.sectionCard}>
        <Text style={styles.sectionTitle}>Evidence</Text>
        {hasMemoryAction(memory, "view_provenance") ? (
          evidenceLoaded ? (
            evidence.length ? (
              <View style={styles.evidenceList}>
                {evidence.map((item) => (
                  <View key={item.evidence_id} style={styles.evidenceItem}>
                    <Text style={styles.evidenceKind}>{classLabel(item.evidence_kind || "Evidence")}</Text>
                    <Text style={styles.mutedText}>{item.source_ref || `${item.source_type} / ${item.source_id}`}</Text>
                    {item.timestamp ? <Text style={styles.smallMuted}>{formatDate(item.timestamp)}</Text> : null}
                  </View>
                ))}
                {auditId ? <Text style={styles.smallMuted}>Evidence view recorded locally.</Text> : null}
              </View>
            ) : (
              <Text style={styles.mutedText}>No displayable evidence is available for this memory.</Text>
            )
          ) : (
            <Pressable style={styles.secondaryAction} onPress={loadEvidence} disabled={busy}>
              <Text style={styles.actionText}>{busy ? "Loading..." : "Load evidence"}</Text>
            </Pressable>
          )
        ) : (
          <Text style={styles.mutedText}>Evidence is not available for this memory.</Text>
        )}
      </View>

      <View style={styles.sectionCard}>
        <Text style={styles.sectionTitle}>Why Milo uses this</Text>
        <Text style={styles.mutedText}>{whyMiloUsesThis(memory)}</Text>
      </View>

      <View style={styles.sectionCard}>
        <Text style={styles.sectionTitle}>Safety boundaries</Text>
        <Text style={styles.mutedText}>You control user memory. Protected runtime, policy, provider, and canon truth can't be changed here.</Text>
      </View>

      {actionStatus ? (
        <View style={styles.statusCard}>
          <Text style={styles.statusCardText}>{actionStatus}</Text>
        </View>
      ) : null}

      <View style={styles.actionGrid}>
        {hasMemoryAction(memory, "correct_supersede") ? (
          <Pressable style={styles.primaryAction} onPress={() => setCorrectionOpen((current) => !current)} disabled={busy}>
            <Text style={styles.primaryActionText}>Correct</Text>
          </Pressable>
        ) : null}
        {hasMemoryAction(memory, "suppress") ? (
          <Pressable
            style={styles.secondaryAction}
            onPress={() =>
              confirmAction("Suppress", "Suppress this memory so it no longer drives retrieval?", async () => {
                const result = await memoryControl.suppressMemory(memory.memory_id, { reason: "user suppressed from memory detail" });
                if (!result.ok || !result.audit_id) throw new Error(result.message || "Suppress was not confirmed by the local sidecar.");
                setActionStatus("Memory suppressed.");
              })
            }
            disabled={busy}
          >
            <Text style={styles.actionText}>Suppress</Text>
          </Pressable>
        ) : null}
        {hasMemoryAction(memory, "restore_suppression") ? (
          <Pressable
            style={styles.secondaryAction}
            onPress={() =>
              confirmAction("Restore", "Restore this memory to active use if the sidecar allows it?", async () => {
                const result = await memoryControl.restoreMemory(memory.memory_id, { reason: "user restored from memory detail" });
                if (!result.ok || !result.audit_id) throw new Error(result.message || "Restore was not confirmed by the local sidecar.");
                setActionStatus("Memory restored.");
              })
            }
            disabled={busy}
          >
            <Text style={styles.actionText}>Restore</Text>
          </Pressable>
        ) : null}
        {hasMemoryAction(memory, "mark_stale") ? (
          <Pressable
            style={styles.secondaryAction}
            onPress={() =>
              confirmAction("Mark stale", "Mark this memory stale so it no longer acts as current truth?", async () => {
                const result = await memoryControl.markMemoryStale(memory.memory_id, { reason: "user marked stale from memory detail" });
                if (!result.ok || !result.audit_id) throw new Error(result.message || "Mark stale was not confirmed by the local sidecar.");
                setActionStatus("Memory marked stale.");
              })
            }
            disabled={busy}
          >
            <Text style={styles.actionText}>Mark stale</Text>
          </Pressable>
        ) : null}
      </View>

      {correctionOpen ? (
        <View style={styles.sectionCard}>
          <Text style={styles.sectionTitle}>Correct memory</Text>
          <TextInput
            multiline
            value={correctionSummary}
            onChangeText={setCorrectionSummary}
            placeholder="Enter the corrected summary..."
            placeholderTextColor="#94A3B8"
            style={styles.correctionInput}
            textAlignVertical="top"
          />
          <Pressable style={[styles.primaryAction, busy && styles.disabled]} onPress={submitCorrection} disabled={busy}>
            <Text style={styles.primaryActionText}>{busy ? "Saving..." : "Save correction"}</Text>
          </Pressable>
        </View>
      ) : null}

      {hasMemoryAction(memory, "delete_user_remove") ? (
        <View style={styles.dangerCard}>
          <Text style={styles.dangerTitle}>Remove from active memory</Text>
          <Text style={styles.mutedText}>This removes the item from Milo's active memory. It can be re-added later if needed.</Text>
          <Pressable
            style={styles.dangerButton}
            onPress={() =>
              confirmAction("Remove", "Remove this memory from active memory?", async () => {
                const result = await memoryControl.deleteMemory(memory.memory_id, {
                  reason: "user removed from memory detail",
                  confirmation: true
                });
                if (!result.ok || !result.audit_id) throw new Error(result.message || "Remove was not confirmed by the local sidecar.");
                setActionStatus("Memory removed from active memory.");
              }, true)
            }
            disabled={busy}
          >
            <Text style={styles.dangerButtonText}>Remove from active memory</Text>
          </Pressable>
        </View>
      ) : null}
    </ScrollView>
  );
}

function InfoRow({ label, value }: { label: string; value: string }) {
  return (
    <View style={styles.infoRow}>
      <Text style={styles.infoLabel}>{label}</Text>
      <Text style={styles.infoValue}>{value}</Text>
    </View>
  );
}

function statusLabel(memory: VisibleMemoryProjection) {
  if (isProtectedMemoryProjection(memory)) return "Protected";
  if (memory.status === "active") return "Active";
  if (memory.suppression_status === "suppressed") return "Suppressed";
  return classLabel(memory.status);
}

function sourceLabel(memory: VisibleMemoryProjection) {
  if (memory.source_type === "direct_user") return "Directly told Milo";
  if (memory.source_type === "verified_runtime") return "Verified runtime";
  if (memory.source_type === "main_hub") return "Main Hub";
  return classLabel(memory.source_type || "Unknown");
}

function confidenceLabel(confidence?: number) {
  if (typeof confidence !== "number") return "Unknown";
  if (confidence >= 0.85) return "High";
  if (confidence >= 0.55) return "Medium";
  return "Low";
}

function whyMiloUsesThis(memory: VisibleMemoryProjection) {
  if (memory.retrieval_eligible === false) return memory.retrieval_reason || "This memory is not currently used for retrieval.";
  if (memory.summary) return `Helps Milo use this context: ${memory.summary}`;
  return "Helps Milo keep useful context available when the local sidecar says it is eligible.";
}

function classLabel(value: string) {
  return value
    .split("_")
    .filter(Boolean)
    .map((part) => part.charAt(0).toUpperCase() + part.slice(1))
    .join(" ");
}

function formatDate(value: string) {
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return value;
  return date.toLocaleString();
}

const styles = StyleSheet.create({
  screen: { flex: 1, backgroundColor: "#0B1020" },
  content: { padding: 16, gap: 14 },
  headerRow: { flexDirection: "row", alignItems: "center", gap: 10 },
  backIconButton: { width: 36, height: 36, borderRadius: 18, alignItems: "center", justifyContent: "center" },
  backIconText: { color: "#E5E7EB", fontSize: 30, lineHeight: 32 },
  headerTitle: { color: "#F8FAFC", fontSize: 22, fontWeight: "800" },
  heroCard: { backgroundColor: "#111827", borderRadius: 16, padding: 16, borderWidth: 1, borderColor: "#263653", gap: 10 },
  heroTop: { flexDirection: "row", gap: 12, alignItems: "center" },
  iconBubble: { width: 50, height: 50, borderRadius: 25, backgroundColor: "#312E81", alignItems: "center", justifyContent: "center" },
  iconBubbleProtected: { backgroundColor: "#15412A" },
  iconText: { color: "#EDE9FE", fontWeight: "900", fontSize: 18 },
  heroTitleBlock: { flex: 1, minWidth: 0 },
  title: { color: "#F8FAFC", fontSize: 18, fontWeight: "900" },
  statusText: { color: "#4ADE80", fontWeight: "800", marginTop: 4 },
  protectedText: { color: "#86EFAC" },
  typeBadge: { alignSelf: "flex-start", color: "#C4B5FD", backgroundColor: "#312E81", borderRadius: 7, paddingHorizontal: 8, paddingVertical: 4, fontSize: 12, fontWeight: "800" },
  infoRow: { flexDirection: "row", justifyContent: "space-between", alignItems: "center", borderTopWidth: 1, borderTopColor: "#1F2937", paddingTop: 10, gap: 12 },
  infoLabel: { color: "#94A3B8", fontWeight: "700", flex: 1 },
  infoValue: { color: "#E5E7EB", fontWeight: "700", textAlign: "right", flex: 1.4 },
  sectionCard: { backgroundColor: "#111827", borderRadius: 16, padding: 14, borderWidth: 1, borderColor: "#1F2937", gap: 10 },
  sectionTitle: { color: "#C4B5FD", fontSize: 15, fontWeight: "900" },
  mutedText: { color: "#CBD5E1", lineHeight: 20 },
  smallMuted: { color: "#94A3B8", fontSize: 12, lineHeight: 18 },
  evidenceList: { gap: 10 },
  evidenceItem: { borderRadius: 12, borderWidth: 1, borderColor: "#263653", backgroundColor: "#0F172A", padding: 12 },
  evidenceKind: { color: "#E5E7EB", fontWeight: "800", marginBottom: 4 },
  actionGrid: { flexDirection: "row", flexWrap: "wrap", gap: 10 },
  primaryAction: { flexGrow: 1, minWidth: 112, backgroundColor: "#2563EB", borderRadius: 14, paddingVertical: 13, alignItems: "center" },
  primaryActionText: { color: "#FFFFFF", fontWeight: "900" },
  secondaryAction: { flexGrow: 1, minWidth: 112, backgroundColor: "#312E81", borderRadius: 14, paddingVertical: 13, alignItems: "center" },
  actionText: { color: "#EDE9FE", fontWeight: "900" },
  correctionInput: {
    minHeight: 96,
    borderRadius: 14,
    borderWidth: 1,
    borderColor: "#334155",
    backgroundColor: "#0F172A",
    color: "#F8FAFC",
    padding: 12
  },
  dangerCard: { backgroundColor: "#111827", borderRadius: 16, padding: 14, borderWidth: 1, borderColor: "#7F1D1D", gap: 10 },
  dangerTitle: { color: "#F87171", fontWeight: "900", fontSize: 15 },
  dangerButton: { borderRadius: 14, borderWidth: 1, borderColor: "#7F1D1D", paddingVertical: 13, alignItems: "center", backgroundColor: "#2A1218" },
  dangerButtonText: { color: "#FCA5A5", fontWeight: "900" },
  statusCard: { backgroundColor: "#102345", borderRadius: 14, padding: 12, borderWidth: 1, borderColor: "#2563EB" },
  statusCardText: { color: "#BFDBFE", fontWeight: "800" },
  centerCard: { margin: 16, backgroundColor: "#111827", borderRadius: 16, padding: 18, borderWidth: 1, borderColor: "#1F2937", gap: 8 },
  errorCard: { backgroundColor: "#2A1218", borderRadius: 16, padding: 14, borderWidth: 1, borderColor: "#7F1D1D" },
  errorText: { color: "#FCA5A5", lineHeight: 20 },
  backButton: { alignSelf: "flex-start", paddingVertical: 8 },
  backButtonText: { color: "#93C5FD", fontWeight: "800" },
  disabled: { opacity: 0.55 }
});
