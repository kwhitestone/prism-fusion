import assert from "node:assert/strict";
import test from "node:test";
import {
  accessTokenExpiry,
  applyRequestAuthBinding,
  canRecoverAuthFailure,
  commitAuthSessionResult,
  crossTabSessionAction,
  createRefreshCoordinator,
  createSingleFlight,
  currentAuthSessionEpoch,
  invalidateAuthSession,
  isAuthSessionEpoch,
  readAuthBindingWithLock
} from "./session.js";

test("an anonymous 401 cannot be refreshed or replayed after login", () => {
  assert.equal(canRecoverAuthFailure(undefined), false);
  assert.equal(canRecoverAuthFailure(""), false);
  assert.equal(canRecoverAuthFailure("session-2"), true);
});

test("a delayed user-info response cannot revive a logged-out session", async () => {
  let committed = false;
  const committedResult = await commitAuthSessionResult({
    expectedSessionId: "session-1",
    expectedEpoch: 1,
    withExclusiveLock: async task => task(),
    readSessionId: () => undefined,
    isEpochCurrent: epoch => epoch === 2,
    commit: () => {
      committed = true;
    }
  });

  assert.equal(committedResult, false);
  assert.equal(committed, false);
});

test("a delayed user-info response cannot overwrite a replacement account", async () => {
  let committed = false;
  const committedResult = await commitAuthSessionResult({
    expectedSessionId: "session-1",
    expectedEpoch: 1,
    withExclusiveLock: async task => task(),
    readSessionId: () => "session-2",
    isEpochCurrent: () => true,
    commit: () => {
      committed = true;
    }
  });

  assert.equal(committedResult, false);
  assert.equal(committed, false);
});

test("cross-tab account switches never reuse a stale UI session", () => {
  assert.equal(crossTabSessionAction("session-1", "session-1"), "none");
  assert.equal(crossTabSessionAction("session-1", "session-2"), "reload");
  assert.equal(crossTabSessionAction("session-1", undefined), "end");
  assert.equal(crossTabSessionAction(undefined, "session-2"), "reload");
});

test("request binding never returns a token from a replacement session", async () => {
  let sessionId = "session-1";
  let authToken = "token-1";
  let releaseLock!: () => void;
  const binding = readAuthBindingWithLock({
    withExclusiveLock: task =>
      new Promise(resolve => {
        releaseLock = () => void task().then(resolve);
      }),
    readSessionId: () => sessionId,
    readObservedSessionId: () => "session-1",
    readAuthToken: () => authToken
  });

  await Promise.resolve();
  sessionId = "session-2";
  authToken = "token-2";
  releaseLock();

  assert.deepEqual(await binding, { action: "reload" });
});

test("request binding reads identity and token while holding one lock", async () => {
  let lockHeld = false;
  const binding = await readAuthBindingWithLock({
    withExclusiveLock: async task => {
      lockHeld = true;
      try {
        return await task();
      } finally {
        lockHeld = false;
      }
    },
    readSessionId: () => {
      assert.equal(lockHeld, true);
      return "session-1";
    },
    readObservedSessionId: () => "session-1",
    readAuthToken: () => {
      assert.equal(lockHeld, true);
      return "token-1";
    }
  });

  assert.deepEqual(binding, {
    action: "none",
    sessionId: "session-1",
    authToken: "token-1"
  });
});

test("a delayed response cannot refresh a replacement account", async () => {
  let rotations = 0;
  const coordinator = createRefreshCoordinator({
    readSession: () => ({
      refreshToken: "token-2",
      sessionId: "session-2",
      refreshRequestId: "request-2",
      expires: 1
    }),
    hasAuthToken: () => true,
    withExclusiveLock: async task => task(),
    invalidate: () => {},
    rotate: async () => {
      rotations++;
      return "ready";
    }
  });

  assert.equal(await coordinator("session-1"), "changed");
  assert.equal(rotations, 0);
});

test("refresh requests do not reacquire the non-reentrant auth lock", async () => {
  let locked = false;
  let bindCalls = 0;
  let authorizationCleared = false;
  const nonReentrantLock = async <T>(task: () => Promise<T>): Promise<T> => {
    if (locked) throw new Error("auth lock is not reentrant");
    locked = true;
    try {
      return await task();
    } finally {
      locked = false;
    }
  };
  const coordinator = createRefreshCoordinator({
    readSession: () => ({
      refreshToken: "token-1",
      sessionId: "session-1",
      refreshRequestId: "request-1",
      expires: 1
    }),
    hasAuthToken: () => true,
    withExclusiveLock: nonReentrantLock,
    invalidate: () => {},
    rotate: async () => {
      await applyRequestAuthBinding({
        isRecoveryRequest: true,
        clearAuthorization: () => {
          authorizationCleared = true;
        },
        bind: async () => {
          bindCalls++;
          await nonReentrantLock(async () => {});
        }
      });
      return "ready";
    }
  });

  assert.equal(await coordinator("session-1"), "ready");
  assert.equal(bindCalls, 0);
  assert.equal(authorizationCleared, true);
});

