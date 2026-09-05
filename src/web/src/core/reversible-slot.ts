/** A local reversible setter, not a provider discovery or dependency registry. */
export function createReversibleSlot<T>(fallback: T) {
  let entries: readonly { key: symbol; value: T }[] = [];
  let version = 0;
  return {
    get: (): T =>
      entries.length ? entries[entries.length - 1].value : fallback,
    get version(): number {
      return version;
    },
    set(value: T): () => void {
      const entry = { key: Symbol("registration"), value };
      entries = [...entries, entry];
      version++;
      return () => {
        if (!entries.some(candidate => candidate.key === entry.key)) return;
        const wasCurrent = entries[entries.length - 1].key === entry.key;
        entries = entries.filter(candidate => candidate.key !== entry.key);
        if (wasCurrent) version++;
      };
    }
  };
}
