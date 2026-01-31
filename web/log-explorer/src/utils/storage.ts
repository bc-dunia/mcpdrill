const MAX_CACHED_ITEMS = 10;

export function saveToLocalStorage<T>(key: string, data: T): void {
  try {
    localStorage.setItem(key, JSON.stringify(data));
  } catch (err) {
    console.warn(`Failed to save to localStorage [${key}]:`, err);
  }
}

export function loadFromLocalStorage<T>(key: string): T | null {
  try {
    const stored = localStorage.getItem(key);
    if (stored) return JSON.parse(stored) as T;
  } catch (err) {
    console.warn(`Failed to load from localStorage [${key}]:`, err);
  }
  return null;
}

export function removeFromLocalStorage(key: string): void {
  try {
    localStorage.removeItem(key);
  } catch (err) {
    console.warn(`Failed to remove from localStorage [${key}]:`, err);
  }
}

export function saveWithEviction<T extends { lastUpdated?: number }>(
  keyPrefix: string,
  key: string,
  data: T,
  maxItems: number = MAX_CACHED_ITEMS
): void {
  try {
    const keys = Object.keys(localStorage).filter(k => k.startsWith(keyPrefix));
    if (keys.length >= maxItems) {
      const oldestKey = keys
        .map(k => {
          try {
            const item = JSON.parse(localStorage.getItem(k) || '{}');
            return { key: k, time: item.lastUpdated || 0 };
          } catch {
            return { key: k, time: 0 };
          }
        })
        .sort((a, b) => a.time - b.time)[0]?.key;
      if (oldestKey) localStorage.removeItem(oldestKey);
    }
    localStorage.setItem(`${keyPrefix}${key}`, JSON.stringify(data));
  } catch (err) {
    console.warn(`Failed to save with eviction [${keyPrefix}${key}]:`, err);
  }
}
