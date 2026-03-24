import { ScrollView, Pressable, StyleSheet, Text, View } from "react-native";

type StarterChipsProps = {
  prompts: string[];
  onSelect: (value: string) => void;
  disabled?: boolean;
};

export function StarterChips({ prompts, onSelect, disabled = false }: StarterChipsProps) {
  if (!prompts.length) return null;

  return (
    <View style={styles.wrap}>
      <ScrollView horizontal showsHorizontalScrollIndicator={false} contentContainerStyle={styles.row}>
        {prompts.map((prompt) => (
          <Pressable
            key={prompt}
            disabled={disabled}
            onPress={() => onSelect(prompt)}
            style={({ pressed }) => [
              styles.chip,
              disabled ? styles.chipDisabled : null,
              pressed ? styles.chipPressed : null
            ]}
          >
            <Text style={styles.text} numberOfLines={1}>
              {prompt}
            </Text>
          </Pressable>
        ))}
      </ScrollView>
    </View>
  );
}

const styles = StyleSheet.create({
  wrap: {
    marginTop: 8
  },
  row: {
    gap: 8,
    paddingVertical: 2
  },
  chip: {
    paddingVertical: 8,
    paddingHorizontal: 12,
    borderRadius: 999,
    backgroundColor: "#111827",
    borderWidth: 1,
    borderColor: "#1F2937"
  },
  chipPressed: {
    transform: [{ scale: 0.98 }],
    opacity: 0.9
  },
  chipDisabled: {
    opacity: 0.5
  },
  text: {
    color: "#CBD5E1",
    fontSize: 13,
    letterSpacing: 0.2,
    maxWidth: 240
  }
});

