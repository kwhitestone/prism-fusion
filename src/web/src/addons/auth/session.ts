const DEFAULT_ACCESS_TTL_MS = 15 * 60 * 1000;
const AUTH_SESSION_LOCK_NAME = "prism-fusion-auth-session";

export function accessTokenExpiry(expiresIn: unknown, now = Date.now()): Date {
  if (typeof expiresIn !== "string") {
    return new Date(now + DEFAULT_ACCESS_TTL_MS);
  }
  const match = expiresIn
    .trim()
    .match(/^(\d+(?:\.\d+)?)(ns|us|µs|ms|s|m|h|d)$/i);
  if (!match) {
    return new Date(now + DEFAULT_ACCESS_TTL_MS);
  }
  const unitMilliseconds: Record<string, number> = {
    ns: 0.000001,
    us: 0.001,
    µs: 0.001,
    ms: 1,
    s: 1000,
    m: 60 * 1000,
    h: 60 * 60 * 1000,
    d: 24 * 60 * 60 * 1000
  };
  const duration = Number(match[1]) * unitMilliseconds[match[2].toLowerCase()];
  return new Date(
    now +
      (Number.isFinite(duration) && duration > 0
        ? Math.max(1, duration)
        : DEFAULT_ACCESS_TTL_MS)
  );
}

export function createSingleFlight<T>(
  task: () => Promise<T>
): () => Promise<T> {
  let current: Promise<T> | null = null;
  return () => {
    if (!current) {
      current = Promise.resolve()
        .then(task)
        .finally(() => {
          current = null;
        });
    }
    return current;
  };
}

export type RefreshOutcome = "ready" | "changed" | "failed";

/** A 401 may be refreshed/replayed only when the original request was bound. */
export function canRecoverAuthFailure(sessionId?: string): sessionId is string {
  return typeof sessionId === "string" && sessionId.length > 0;
}

export interface RefreshSessionSnapshot {
  refreshToken: string;
  sessionId?: string;
  refreshRequestId?: string;
  expires: number;
}

export function createRefreshCoordinator(options: {
  readSession: () => RefreshSessionSnapshot | null;
  hasAuthToken: () => boolean;
  withExclusiveLock: <T>(task: () => Promise<T>) => Promise<T>;
  rotate: (
    session: Required<RefreshSessionSnapshot>
  ) => Promise<RefreshOutcome>;
  invalidate: (session: RefreshSessionSnapshot) => void;
  now?: () => number;
}): (expectedSessionId?: string) => Promise<RefreshOutcome> {
  const now = options.now ?? Date.now;
  let active:
    | {
        expectedSessionId?: string;
        promise: Promise<RefreshOutcome>;
      }
    | undefined;

  const run = async (expectedSessionId?: string): Promise<RefreshOutcome> => {
    const snapshot = options.readSession();
    if (!snapshot?.refreshToken) return "failed";
    if (
      expectedSessionId !== undefined &&
      snapshot.sessionId !== expectedSessionId
    ) {
      return "changed";
    }

    try {
      return await options.withExclusiveLock(async () => {
        const current = options.readSession();
        if (!current || current.sessionId !== snapshot.sessionId) {
          return "changed";
        }
        if (current.refreshToken !== snapshot.refreshToken) {
          return current.expires > now() && options.hasAuthToken()
            ? "ready"
            : "changed";
        }
        if (!current.sessionId || !current.refreshRequestId) {
          options.invalidate(current);
          return "failed";
        }
        const outcome = await options.rotate(
          current as Required<RefreshSessionSnapshot>
        );
        if (outcome === "failed") {
          // Invalidating while the same cross-tab lock is still held closes
          // the gap where a new login could otherwise be erased by an old
          // refresh failure after this task returns.
          const latest = options.readSession();
          if (
            latest?.sessionId === current.sessionId &&
            latest.refreshToken === current.refreshToken
          ) {
            options.invalidate(latest);
          }
        }
        return outcome;
      });
    } catch {
      return "failed";
    }
  };

  return expectedSessionId => {
    if (active) {
      if (
        expectedSessionId !== undefined &&
        expectedSessionId !== active.expectedSessionId
      ) {
        return Promise.resolve("changed");
      }
      return active.promise;
    }
    const promise = run(expectedSessionId).finally(() => {
      if (active?.promise === promise) active = undefined;
    });
    active = { expectedSessionId, promise };
    return promise;
  };
}

