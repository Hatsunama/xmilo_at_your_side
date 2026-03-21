import { useEffect, useState } from "react";
import { ScrollView, StyleSheet, Text, View } from "react-native";
import { listArchiveRecords, initArchiveDb } from "../src/lib/archiveDb";

type ArchiveRow = {
  task_id: string;
  title: string;
  description: string;
  created_at: string;
};

export default function ArchiveScreen() {
  const [rows, setRows] = useState<ArchiveRow[]>([]);

  useEffect(() => {
    async function load() {
      await initArchiveDb();
      setRows(await listArchiveRecords());
    }
    load();
  }, []);

  return (
    <ScrollView style={styles.screen} contentContainerStyle={styles.content}>
      <Text style={styles.title}>Archive</Text>
      {rows.length === 0 ? (
        <View style={styles.card}>
          <Text style={styles.emptyTitle}>No archive records yet</Text>
          <Text style={styles.emptyBody}>`archive.record_created` events will be cached here as the sidecar starts completing tasks.</Text>
        </View>
      ) : (
        rows.map((row) => (
          <View style={styles.card} key={`${row.task_id}-${row.created_at}`}>
            <Text style={styles.cardTitle}>{row.title}</Text>
            <Text style={styles.cardMeta}>{row.created_at}</Text>
            <Text style={styles.cardBody}>{row.description}</Text>
          </View>
        ))
      )}
    </ScrollView>
  );
}

const styles = StyleSheet.create({
  screen: { flex: 1, backgroundColor: "#0B1020" },
  content: { padding: 16, gap: 12 },
  title: { color: "#F8FAFC", fontSize: 22, fontWeight: "700" },
  card: { backgroundColor: "#111827", borderRadius: 18, padding: 16, borderWidth: 1, borderColor: "#1F2937" },
  emptyTitle: { color: "#F8FAFC", fontWeight: "700" },
  emptyBody: { color: "#CBD5E1", lineHeight: 20, marginTop: 8 },
  cardTitle: { color: "#F8FAFC", fontWeight: "700" },
  cardMeta: { color: "#93C5FD", marginTop: 4 },
  cardBody: { color: "#CBD5E1", lineHeight: 20, marginTop: 10 }
});
