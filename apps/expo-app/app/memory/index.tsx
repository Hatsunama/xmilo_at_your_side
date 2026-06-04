import { useFocusEffect, useRouter } from "expo-router";
import { useCallback, useMemo, useState } from "react";
import { ActivityIndicator, Alert, Pressable, ScrollView, StyleSheet, Text, TextInput, View } from "react-native";

import {
  candidateMatchesSearch,
  createMemoryControlClient,
  hasMemoryAction,
  isProtectedMemoryProjection,
  memoryMatchesFilter,
  memoryMatchesSearch,
  type MemoryCandidateProjection,
  type MemoryFilter,
  type VisibleMemoryProjection,
  visibleListActions
} from "../../src/lib/memoryControl";

const FILTERS: Array<{ id: MemoryFilter; label: string }> = [
  { id: "all", label: "All" },
  { id: "important", label: "Important" },
  { id: "suppressed", label: "Suppressed" },
  { id: "review", label: "Review" }
];

const memoryControl = createMemoryControlClient();

export default function MemoryScreen() {
  const router = useRouter();
  const [memories, setMemories] = useState<VisibleMemoryProjection[]>([]);
  const [candidates, setCandidates] = useState<MemoryCandidateProjection[]>([]);
  const [filter, setFilter] = useState<MemoryFilter>("all");
  const [query, setQuery] = useState("");
  const [loading, setLoading] = useState(true);
  const [refreshing, setRefreshing] = useState(false);
  const [error, setError] = useState("");

  const loadMemory = useCallback(async () => {
    setRefreshing(true);
    try {
      const [nextMemories, nextCandidates] = await Promise.all([
        memoryControl.listMemory(),
        memoryControl.listMemoryCandidates().catch(() => [])
      ]);
      setMemories(nextMemories);
      setCandidates(nextCandidates);
      setError("");
    } catch (loadError: any) {
      setError(loadError?.message ?? "Memory controls are unavailable until the local sidecar is connected.");
    } finally {
      setLoading(false);
      setRefreshing(false);
    }
  }, []);

  useFocusEffect(
    useCallback(() => {
      void loadMemory();
    }, [loadMemory])
  );

  const visibleMemories = useMemo(
    () => memories.filter((memory) => memoryMatchesFilter(memory, filter)).filter((memory) => memoryMatchesSearch(memory, query)),
    [filter, memories, query]
  );
  const visibleCandidates = useMemo(
    () => candidates.filter((candidate) => candidateMatchesSearch(candidate, query)),
    [candidates, query]
  );

  function openDetail(memory: VisibleMemoryProjection) {
    router.push({ pathname: "/memory/[memoryId]", params: { memoryId: memory.memory_id } });
  }

  async function runQuickAction(memory: VisibleMemoryProjection, action: "suppress" | "restore_suppression") {
    const actionLabel = action === "suppress" ? "Suppress" : "Restore";
    Alert.alert(actionLabel, `${actionLabel} "${memory.title}"?`, [
      { text: "Cancel", style: "cancel" },
      {
        text: actionLabel,
        onPress: () => {
          void (async () => {
            try {
              if (action === "suppress") {
                await memoryControl.suppressMemory(memory.memory_id, { reason: "user action from memory list" });
              } else {
                await memoryControl.restoreMemory(memory.memory_id, { reason: "user action from memory list" });
              }
              await loadMemory();
            } catch (actionError: any) {
              Alert.alert("Memory action failed", actionError?.message ?? "Could not update this memory.");
            }
          })();
        }
      }
    ]);
  }

  async function rejectCandidate(candidate: MemoryCandidateProjection) {
    Alert.alert("Reject candidate", `Reject "${candidate.title}"?`, [
      { text: "Cancel", style: "cancel" },
      {
        text: "Reject",
        style: "destructive",
        onPress: () => {
          void (async () => {
            try {
              await memoryControl.rejectMemoryCandidate(candidate.candidate_id, { reason: "user rejected candidate from review" });
              await loadMemory();
            } catch (actionError: any) {
              Alert.alert("Candidate action failed", actionError?.message ?? "Could not reject this candidate.");
            }
          })();
        }
      }
    ]);
  }

  return (
    <View style={styles.screen}>
      <ScrollView style={styles.scroll} contentContainerStyle={styles.content}>
        <View style={styles.headerRow}>
          <Pressable style={styles.backButton} onPress={() => router.back()} accessibilityRole="button" accessibilityLabel="Back">
            <Text style={styles.backButtonText}>‹</Text>
          </Pressable>
          <View style={styles.headerText}>
            <Text style={styles.title}>Memory</Text>
            <Text style={styles.subtitle}>What Milo remembers and what you control.</Text>
          </View>
        </View>

        <View style={styles.searchRow}>
          <TextInput
            value={query}
            onChangeText={setQuery}
            placeholder="Search memory..."
            placeholderTextColor="#94A3B8"
            style={styles.searchInput}
            autoCapitalize="none"
          />
          <Pressable style={styles.refreshButton} onPress={loadMemory} disabled={refreshing}>
            {refreshing ? <ActivityIndicator color="#93C5FD" /> : <Text style={styles.refreshButtonText}>↻</Text>}
          </Pressable>
        </View>

        <ScrollView horizontal showsHorizontalScrollIndicator={false} contentContainerStyle={styles.filterRow}>
          {FILTERS.map((item) => (
            <Pressable key={item.id} style={[styles.filterChip, filter === item.id && styles.filterChipActive]} onPress={() => setFilter(item.id)}>
              <Text style={[styles.filterText, filter === item.id && styles.filterTextActive]}>{item.label}</Text>
            </Pressable>
          ))}
        </ScrollView>

        {error ? (
          <View style={styles.errorCard}>
            <Text style={styles.errorText}>{error}</Text>
          </View>
        ) : null}

        {loading ? (
          <View style={styles.centerCard}>
            <ActivityIndicator color="#93C5FD" />
            <Text style={styles.mutedText}>Loading memory...</Text>
          </View>
        ) : filter === "review" ? (
          visibleCandidates.length ? (
            <View style={styles.list}>
              {visibleCandidates.map((candidate) => (
                <CandidateCard key={candidate.candidate_id} candidate={candidate} onReject={rejectCandidate} />
              ))}
            </View>
          ) : (
            <EmptyState title="No candidate memories to review." body="Review items will appear here when the sidecar exposes safe candidate projections." />
          )
        ) : visibleMemories.length ? (
          <View style={styles.list}>
            {visibleMemories.map((memory) => (
              <MemoryCard key={memory.memory_id} memory={memory} onDetails={openDetail} onAction={runQuickAction} />
            ))}
          </View>
        ) : filter === "suppressed" ? (
          <EmptyState title="No suppressed memory." body="Suppressed or inactive memory will appear here when returned by the local sidecar." />
        ) : (
          <EmptyState title="No memory saved yet." body="As Milo learns preferences, task continuity, and approved summaries, they'll appear here." />
        )}

        <View style={styles.footerNote}>
          <Text style={styles.footerNoteText}>You control what stays in active memory. Review candidate memories from the Review tab.</Text>
        </View>
      </ScrollView>
    </View>
  );
}