let authSessionEpoch = 0;

export function currentAuthSessionEpoch(): number {
  return authSessionEpoch;
}

export function invalidateAuthSession(): void {
  authSessionEpoch += 1;
}

export function isAuthSessionEpoch(epoch: number): boolean {
  return epoch === authSessionEpoch;
}

export function createAuthRequestID(): string {
  return globalThis.crypto.randomUUID();
}

let observedAuthSessionId: string | undefined;
let authSessionObservationInitialized = false;

export function observeAuthSession(sessionId?: string): void {
  observedAuthSessionId = sessionId;
  authSessionObservationInitialized = true;
}

export function getObservedAuthSession(): {
  initialized: boolean;
  sessionId?: string;
} {
  return {
    initialized: authSessionObservationInitialized,
    sessionId: observedAuthSessionId
  };
}

export type CrossTabSessionAction = "none" | "reload" | "end";

export function crossTabSessionAction(
  observedSessionId: string | undefined,
  nextSessionId: string | undefined
): CrossTabSessionAction {
  if (observedSessionId === nextSessionId) return "none";
  return nextSessionId ? "reload" : "end";
}

export type AuthBindingSnapshot =
  | {
      action: "none";
      sessionId?: string;
      authToken: string | null;
    }
  | {
      action: "reload" | "end";
    };

/**
 * Read the session identity and access token as one cross-tab transaction.
 * All built-in auth writers use the same lock, so the returned token can
 * never belong to a different session than sessionId.
 */
export function readAuthBindingWithLock(options: {
  withExclusiveLock: <T>(task: () => Promise<T>) => Promise<T>;
  readSessionId: () => string | undefined;
  readObservedSessionId: () => string | undefined;
  readAuthToken: () => string | null;
}): Promise<AuthBindingSnapshot> {
  return options.withExclusiveLock(async () => {
    const sessionId = options.readSessionId();
    const action = crossTabSessionAction(
      options.readObservedSessionId(),
      sessionId
    );
    if (action !== "none") return { action };
    return {
      action,
      sessionId,
      authToken: options.readAuthToken()
    };
  });
}

export function applyRequestAuthBinding(options: {
  isRecoveryRequest: boolean;
  clearAuthorization: () => void;
  bind: () => Promise<void>;
}): Promise<void> {
  if (options.isRecoveryRequest) {
    options.clearAuthorization();
    return Promise.resolve();
  }
  return options.bind();
}

/**
 * Commit asynchronous identity data only while it still belongs to the
 * session and local auth epoch that started the request.
 */
export function commitAuthSessionResult(options: {
  expectedSessionId: string;
  expectedEpoch: number;
  withExclusiveLock: <T>(task: () => Promise<T>) => Promise<T>;
  readSessionId: () => string | undefined;
  isEpochCurrent: (epoch: number) => boolean;
  commit: () => void | Promise<void>;
}): Promise<boolean> {
  return options.withExclusiveLock(async () => {
    if (
      options.readSessionId() !== options.expectedSessionId ||
      !options.isEpochCurrent(options.expectedEpoch)
    ) {
      return false;
    }
    await options.commit();
    return true;
  });
}

export async function withAuthSessionLock<T>(
  task: () => Promise<T>
): Promise<T> {
  if (typeof navigator === "undefined" || !navigator.locks) {
    throw new Error("Web Locks API is required for built-in authentication");
  }
  return navigator.locks.request(
    AUTH_SESSION_LOCK_NAME,
    { mode: "exclusive" },
    task
  );
}
