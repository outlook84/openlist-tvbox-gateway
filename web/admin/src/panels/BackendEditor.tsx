import { useState } from "react";
import { PlugZap } from "lucide-react";
import { testBackend } from "../api";
import type { Backend } from "../types";
import type { EditorProps } from "../shared";
import { normalizeBackend } from "../configState";
import { localizeError } from "../errors";
import { uniqueID } from "../utils";
import { useStableRowKeys } from "../hooks";
import { CollapsibleItem, Field, PanelHeader, Status } from "../components/ui";
import { SecretField } from "../components/secrets";

export function BackendEditor({ config, setConfig, t }: EditorProps) {
  const backendRows = useStableRowKeys("backend-row", config.backends.length);
  const [newBackendRows, setNewBackendRows] = useState<Set<string>>(() => new Set());
  const [testingBackend, setTestingBackend] = useState("");
  const [backendStatus, setBackendStatus] = useState<Record<string, { message: string; error: string }>>({});

  function updateBackend(index: number, patch: Partial<Backend>) {
    setConfig((current) => ({
      ...current,
      backends: current.backends.map((backend, i) => (i === index ? normalizeBackend({ ...backend, ...patch }) : backend)),
    }));
  }

  function addBackend() {
    const id = uniqueID("backend", config.backends.map((item) => item.id));
    const rowKey = backendRows.add();
    setNewBackendRows((current) => new Set(current).add(rowKey));
    setConfig((current) => ({
      ...current,
      backends: [...current.backends, { id, server: "https://openlist.example.com", auth_type: "anonymous", version: "v3" }],
    }));
  }

  function removeBackend(index: number) {
    const rowKey = backendRows.keys[index];
    backendRows.remove(index);
    setNewBackendRows((current) => {
      const next = new Set(current);
      next.delete(rowKey);
      return next;
    });
    setConfig((current) => ({ ...current, backends: current.backends.filter((_, i) => i !== index) }));
  }

  async function handleTestBackend(rowKey: string, backend: Backend) {
    setTestingBackend(rowKey);
    setBackendStatus((current) => ({ ...current, [rowKey]: { message: "", error: "" } }));
    try {
      await testBackend(backend);
      setBackendStatus((current) => ({ ...current, [rowKey]: { message: t("backendTestPassed"), error: "" } }));
    } catch (err) {
      setBackendStatus((current) => ({ ...current, [rowKey]: { message: "", error: localizeError(err, t) } }));
    } finally {
      setTestingBackend("");
    }
  }

  return (
    <section className="panel">
      <PanelHeader onAdd={addBackend} t={t} />
      {config.backends.map((backend, index) => {
        const rowKey = backendRows.keys[index];
        const status = backendStatus[rowKey];
        return (
        <CollapsibleItem
          title={backend.id || t("backend")}
          onRemove={() => removeBackend(index)}
          actions={
            <button type="button" className="small" disabled={testingBackend === rowKey} onClick={() => handleTestBackend(rowKey, backend)}>
              <PlugZap size={16} /> {testingBackend === rowKey ? t("testing") : t("test")}
            </button>
          }
          defaultOpen={newBackendRows.has(rowKey)}
          t={t}
          key={rowKey}
        >
          <div className="form-grid">
            <Field label={t("id")} help={t("helpBackendID")}>
              <input value={backend.id} onChange={(event) => updateBackend(index, { id: event.target.value })} autoComplete="off" name={`backend-id-${backend.id || index}`} />
            </Field>
            <Field label={t("server")} help={t("helpBackendServer")}>
              <input value={backend.server} onChange={(event) => updateBackend(index, { server: event.target.value })} autoComplete="off" name={`backend-server-${backend.id || index}`} />
            </Field>
            <Field label={t("auth")} help={t("helpBackendAuth")}>
              <select value={backend.auth_type || "anonymous"} onChange={(event) => updateBackend(index, { auth_type: event.target.value as Backend["auth_type"] })}>
                <option value="anonymous">{t("anonymous")}</option>
                <option value="api_key">{t("apiKey")}</option>
                <option value="password">{t("password")}</option>
              </select>
            </Field>
            <Field label={t("version")} help={t("helpBackendVersion")}>
              <input value={backend.version || "v3"} onChange={(event) => updateBackend(index, { version: event.target.value })} autoComplete="off" name={`backend-version-${backend.id || index}`} />
            </Field>
          </div>
          {backend.auth_type === "api_key" && (
            <SecretField
              label={t("apiKey")}
              inputName={`backend-api-key-${backend.id || index}`}
              set={Boolean(backend.api_key_set)}
              action={backend.api_key_action || "keep"}
              value={backend.api_key || ""}
              onAction={(action) => updateBackend(index, { api_key_action: action, api_key: action === "replace" ? backend.api_key || "" : "" })}
              onValue={(value) => updateBackend(index, { api_key: value, api_key_action: "replace" })}
              t={t}
            />
          )}
          {backend.auth_type === "password" && (
            <>
              <Field label={t("user")} help={t("helpBackendUser")}>
                <input
                  value={backend.user || ""}
                  onChange={(event) => updateBackend(index, { user: event.target.value })}
                  autoComplete="off"
                  name={`backend-user-${backend.id || index}`}
                />
              </Field>
              <SecretField
                label={t("password")}
                inputName={`backend-password-${backend.id || index}`}
                set={Boolean(backend.password_set)}
                action={backend.password_action || "keep"}
                value={backend.password || ""}
                onAction={(action) => updateBackend(index, { password_action: action, password: action === "replace" ? backend.password || "" : "" })}
                onValue={(value) => updateBackend(index, { password: value, password_action: "replace" })}
                t={t}
              />
            </>
          )}
          {status && (status.message || status.error) && <Status message={status.message} error={status.error} />}
        </CollapsibleItem>
        );
      })}
    </section>
  );
}
