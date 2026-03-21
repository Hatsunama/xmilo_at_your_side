import { Pressable, StyleSheet, Text, View } from "react-native";

export function StarterChips({
  prompts,
  onSelect
}: {
  prompts: string[];
  onSelect: (prompt: string) => void;
}) {
  return (
    <View style={styles.wrap}>
      {prompts.map((prompt) => (
        <Pressable key={prompt} style={styles.chip} onPress={() => onSelect(prompt)}>
          <Text style={styles.text}>{prompt}</Text>
        </Pressable>
      ))}
    </View>
  );
}

const styles = StyleSheet.create({
  wrap: { flexDirection: "row", flexWrap: "wrap", gap: 10 },
  chip: {
    backgroundColor: "#111827",
    borderRadius: 999,
    borderWidth: 1,
    borderColor: "#1F2937",
    paddingHorizontal: 14,
    paddingVertical: 10
  },
  text: { color: "#E5E7EB", fontWeight: "600" }
});