function MemoryCard({
  memory,
  onDetails,
  onAction
}: {
  memory: VisibleMemoryProjection;
  onDetails: (memory: VisibleMemoryProjection) => void;
  onAction: (memory: VisibleMemoryProjection, action: "suppress" | "restore_suppression") => void;
}) {
  const actions = visibleListActions(memory);
  const protectedTruth = isProtectedMemoryProjection(memory);
  const body = memory.summary || memory.content_excerpt || "No summary provided.";

  return (
    <View style={styles.card}>
      <View style={styles.cardTop}>
        <View style={[styles.iconBubble, protectedTruth && styles.iconBubbleProtected]}>
          <Text style={styles.iconText}>{categoryIcon(memory)}</Text>
        </View>
        <View style={styles.cardMain}>
          <View style={styles.cardTitleRow}>
            <Text style={styles.cardTitle} numberOfLines={2}>{memory.title || "Untitled memory"}</Text>
            <Text style={[styles.statusText, protectedTruth && styles.protectedText]}>{statusLabel(memory)}</Text>
          </View>
          <Text style={styles.cardBody} numberOfLines={2}>{body}</Text>
          <View style={styles.badgeRow}>
            <Text style={styles.typeBadge}>{classLabel(memory.memory_class)}</Text>
            {memory.freshness_state ? <Text style={styles.subtleBadge}>{memory.freshness_state}</Text> : null}
          </View>
        </View>
      </View>
      <View style={styles.actionRow}>
        {actions.includes("correct_supersede") ? (
          <Pressable style={styles.secondaryAction} onPress={() => onDetails(memory)}>
            <Text style={styles.actionText}>Correct</Text>
          </Pressable>
        ) : null}
        {actions.includes("suppress") ? (
          <Pressable style={styles.secondaryAction} onPress={() => onAction(memory, "suppress")}>
            <Text style={styles.actionText}>Suppress</Text>
          </Pressable>
        ) : null}
        {actions.includes("restore_suppression") ? (
          <Pressable style={styles.secondaryAction} onPress={() => onAction(memory, "restore_suppression")}>
            <Text style={styles.actionText}>Restore</Text>
          </Pressable>
        ) : null}
        <Pressable style={styles.secondaryAction} onPress={() => onDetails(memory)}>
          <Text style={styles.actionText}>Details</Text>
        </Pressable>
      </View>
    </View>
  );
}

