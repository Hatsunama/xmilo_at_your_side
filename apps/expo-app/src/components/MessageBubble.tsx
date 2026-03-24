import * as Clipboard from "expo-clipboard";
import { Pressable, StyleSheet, Text, View } from "react-native";
import type { EventEnvelope } from "../types/contracts";

export function MessageBubble({ event, onReport }: { event: EventEnvelope; onReport?: (event: EventEnvelope) => void }) {
  const text = event.payload?.report_text || event.payload?.text || event.payload?.message || JSON.stringify(event.payload);

  return (
    <Pressable style={styles.card} onLongPress={() => Clipboard.setStringAsync(String(text))}>
      <View style={styles.headerRow}>
        <View style={styles.headerMeta}>
          <Text style={styles.type}>{event.type}</Text>
          <Text style={styles.time}>{new Date(event.timestamp).toLocaleTimeString()}</Text>
        </View>
        {onReport ? (
          <Pressable style={styles.reportButton} onPress={() => onReport(event)}>
            <Text style={styles.reportButtonText}>Report</Text>
          </Pressable>
        ) : null}
      </View>
      <MarkdownishText text={String(text)} />
    </Pressable>
  );
}

function MarkdownishText({ text }: { text: string }) {
  const lines = text.split("\n");
  return (
    <View style={styles.bodyWrap}>
      {lines.map((line, index) => {
        const trimmed = line.trim();
        const bullet = trimmed.startsWith("- ") || trimmed.startsWith("* ");
        return (
          <Text key={`${line}-${index}`} style={styles.body}>
            {bullet ? "• " + trimmed.slice(2) : renderInline(trimmed)}
          </Text>
        );
      })}
    </View>
  );
}

function renderInline(input: string) {
  const parts = input.split(/(\*\*[^*]+\*\*|`[^`]+`)/g).filter(Boolean);
  return (
    <>
      {parts.map((part, index) => {
        if (part.startsWith("**") && part.endsWith("**")) {
          return (
            <Text key={index} style={styles.bold}>
              {part.slice(2, -2)}
            </Text>
          );
        }
        if (part.startsWith("`") && part.endsWith("`")) {
          return (
            <Text key={index} style={styles.code}>
              {part.slice(1, -1)}
            </Text>
          );
        }
        return <Text key={index}>{part}</Text>;
      })}
    </>
  );
}

const styles = StyleSheet.create({
  card: { backgroundColor: "#111827", borderRadius: 18, padding: 16, borderWidth: 1, borderColor: "#1F2937" },
  headerRow: { flexDirection: "row", justifyContent: "space-between", marginBottom: 8, alignItems: "center" },
  headerMeta: { flex: 1, marginRight: 12 },
  type: { color: "#93C5FD", fontWeight: "700" },
  time: { color: "#94A3B8" },
  reportButton: {
    paddingHorizontal: 10,
    paddingVertical: 6,
    borderRadius: 999,
    backgroundColor: "#312E81",
    borderWidth: 1,
    borderColor: "#4338CA"
  },
  reportButtonText: { color: "#EDE9FE", fontWeight: "700", fontSize: 12 },
  bodyWrap: { gap: 6 },
  body: { color: "#E5E7EB", lineHeight: 21 },
  bold: { fontWeight: "700", color: "#F8FAFC" },
  code: { fontFamily: "monospace", backgroundColor: "#0F172A", color: "#F8FAFC" }
});
