// Access-token bootstrap for remote mode.
//
// In remote mode the server guards the UI and every /_sim/* endpoint with a
// token. The SPA cannot read it from a header, so we accept it from the URL
// (?token=...) on first load — or from an optional <meta name="sim-token">
// the server may inject — persist it in sessionStorage, and attach it to every
// fetch (X-Sim-Token) and EventSource (?token=) request.

const STORAGE_KEY = 'sim-token';

function readMetaToken(): string {
  if (typeof document === 'undefined') {
    return '';
  }
  const meta = document.querySelector('meta[name="sim-token"]');
  const value = meta?.getAttribute('content')?.trim() ?? '';
  // Ignore an un-substituted server placeholder.
  return value && !value.startsWith('{{') ? value : '';
}

function bootstrap(): string {
  if (typeof window === 'undefined') {
    return '';
  }
  const params = new URLSearchParams(window.location.search);
  const fromQuery = params.get('token')?.trim() ?? '';
  if (fromQuery) {
    try {
      window.sessionStorage.setItem(STORAGE_KEY, fromQuery);
    } catch {
      /* sessionStorage may be unavailable (private mode); fall back to memory */
    }
    // Strip the token from the visible URL so it is not left in the address bar.
    params.delete('token');
    const query = params.toString();
    const next = window.location.pathname + (query ? `?${query}` : '') + window.location.hash;
    window.history.replaceState(null, '', next);
    return fromQuery;
  }
  try {
    const stored = window.sessionStorage.getItem(STORAGE_KEY);
    if (stored) {
      return stored;
    }
  } catch {
    /* ignore */
  }
  return readMetaToken();
}

let cachedToken = bootstrap();

export function getSimToken(): string {
  return cachedToken;
}

export function setSimToken(token: string): void {
  cachedToken = token;
  try {
    if (token) {
      window.sessionStorage.setItem(STORAGE_KEY, token);
    } else {
      window.sessionStorage.removeItem(STORAGE_KEY);
    }
  } catch {
    /* ignore */
  }
}

/** Headers to merge into fetch() calls against the control plane. */
export function tokenHeaders(): Record<string, string> {
  return cachedToken ? { 'X-Sim-Token': cachedToken } : {};
}

/** Append the access token to an SSE/EventSource URL when one is set. */
export function withTokenURL(path: string): string {
  if (!cachedToken) {
    return path;
  }
  const separator = path.includes('?') ? '&' : '?';
  return `${path}${separator}token=${encodeURIComponent(cachedToken)}`;
}
