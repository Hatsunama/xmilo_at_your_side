import { useFocusEffect, useRouter } from "expo-router";
import { useCallback, useState } from "react";
import { Pressable, ScrollView, StyleSheet, Text, View } from "react-native";

import { listArchiveRecords } from "../src/lib/archiveDb";

type ArchiveRecord = Awaited<ReturnType<typeof listArchiveRecords>>[number];

export default function ArchiveScreen() {
  const router = useRouter();
  const [records, setRecords] = useState<ArchiveRecord[]>([]);
  const [error, setError] = useState("");

  const refreshArchive = useCallback(async () => {
    try {
      setRecords(await listArchiveRecords());
      setError("");
    } catch (archiveError: any) {
      setError(archiveError?.message ?? "Could not load archive records.");
    }
  }, []);

  useFocusEffect(
    useCallback(() => {
      void refreshArchive();
    }, [refreshArchive])
  );

  return (
    <ScrollView style={styles.screen} contentContainerStyle={styles.content}>
      <View style={styles.headerCard}>
        <Text style={styles.kicker}>Archive</Text>
        <Text style={styles.title}>Milo's reports</Text>
        <Text style={styles.body}>
          Historical reports live here. Current task status stays on the Lair and Main Hall send surfaces.
        </Text>
        <View style={styles.row}>
          <Pressable style={styles.secondaryButton} onPress={() => router.back()}>
            <Text style={styles.secondaryButtonText}>← Back</Text>
          </Pressable>
          <Pressable style={styles.primaryButton} onPress={refreshArchive}>
            <Text style={styles.primaryButtonText}>Refresh</Text>
          </Pressable>
        </View>
      </View>

      {error ? (
        <View style={styles.errorCard}>
          <Text style={styles.errorText}>{error}</Text>
        </View>
      ) : null}

      {records.length === 0 && !error ? (
        <View style={styles.emptyCard}>
          <Text style={styles.emptyTitle}>No archived reports yet</Text>
          <Text style={styles.body}>When Milo completes a report, the sidecar will add it here.</Text>
        </View>
      ) : null}

      {records.map((record) => (
        <View key={`${record.task_id}-${record.created_at}`} style={styles.recordCard}>
          <Text style={styles.recordTitle}>{record.title || "Untitled report"}</Text>
          <Text style={styles.recordTime}>{new Date(record.created_at).toLocaleString()}</Text>
          <Text style={styles.recordBody}>{record.description || "No description provided."}</Text>
        </View>
      ))}
    </ScrollView>
  );
}

const styles = StyleSheet.create({
  screen: { flex: 1, backgroundColor: "#0B1020" },
  content: { padding: 16, gap: 16 },
  headerCard: { backgroundColor: "#111827", borderRadius: 18, padding: 16, borderWidth: 1, borderColor: "#1F2937" },
  kicker: { color: "#93C5FD", fontSize: 12, fontWeight: "700", textTransform: "uppercase", letterSpacing: 1 },
  title: { color: "#F8FAFC", fontSize: 24, fontWeight: "700", marginTop: 6 },
  body: { color: "#CBD5E1", marginTop: 8, lineHeight: 20 },
  row: { flexDirection: "row", gap: 12, marginTop: 14 },
  primaryButton: { flex: 1, backgroundColor: "#2563EB", paddingVertical: 12, borderRadius: 14, alignItems: "center" },
  primaryButtonText: { color: "#FFFFFF", fontWeight: "700" },
  secondaryButton: { flex: 1, backgroundColor: "#1F2937", paddingVertical: 12, borderRadius: 14, alignItems: "center" },
  secondaryButtonText: { color: "#E5E7EB", fontWeight: "700" },
  emptyCard: { backgroundColor: "#111827", borderRadius: 18, padding: 16, borderWidth: 1, borderColor: "#1F2937" },
  emptyTitle: { color: "#F8FAFC", fontWeight: "700" },
  errorCard: { backgroundColor: "#2A1218", borderRadius: 18, padding: 16, borderWidth: 1, borderColor: "#7F1D1D" },
  errorText: { color: "#FCA5A5", lineHeight: 20 },
  recordCard: { backgroundColor: "#111827", borderRadius: 18, padding: 16, borderWidth: 1, borderColor: "#1F2937" },
  recordTitle: { color: "#F8FAFC", fontSize: 18, fontWeight: "700" },
  recordTime: { color: "#94A3B8", marginTop: 6, fontSize: 12 },
  recordBody: { color: "#CBD5E1", marginTop: 10, lineHeight: 20 },
});