function CandidateCard({ candidate, onReject }: { candidate: MemoryCandidateProjection; onReject: (candidate: MemoryCandidateProjection) => void }) {
  return (
    <View style={styles.card}>
      <View style={styles.cardTop}>
        <View style={[styles.iconBubble, styles.iconBubbleReview]}>
          <Text style={styles.iconText}>?</Text>
        </View>
        <View style={styles.cardMain}>
          <View style={styles.cardTitleRow}>
            <Text style={styles.cardTitle} numberOfLines={2}>{candidate.title || "Review candidate"}</Text>
            <Text style={styles.reviewText}>Review</Text>
          </View>
          <Text style={styles.cardBody} numberOfLines={2}>{candidate.summary || "No summary provided."}</Text>
          <View style={styles.badgeRow}>
            <Text style={styles.typeBadge}>{classLabel(candidate.candidate_type)}</Text>
            <Text style={styles.subtleBadge}>{candidate.status}</Text>
          </View>
        </View>
      </View>
      <View style={styles.actionRow}>
        {hasMemoryAction(candidate, "reject_candidate") ? (
          <Pressable style={styles.secondaryAction} onPress={() => onReject(candidate)}>
            <Text style={styles.actionText}>Reject</Text>
          </Pressable>
        ) : null}
      </View>
    </View>
  );
}

function EmptyState({ title, body }: { title: string; body: string }) {
  return (
    <View style={styles.centerCard}>
      <Text style={styles.emptyTitle}>{title}</Text>
      <Text style={styles.mutedText}>{body}</Text>
    </View>
  );
}

function categoryIcon(memory: VisibleMemoryProjection) {
  if (memory.memory_class.includes("profile")) return "○";
  if (memory.memory_class.includes("task")) return "□";
  if (memory.memory_class.includes("summary")) return "◇";
  if (isProtectedMemoryProjection(memory)) return "⌂";
  return "●";
}

function statusLabel(memory: VisibleMemoryProjection) {
  if (isProtectedMemoryProjection(memory)) return "Protected";
  if (memory.status === "active") return "Active";
  if (memory.suppression_status === "suppressed") return "Suppressed";
  return classLabel(memory.status);
}

function classLabel(value: string) {
  return value
    .split("_")
    .filter(Boolean)
    .map((part) => part.charAt(0).toUpperCase() + part.slice(1))
    .join(" ");
}

