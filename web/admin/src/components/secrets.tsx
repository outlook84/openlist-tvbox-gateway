import { useState } from "react";
import { Pencil, RotateCcw, Trash2 } from "lucide-react";
import type { SecretAction, Subscription } from "../types";
import type { T } from "../shared";
import { HelpTip } from "./ui";

export function SecretHashField({ sub, onChange, t }: { sub: Subscription; onChange: (patch: Partial<Subscription>) => void; t: T }) {
  const [editing, setEditing] = useState(false);
  const action = sub.access_code_hash_action || "keep";
  const saved = Boolean(sub.access_code_hash_set);
  const set = action === "clear" ? false : saved;
  const normalAction = saved ? "keep" : "clear";
  const canReset = editing || Boolean(sub.access_code) || action !== normalAction;
  const resetAction = saved ? "keep" : "clear";

  function resetDraft() {
    onChange({ access_code_hash_action: resetAction, access_code: "" });
    setEditing(false);
  }

  return (
    <div className="access-code-row">
      <div>
        <span className="field-label">
          <span className="label">{t("subscriptionAccessCode")}</span>
          <HelpTip text={t("helpSubscriptionAccessCode")} />
        </span>
        <span className="muted">{set ? t("set") : t("notSet")}</span>
      </div>
      {editing ? (
        <input
          type="password"
          inputMode="numeric"
          autoComplete="new-password"
          name={`subscription-access-code-${sub.id || "new"}`}
          value={sub.access_code || ""}
          onChange={(event) => onChange({ access_code: event.target.value, access_code_hash_action: "replace" })}
          placeholder={t("newSubscriptionAccessCode")}
          autoFocus
        />
      ) : (
        <div className="secret-placeholder" aria-hidden="true" />
      )}
      <SecretPendingActions
        action={action}
        editing={editing}
        canReset={canReset}
        canDelete={saved}
        onEdit={() => setEditing(true)}
        onKeep={resetDraft}
        onClear={() => onChange({ access_code_hash_action: "clear", access_code: "" })}
        t={t}
      />
    </div>
  );
}

export function SecretField({
  label,
  inputName,
  set,
  action,
  value,
  onAction,
  onValue,
  t,
}: {
  label: string;
  inputName: string;
  set: boolean;
  action: SecretAction;
  value: string;
  onAction: (action: SecretAction) => void;
  onValue: (value: string) => void;
  t: T;
}) {
  const [editing, setEditing] = useState(false);
  const normalAction = set ? "keep" : "clear";
  const canReset = editing || Boolean(value) || action !== normalAction;

  function resetDraft() {
    onAction(normalAction);
    setEditing(false);
  }

  function clearSecret() {
    onAction("clear");
    setEditing(false);
  }

  return (
    <div className="secret-row">
      <div>
        <span className="label">{label}</span>
        <span className="muted">{action === "clear" ? t("notSet") : set ? t("set") : t("notSet")}</span>
      </div>
      {editing ? (
        <input
          type="password"
          value={value}
          onChange={(event) => onValue(event.target.value)}
          placeholder={t("newSecret")}
          autoComplete="new-password"
          name={inputName}
          autoFocus
        />
      ) : (
        <div className="secret-placeholder" aria-hidden="true" />
      )}
      <SecretPendingActions
        action={action}
        editing={editing}
        canReset={canReset}
        canDelete={set}
        showClear={false}
        onEdit={() => setEditing(true)}
        onKeep={resetDraft}
        onClear={clearSecret}
        t={t}
      />
    </div>
  );
}

export function SecretPendingActions({
  action,
  editing = true,
  canReset,
  canDelete,
  showClear = true,
  onEdit,
  onKeep,
  onClear,
  t,
}: {
  action: SecretAction;
  editing?: boolean;
  canReset: boolean;
  canDelete: boolean;
  showClear?: boolean;
  onEdit?: () => void;
  onKeep: () => void;
  onClear: () => void;
  t: T;
}) {
  return (
    <div className="pending-actions">
      {editing ? (
        <button type="button" className="icon" aria-label={t("resetDraft")} title={t("resetDraft")} disabled={!canReset} onClick={onKeep}>
          <RotateCcw size={16} />
        </button>
      ) : (
        <button type="button" className="icon" aria-label={t("edit")} title={t("edit")} onClick={onEdit}>
          <Pencil size={16} />
        </button>
      )}
      {showClear && (
        <button type="button" className={action === "clear" && canDelete ? "icon danger active" : "icon danger"} aria-label={t("clear")} title={t("clear")} disabled={!canDelete} onClick={onClear}>
          <Trash2 size={16} />
        </button>
      )}
    </div>
  );
}
