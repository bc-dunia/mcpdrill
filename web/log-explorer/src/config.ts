/**
 * UI Configuration Constants
 * Centralized configuration for refresh intervals, debouncing, and storage keys
 */

export const CONFIG = {
  REFRESH_INTERVALS: {
    METRICS: 2000,
    SERVER_RESOURCES: 2000,
    ERROR_SIGNATURES: 5000,
  },
  DEBOUNCE_MS: 500,
  MAX_DATA_POINTS: 60,
  MAX_CACHED_RUNS: 10,
  BATCH_SIZE: 1000,
};

export const STORAGE_KEYS = {
  METRICS_PREFIX: 'mcpdrill_metrics_',
  SERVER_METRICS_PREFIX: 'mcpdrill_server_metrics_',
  ARG_PRESETS: 'mcpdrill-arg-presets',
  WIZARD_PROGRESS: 'mcpdrill-wizard-progress',
};
