const MAX_CACHED_ITEMS = 10;

const warnedKeys = new Set<string>();

function warnOnce(key: string, message: string, err?: unknown): void {
  if (warnedKeys.has(key)) return;
  warnedKeys.add(key);
  console.warn(message, err);
}

export function saveToLocalStorage<T>(key: string, data: T): void {
  try {
    localStorage.setItem(key, JSON.stringify(data));
  } catch (err) {
    warnOnce(`save:${key}`, `Failed to save to localStorage [${key}]:`, err);
  }
}

export function loadFromLocalStorage<T>(key: string): T | null {
  try {
    const stored = localStorage.getItem(key);
    if (stored) return JSON.parse(stored) as T;
  } catch (err) {
    warnOnce(`load:${key}`, `Failed to load from localStorage [${key}]:`, err);
  }
  return null;
}

export function removeFromLocalStorage(key: string): void {
  try {
    localStorage.removeItem(key);
  } catch (err) {
    warnOnce(`remove:${key}`, `Failed to remove from localStorage [${key}]:`, err);
  }
}

function getKeysByPrefix(prefix: string): string[] {
  const keys: string[] = [];
  for (let i = 0; i < localStorage.length; i++) {
    const key = localStorage.key(i);
    if (key && key.startsWith(prefix)) {
      keys.push(key);
    }
  }
  return keys;
}

export function saveWithEviction<T extends { lastUpdated?: number }>(
  keyPrefix: string,
  key: string,
  data: T,
  maxItems: number = MAX_CACHED_ITEMS
): void {
  if (maxItems <= 0) return;
  
  try {
    const fullKey = `${keyPrefix}${key}`;
    const existingKeys = getKeysByPrefix(keyPrefix);
    const isUpdate = existingKeys.includes(fullKey);
    
    if (!isUpdate && existingKeys.length >= maxItems) {
      const oldestKey = existingKeys
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
    
    const dataWithTimestamp = { ...data, lastUpdated: data.lastUpdated ?? Date.now() };
    localStorage.setItem(fullKey, JSON.stringify(dataWithTimestamp));
  } catch (err) {
    warnOnce(`evict:${keyPrefix}${key}`, `Failed to save with eviction [${keyPrefix}${key}]:`, err);
  }
}
