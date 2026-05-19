import js from "@eslint/js";
import reactHooks from "eslint-plugin-react-hooks";
import tseslint from "typescript-eslint";

export default tseslint.config(
  {
    ignores: [
      "node_modules/**",
      "android/**",
      "dist/**",
      ".expo/**",
      "_artifacts/**",
      "validation-artifacts/**"
    ]
  },
  js.configs.recommended,
  ...tseslint.configs.recommended,
  {
    files: ["app/**/*.{ts,tsx}", "src/**/*.{ts,tsx}", "*.{ts,tsx}"],
    plugins: {
      "react-hooks": reactHooks
    },
    languageOptions: {
      parserOptions: {
        ecmaFeatures: {
          jsx: true
        },
        sourceType: "module"
      }
    },
    rules: {
      "no-undef": "off",
      "no-empty": ["error", { "allowEmptyCatch": true }],
      "no-useless-assignment": "off",
      "@typescript-eslint/no-explicit-any": "off",
      "@typescript-eslint/no-unused-vars": "off",
      "react-hooks/rules-of-hooks": "error",
      "react-hooks/exhaustive-deps": "warn"
    }
  }
);
