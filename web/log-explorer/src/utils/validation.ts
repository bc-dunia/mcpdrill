export function isValidUrl(urlString: string): boolean {
  const trimmed = urlString?.trim();
  if (!trimmed) return false;
  try {
    const url = new URL(trimmed);
    return ['http:', 'https:'].includes(url.protocol);
  } catch {
    return false;
  }
}

export function normalizeHeaderKey(key: string): string {
  return key.toLowerCase().trim();
}