const styles = StyleSheet.create({
  screen: { flex: 1, backgroundColor: "#0B1020" },
  scroll: { flex: 1 },
  content: { padding: 16, gap: 14 },
  headerRow: { flexDirection: "row", alignItems: "center", gap: 12 },
  backButton: { width: 36, height: 36, borderRadius: 18, alignItems: "center", justifyContent: "center" },
  backButtonText: { color: "#E5E7EB", fontSize: 30, lineHeight: 32 },
  headerText: { flex: 1 },
  title: { color: "#F8FAFC", fontSize: 24, fontWeight: "800" },
  subtitle: { color: "#94A3B8", marginTop: 3, lineHeight: 19 },
  searchRow: { flexDirection: "row", gap: 10 },
  searchInput: {
    flex: 1,
    minHeight: 48,
    borderRadius: 16,
    borderWidth: 1,
    borderColor: "#263653",
    backgroundColor: "#0F172A",
    color: "#F8FAFC",
    paddingHorizontal: 16
  },
  refreshButton: {
    width: 48,
    height: 48,
    borderRadius: 16,
    borderWidth: 1,
    borderColor: "#263653",
    backgroundColor: "#111827",
    alignItems: "center",
    justifyContent: "center"
  },
  refreshButtonText: { color: "#93C5FD", fontSize: 20, fontWeight: "800" },
  filterRow: { gap: 8, paddingRight: 2 },
  filterChip: {
    borderRadius: 999,
    borderWidth: 1,
    borderColor: "#1F2937",
    backgroundColor: "#111827",
    paddingHorizontal: 18,
    paddingVertical: 9
  },
  filterChipActive: { borderColor: "#2563EB", backgroundColor: "#102345" },
  filterText: { color: "#CBD5E1", fontWeight: "700" },
  filterTextActive: { color: "#BFDBFE" },
  list: { gap: 12 },
  card: { backgroundColor: "#111827", borderRadius: 16, padding: 14, borderWidth: 1, borderColor: "#1F2937" },
  cardTop: { flexDirection: "row", gap: 12 },
  iconBubble: {
    width: 44,
    height: 44,
    borderRadius: 22,
    backgroundColor: "#312E81",
    alignItems: "center",
    justifyContent: "center"
  },
  iconBubbleProtected: { backgroundColor: "#15412A" },
  iconBubbleReview: { backgroundColor: "#7C3AED" },
  iconText: { color: "#EDE9FE", fontWeight: "900", fontSize: 17 },
  cardMain: { flex: 1, minWidth: 0 },
  cardTitleRow: { flexDirection: "row", alignItems: "flex-start", justifyContent: "space-between", gap: 8 },
  cardTitle: { color: "#F8FAFC", fontSize: 17, fontWeight: "800", flex: 1 },
  statusText: { color: "#4ADE80", fontSize: 12, fontWeight: "800", marginTop: 2 },
  protectedText: { color: "#86EFAC" },
  reviewText: { color: "#93C5FD", fontSize: 12, fontWeight: "800", marginTop: 2 },
  cardBody: { color: "#CBD5E1", lineHeight: 19, marginTop: 5 },
  badgeRow: { flexDirection: "row", flexWrap: "wrap", gap: 6, marginTop: 8 },
  typeBadge: { color: "#C4B5FD", backgroundColor: "#312E81", borderRadius: 7, paddingHorizontal: 8, paddingVertical: 4, fontSize: 11, fontWeight: "800" },
  subtleBadge: { color: "#93C5FD", backgroundColor: "#172554", borderRadius: 7, paddingHorizontal: 8, paddingVertical: 4, fontSize: 11, fontWeight: "800" },
  actionRow: { flexDirection: "row", gap: 8, marginTop: 12, flexWrap: "wrap" },
  secondaryAction: {
    flexGrow: 1,
    minWidth: 86,
    borderRadius: 10,
    borderWidth: 1,
    borderColor: "#334155",
    backgroundColor: "#1F2937",
    paddingVertical: 10,
    alignItems: "center"
  },
  actionText: { color: "#E5E7EB", fontSize: 12, fontWeight: "800" },
  centerCard: { backgroundColor: "#111827", borderRadius: 16, padding: 18, borderWidth: 1, borderColor: "#1F2937", gap: 8 },
  emptyTitle: { color: "#F8FAFC", fontWeight: "800", fontSize: 16 },
  mutedText: { color: "#CBD5E1", lineHeight: 20 },
  errorCard: { backgroundColor: "#2A1218", borderRadius: 16, padding: 14, borderWidth: 1, borderColor: "#7F1D1D" },
  errorText: { color: "#FCA5A5", lineHeight: 20 },
  footerNote: { backgroundColor: "#0F172A", borderRadius: 14, padding: 12, borderWidth: 1, borderColor: "#1F2937" },
  footerNoteText: { color: "#CBD5E1", fontSize: 12, lineHeight: 18 }
});
