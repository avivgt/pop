// Jest mock for @patternfly/react-icons.
//
// Rather than maintain a hand-written allowlist (which silently breaks the
// moment a component imports an icon that isn't listed), this returns a stub
// component for ANY named export. Each name resolves to a stable no-op React
// component so tests can render real components without pulling in the large
// icon barrel.

const makeIcon = (name: string) => {
  const fn = () => null;
  (fn as unknown as { displayName: string }).displayName = name;
  return fn;
};

const cache: Record<string, unknown> = {};

const handler: ProxyHandler<Record<string, unknown>> = {
  get: (_target, prop: string | symbol) => {
    // Let the interop layer treat this as an ES module namespace.
    if (prop === '__esModule') {
      return true;
    }
    const key = String(prop);
    if (!(key in cache)) {
      cache[key] = makeIcon(key);
    }
    return cache[key];
  },
};

module.exports = new Proxy({}, handler);
