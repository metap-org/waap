/**
 * Change requests (`waf.change_requests`, docs/delivery/management/03-change-requests.md): a list of
 * writes to one zone's policy, or to the tenant-wide records, that reaches the live records only when
 * it is approved. The rows are read like any record; everything that changes one is an action.
 */
import { ApiError, invokeAction } from "@metap/platform-ui";
import { ENTITIES, type WafRecord } from "./waf";

export type ChangeOperation = {
  op: "create" | "update" | "delete";
  entity: string;
  id?: string;
  data?: Record<string, unknown>;
};

export type ChangeDiffEntry = {
  kind: "create" | "update" | "delete";
  entity: string;
  id?: string;
  before?: Record<string, unknown>;
  after?: Record<string, unknown>;
};

export type ChangeCheck = {
  ok: boolean;
  error?: {
    operation: number;
    code: string;
    message?: string;
    fieldErrors?: Record<string, string[]>;
  };
  findings: { severity: string; code: string; subject?: string }[];
};

export type ChangeStatus =
  "draft" | "pendingReview" | "published" | "rejected" | "cancelled";

export type ChangeRequestData = {
  /** Absent for a change to the tenant-wide records. */
  zoneId?: string | null;
  title: string;
  description?: string;
  status: ChangeStatus;
  operations: ChangeOperation[];
  diff?: ChangeDiffEntry[];
  checkResult?: ChangeCheck;
  authorId: string;
  submittedAt?: string;
  decidedBy?: string;
  decidedAt?: string;
  decision?: string;
  decisionComment?: string;
  publishedVersion?: number;
};
export type ChangeRequest = WafRecord<ChangeRequestData>;

export type ChangeOutcome = {
  id: string;
  status: ChangeStatus;
  check?: ChangeCheck;
  publishedVersion?: number;
};

/** The server refused a direct write because the account has change control on. */
export function needsChangeRequest(error: unknown): boolean {
  return error instanceof ApiError && error.code === "change_request_required";
}

export function proposeChange(
  zoneId: string | undefined,
  title: string,
  operations: ChangeOperation[],
) {
  const input = { title, operations };
  return zoneId
    ? invokeAction<ChangeOutcome>(ENTITIES.zones, "createChangeRequest", {
        id: zoneId,
        input,
      })
    : invokeAction<ChangeOutcome>(
        ENTITIES.changeRequests,
        "createTenantChangeRequest",
        { input },
      );
}

export function changeAction(
  id: string,
  action: "revise" | "submit" | "approve" | "reject" | "cancel",
  input?: Record<string, unknown>,
) {
  return invokeAction<ChangeOutcome>(ENTITIES.changeRequests, action, {
    id,
    input,
  });
}

/** What an operation or a diff entry is about, as a person names it. */
export function subjectOf(
  data: Record<string, unknown> | undefined,
  id?: string,
): string {
  const named = data?.name ?? data?.value;
  if (typeof named === "string" && named) return named;
  return id ? id.slice(0, 8) : "";
}
