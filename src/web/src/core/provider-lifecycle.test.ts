import assert from "node:assert/strict";
import { test } from "node:test";
import auth from "../addons/auth/index";
import rbac from "../addons/rbac/index";
import { getAsyncRoutes, setAsyncRoutesProvider } from "../api/routes";
import { setLoginHandler, useUserStoreHook } from "../store/modules/user";
import {
  setLoginComponent,
  useLoginUIStoreHook
} from "../store/modules/loginUI";
import {
  configureLock,
  configureLogin,
  configureRefresh,
  configureRoutes,
  fixtureState,
  resetFixture,
  successResponse
} from "../../tests/fixtures/provider-lifecycle";

const credentials = { username: "alice", password: "test-input" };

test("unconfigured auth/refresh fail closed and absent route provider grants no metadata", async () => {
  resetFixture();
  assert.equal(
    (await useUserStoreHook().loginByPassword(credentials)).success,
    false
  );
  assert.equal(
    (
      await useUserStoreHook().handRefreshToken({
        sessionId: "session",
        refreshToken: "refresh",
        requestId: "request"
      })
    ).success,
    false
  );
  assert.deepEqual(await getAsyncRoutes(), { success: true, data: [] });
});

test("auth destroy restores prior handlers/UI and removes the storage listener", async context => {
  context.mock.method(console, "log", () => {});
  resetFixture();
  const oldUI = { name: "ExternalLogin", render: () => null };
  const disposeUI = setLoginComponent(oldUI);
  const disposeLogin = setLoginHandler(
    async () => ({ success: false, message: "external" }) as any
  );
  await auth.setup();
  assert.equal(fixtureState().listeners, 1);
  assert.equal(useLoginUIStoreHook().loginComponent.name, "LoginForm");
  await auth.destroy();
  assert.equal(fixtureState().listeners, 0);
  assert.equal(useLoginUIStoreHook().loginComponent, oldUI);
  assert.equal(
    ((await useUserStoreHook().loginByPassword(credentials)) as any).message,
    "external"
  );
  assert.equal(fixtureState().token.sessionId, "session");
  disposeLogin();
  disposeUI();
  assert.equal(useLoginUIStoreHook().loginComponent, null);
  await auth.setup();
  assert.equal(fixtureState().listeners, 1);
  await auth.destroy();
  await auth.destroy();
  assert.equal(fixtureState().listeners, 0);
});

test("pending login and refresh responses cannot write credentials after auth disposal", async context => {
  context.mock.method(console, "log", () => {});
  for (const operation of ["login", "refresh"] as const) {
    resetFixture();
    let finish: (response: any) => void;
    const waiting = new Promise(resolve => {
      finish = resolve;
    });
    if (operation === "login") configureLogin(() => waiting);
    else configureRefresh(() => waiting);
    await auth.setup();
    const pending =
      operation === "login"
        ? useUserStoreHook().loginByPassword(credentials)
        : useUserStoreHook().handRefreshToken({
            sessionId: "session",
            refreshToken: "refresh",
            requestId: "request"
          });
    await auth.destroy();
    finish(successResponse());
    assert.equal((await pending).success, false);
    assert.equal(fixtureState().tokenWrites, 0);
    assert.equal(fixtureState().headerWrites, 0);
    assert.equal(fixtureState().token.sessionId, "session");
  }
});

test("login waiting for the session lock cannot commit after auth disposal", async context => {
  context.mock.method(console, "log", () => {});
  resetFixture();
  let entered: () => void;
  let release: () => void;
  const started = new Promise<void>(resolve => {
    entered = resolve;
  });
  const waiting = new Promise<void>(resolve => {
    release = resolve;
  });
  configureLock(async operation => {
    entered();
    await waiting;
    return operation();
  });
  await auth.setup();
  const pending = useUserStoreHook().loginByPassword(credentials);
  await started;
  await auth.destroy();
  release();
  assert.equal((await pending).success, false);
  assert.equal(fixtureState().tokenWrites, 0);
});

test("RBAC destroy restores prior routes and discards disposed provider responses", async context => {
  context.mock.method(console, "log", () => {});
  resetFixture();
  const disposePrior = setAsyncRoutesProvider(async () => ({
    success: true,
    data: [{ path: "/prior" }]
  }));
  let finish: (response: any) => void;
  configureRoutes(
    () =>
      new Promise(resolve => {
        finish = resolve;
      })
  );
  await rbac.setup();
  const pending = getAsyncRoutes();
  await rbac.destroy();
  finish({ data: { success: true, data: [{ path: "/stale" }] } });
  assert.deepEqual(await pending, { success: false, data: [] });
  assert.deepEqual(await getAsyncRoutes(), {
    success: true,
    data: [{ path: "/prior" }]
  });
  disposePrior();
  assert.deepEqual(await getAsyncRoutes(), { success: true, data: [] });
});
