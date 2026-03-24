export const CASTLE_RENDERER_BUDGETS = {
  healthCheckDeadlineMs: 180,
  coldStartReadyBudgetMs: 450,
  fallbackDecisionDeadlineMs: 220,
  activeSceneMinFps: 30,
  nightlySceneMinFps: 24,
  rendererMemoryCeilingMb: 96,
  rendererWarmMemoryTargetMb: 64,
  nightlyRitualPeakExtraMemoryMb: 12,
} as const;

export type CastleRendererDegradationReason =
  | "native_module_unavailable"
  | "platform_not_android"
  | "renderer_health_timeout"
  | "renderer_disabled";
