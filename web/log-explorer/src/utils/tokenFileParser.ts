/**
 * Parses token files in multiple formats (txt, csv, json)
 * @param file - The file to parse (File object from browser)
 * @returns Promise resolving to an array of non-empty trimmed tokens
 * @throws Error if file format is unsupported or content is invalid
 */
export async function parseTokenFile(file: File): Promise<string[]> {
  const extension = file.name.split('.').pop()?.toLowerCase();

  if (!extension) {
    throw new Error('File must have an extension (.txt, .csv, or .json)');
  }

  const content = await file.text();

  if (!content.trim()) {
    throw new Error('File is empty');
  }

  switch (extension) {
    case 'txt':
      return parseTxt(content);
    case 'csv':
      return parseCsv(content);
    case 'json':
      return parseJson(content);
    default:
      throw new Error(
        `Unsupported file format: .${extension}. Supported formats: .txt, .csv, .json`
      );
  }
}

/**
 * Parses TXT format: line-separated tokens
 */
function parseTxt(content: string): string[] {
  return content
    .split('\n')
    .map((line) => line.trim())
    .filter((line) => line.length > 0);
}

/**
 * Parses CSV format: handles both with and without header row
 * Assumes tokens are in the first column
 */
function parseCsv(content: string): string[] {
  const lines = content.split('\n').map((line) => line.trim());

  if (lines.length === 0) {
    throw new Error('CSV file is empty');
  }

  // Check if first line is a header (common patterns: "token", "Token", "TOKEN")
  const firstLine = lines[0];
  const isHeader =
    firstLine.toLowerCase() === 'token' ||
    firstLine.toLowerCase() === 'tokens' ||
    firstLine.toLowerCase() === '"token"' ||
    firstLine.toLowerCase() === "'token'";

  const startIndex = isHeader ? 1 : 0;
  const tokens: string[] = [];

  for (let i = startIndex; i < lines.length; i++) {
    const line = lines[i];
    if (!line) continue;

    // Extract first column (handle quoted values)
    const firstColumn = line.split(',')[0].trim().replace(/^["']|["']$/g, '');

    if (firstColumn) {
      tokens.push(firstColumn);
    }
  }

  if (tokens.length === 0) {
    throw new Error('No tokens found in CSV file');
  }

  return tokens;
}

/**
 * Parses JSON format: supports both array and object with "tokens" key
 */
function parseJson(content: string): string[] {
  let parsed: unknown;

  try {
    parsed = JSON.parse(content);
  } catch (error) {
    throw new Error(
      `Invalid JSON format: ${error instanceof Error ? error.message : 'Unknown error'}`
    );
  }

  let tokens: unknown[] = [];

  if (Array.isArray(parsed)) {
    tokens = parsed;
  } else if (
    typeof parsed === 'object' &&
    parsed !== null &&
    'tokens' in parsed
  ) {
    const tokensValue = (parsed as Record<string, unknown>).tokens;
    if (Array.isArray(tokensValue)) {
      tokens = tokensValue;
    } else {
      throw new Error(
        'JSON object must have a "tokens" key with an array value'
      );
    }
  } else {
    throw new Error(
      'JSON must be either an array of tokens or an object with a "tokens" key'
    );
  }

  if (tokens.length === 0) {
    throw new Error('No tokens found in JSON file');
  }

  // Convert all tokens to strings and filter empty ones
  const result = tokens
    .map((token) => String(token).trim())
    .filter((token) => token.length > 0);

  if (result.length === 0) {
    throw new Error('No valid tokens found in JSON file');
  }

  return result;
}
