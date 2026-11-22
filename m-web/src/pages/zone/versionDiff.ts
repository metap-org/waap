import type { PolicySnapshot } from "../../api/waf";

export type SnapshotDiff = {
  kind: "added" | "removed" | "changed";
  entity: string;
  id: string;
  /** A short name for the record: its name, value or sensitivity, whichever it has. */
  title: string;
  /** For a change: each field that differs, rendered for display. */
  fields: { field: string; before: string; after: string }[];
};

const show = (value: unknown): string =>
  value === undefined || value === null
    ? "—"
    : typeof value === "string"
      ? value
      : JSON.stringify(value);

function titleOf(data: Record<string, unknown>): string {
  for (const key of ["name", "value", "pathPrefix", "sensitivity"]) {
    const v = data[key];
    if (typeof v === "string" && v) return v;
  }
  return "";
}

/** What differs between two snapshots of the same zone, matched by record id. A record that a
 *  rollback re-created has a new id, so it reads as removed and added rather than changed. */
export function diffSnapshots(
  before: PolicySnapshot,
  after: PolicySnapshot,
): SnapshotDiff[] {
  const out: SnapshotDiff[] = [];
  for (const entity of new Set([
    ...Object.keys(before),
    ...Object.keys(after),
  ])) {
    const was = new Map((before[entity] ?? []).map((r) => [r.id, r.data]));
    const now = new Map((after[entity] ?? []).map((r) => [r.id, r.data]));
    for (const [id, data] of now) {
      const old = was.get(id);
      if (!old) {
        out.push({
          kind: "added",
          entity,
          id,
          title: titleOf(data),
          fields: [],
        });
        continue;
      }
      const fields = [...new Set([...Object.keys(old), ...Object.keys(data)])]
        .filter((f) => JSON.stringify(old[f]) !== JSON.stringify(data[f]))
        .map((f) => ({ field: f, before: show(old[f]), after: show(data[f]) }));
      if (fields.length > 0) {
        out.push({ kind: "changed", entity, id, title: titleOf(data), fields });
      }
    }
    for (const [id, data] of was) {
      if (!now.has(id)) {
        out.push({
          kind: "removed",
          entity,
          id,
          title: titleOf(data),
          fields: [],
        });
      }
    }
  }
  return out;
}