test("accessTokenExpiry follows backend duration values", () => {
  const now = 1_000_000;
  assert.equal(accessTokenExpiry("15m", now).getTime(), now + 15 * 60 * 1000);
  assert.equal(
    accessTokenExpiry("2h", now).getTime(),
    now + 2 * 60 * 60 * 1000
  );
  assert.equal(
    accessTokenExpiry("invalid", now).getTime(),
    now + 15 * 60 * 1000
  );
});

test("createSingleFlight shares concurrent work and resets after completion", async () => {
  let calls = 0;
  let release!: (value: boolean) => void;
  const work = createSingleFlight(
    () =>
      new Promise<boolean>(resolve => {
        calls++;
        release = resolve;
      })
  );

  const first = work();
  const second = work();
  assert.equal(calls, 0);
  await Promise.resolve();
  assert.equal(calls, 1);
  release(true);
  assert.equal(await first, true);
  assert.equal(await second, true);

  const third = work();
  await Promise.resolve();
  assert.equal(calls, 2);
  release(false);
  assert.equal(await third, false);
});

test("auth session epochs invalidate stale asynchronous work", () => {
  const epoch = currentAuthSessionEpoch();
  assert.equal(isAuthSessionEpoch(epoch), true);
  invalidateAuthSession();
  assert.equal(isAuthSessionEpoch(epoch), false);
});

test("refresh coordinator accepts another tab rotation in the same session", async () => {
  let session = {
    refreshToken: "old-token",
    sessionId: "same-session",
    refreshRequestId: "request-1",
    expires: 1
  };
  let releaseLock!: () => void;
  let rotations = 0;
  const coordinator = createRefreshCoordinator({
    readSession: () => session,
    hasAuthToken: () => true,
    now: () => 100,
    withExclusiveLock: task =>
      new Promise(resolve => {
        releaseLock = () => void task().then(resolve);
      }),
    invalidate: () => {},
    rotate: async () => {
      rotations++;
      return "ready";
    }
  });

  const result = coordinator();
  await Promise.resolve();
  await Promise.resolve();
  session = { ...session, refreshToken: "new-token", expires: 1_000 };
  releaseLock();

  assert.equal(await result, "ready");
  assert.equal(rotations, 0);
});

test("refresh coordinator aborts when another login replaces the session", async () => {
  const initial = {
    refreshToken: "old-token",
    sessionId: "old-session",
    refreshRequestId: "request-1",
    expires: 1
  };
  let session = initial;
  let releaseLock!: () => void;
  const coordinator = createRefreshCoordinator({
    readSession: () => session,
    hasAuthToken: () => true,
    withExclusiveLock: task =>
      new Promise(resolve => {
        releaseLock = () => void task().then(resolve);
      }),
    invalidate: () => {},
    rotate: async () => "ready"
  });

  const result = coordinator();
  await Promise.resolve();
  await Promise.resolve();
  session = { ...initial, sessionId: "new-session" };
  releaseLock();

  assert.equal(await result, "changed");
});

test("refresh failure invalidates the old session before releasing the lock", async () => {
  let session: {
    refreshToken: string;
    sessionId: string;
    refreshRequestId: string;
    expires: number;
  } | null = {
    refreshToken: "old-token",
    sessionId: "old-session",
    refreshRequestId: "request-1",
    expires: 1
  };
  let lockHeld = false;
  const coordinator = createRefreshCoordinator({
    readSession: () => session,
    hasAuthToken: () => true,
    withExclusiveLock: async task => {
      lockHeld = true;
      try {
        return await task();
      } finally {
        lockHeld = false;
      }
    },
    invalidate: expected => {
      assert.equal(lockHeld, true);
      if (
        session?.sessionId === expected.sessionId &&
        session.refreshToken === expected.refreshToken
      ) {
        session = null;
      }
    },
    rotate: async () => "failed"
  });

  assert.equal(await coordinator(), "failed");
  assert.equal(session, null);

  // A login that commits after the lock is released remains authoritative;
  // the old caller has no credential cleanup left to perform.
  session = {
    refreshToken: "new-token",
    sessionId: "new-session",
    refreshRequestId: "request-2",
    expires: 1_000
  };
  assert.equal(session.sessionId, "new-session");
});

test("legacy refresh metadata is invalidated under the shared lock", async () => {
  const legacySession = {
    refreshToken: "legacy-token",
    expires: 1
  };
  let session: typeof legacySession | null = legacySession;
  let lockHeld = false;
  let rotations = 0;
  const coordinator = createRefreshCoordinator({
    readSession: () => session,
    hasAuthToken: () => true,
    withExclusiveLock: async task => {
      lockHeld = true;
      try {
        return await task();
      } finally {
        lockHeld = false;
      }
    },
    invalidate: expected => {
      assert.equal(lockHeld, true);
      if (session?.refreshToken === expected.refreshToken) session = null;
    },
    rotate: async () => {
      rotations++;
      return "ready";
    }
  });

  assert.equal(await coordinator(), "failed");
  assert.equal(session, null);
  assert.equal(rotations, 0);
});
