import { Pressable, StyleSheet, Text, View } from "react-native";

export function StagedContentChip({
  label,
  meta,
  preview,
  onRemove,
}: {
  label: string;
  meta: string;
  preview?: string | null;
  onRemove: () => void;
}) {
  return (
    <View style={styles.chip}>
      <Text style={styles.text} numberOfLines={1}>
        📎 {meta} · {label}{preview ? ` · "${preview}…"` : ""}
      </Text>
      <Pressable onPress={onRemove} hitSlop={8} style={styles.dismiss}>
        <Text style={styles.dismissText}>✕</Text>
      </Pressable>
    </View>
  );
}

const styles = StyleSheet.create({
  chip: {
    flexDirection: "row",
    alignItems: "center",
    backgroundColor: "#2A1E4A",
    borderRadius: 10,
    borderWidth: 1,
    borderColor: "#5B3FA6",
    paddingHorizontal: 12,
    paddingVertical: 7,
    gap: 8,
  },
  text: {
    flex: 1,
    color: "#C8B8FF",
    fontSize: 12,
    fontWeight: "600",
  },
  dismiss: {
    paddingHorizontal: 4,
  },
  dismissText: {
    color: "#7B5FD6",
    fontSize: 14,
    fontWeight: "700",
  },
});
