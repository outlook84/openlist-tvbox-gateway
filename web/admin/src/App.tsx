import { useEffect, useMemo, useRef, useState } from "react";
import { Check, ChevronDown, ChevronRight, CircleHelp, Clipboard, Languages, LogOut, Plus, RotateCcw, Save, ShieldCheck, Trash2 } from "lucide-react";
import { getConfig, getMeta, getSession, login, logout, saveConfig, setupAdmin, updateAdminAccessCode, validateConfig } from "./api";
import { APIError, type AdminConfig, type Backend, type ConfigMeta, type ErrorParams, type Mount, type SecretAction, type SessionState, type Subscription } from "./types";
import { detectLanguage, languageNames, saveLanguage, translate, type Language, type MessageKey } from "./i18n";

const emptyConfig: AdminConfig = { backends: [], subs: [], tvbox: {} };
type T = (key: MessageKey) => string;
type AdminTab = "overview" | "backends" | "subscriptions";

export function App() {
  const [session, setSession] = useState<SessionState | null>(null);
  const [meta, setMeta] = useState<ConfigMeta | null>(null);
  const [config, setConfig] = useState<AdminConfig>(emptyConfig);
  const [language, setLanguage] = useState<Language>(() => detectLanguage());
  const [message, setMessage] = useState("");
  const [error, setError] = useState("");
  const [loading, setLoading] = useState(true);
  const [activeTab, setActiveTab] = useState<AdminTab>("overview");
  const t: T = (key) => translate(language, key);

  function changeLanguage(next: Language) {
    setLanguage(next);
    saveLanguage(next);
  }

  async function load() {
    setError("");
    setLoading(true);
    try {
      const nextSession = await getSession();
      setSession(nextSession);
      if (nextSession.authenticated) {
        const [nextMeta, nextConfig] = await Promise.all([getMeta(), getConfig()]);
        setMeta(nextMeta);
        setConfig(normalizeConfig(nextConfig));
      }
    } catch (err) {
      setError(localizeError(err, t));
    } finally {
      setLoading(false);
    }
  }

  useEffect(() => {
    void load();
  }, []);

  async function handleValidate() {
    setMessage("");
    setError("");
    try {
      const result = await validateConfig(config);
      if (result.valid) {
        setMessage(t("configValid"));
      } else {
        setError(result.error ? localizeError(new APIError(result.error, result.error_code, result.error_params), t, "config") : t("configInvalid"));
      }
    } catch (err) {
      setError(localizeError(err, t));
    }
  }

  async function handleSave() {
    setMessage("");
    setError("");
    try {
      await saveConfig(config);
      setMessage(t("configSaved"));
      const nextConfig = await getConfig();
      setConfig(normalizeConfig(nextConfig));
    } catch (err) {
      setError(localizeError(err, t));
    }
  }

  async function handleLogout() {
    await logout();
    setSession({ authenticated: false, setup_required: false });
    setConfig(emptyConfig);
  }

  if (loading) {
    return <div className="screen center">{t("loading")}</div>;
  }

  if (!session?.authenticated) {
    return <AuthPanel setupRequired={Boolean(session?.setup_required)} onDone={load} t={t} language={language} onLanguageChange={changeLanguage} />;
  }

  return (
    <main className="app-shell">
      <header className="topbar">
        <div>
          <h1>{t("adminDashboard")}</h1>
        </div>
        <div className="actions">
          <LanguageSelect language={language} onChange={changeLanguage} t={t} />
          <button type="button" onClick={handleValidate}>
            <Check size={18} /> <span>{t("validate")}</span>
          </button>
          <button type="button" className="primary" onClick={handleSave}>
            <Save size={18} /> <span>{t("save")}</span>
          </button>
          <button type="button" className="icon" aria-label={t("logOut")} title={t("logOut")} onClick={handleLogout}>
            <LogOut size={18} />
          </button>
        </div>
      </header>

      {(message || error) && <Status message={message} error={error} />}

      <section className="workspace">
        <div className="tabs" role="tablist" aria-label={t("editorSections")}>
          <button type="button" role="tab" aria-selected={activeTab === "overview"} className={activeTab === "overview" ? "tab active" : "tab"} onClick={() => setActiveTab("overview")}>
            {t("overview")}
          </button>
          <button type="button" role="tab" aria-selected={activeTab === "backends"} className={activeTab === "backends" ? "tab active" : "tab"} onClick={() => setActiveTab("backends")}>
            {t("backends")}
          </button>
          <button type="button" role="tab" aria-selected={activeTab === "subscriptions"} className={activeTab === "subscriptions" ? "tab active" : "tab"} onClick={() => setActiveTab("subscriptions")}>
            {t("subscriptions")}
          </button>
        </div>
        <div className={`tab-panel active-${activeTab}`} role="tabpanel">
          {activeTab === "overview" && <OverviewPanel config={config} setConfig={setConfig} t={t} />}
          {activeTab === "backends" && <BackendEditor config={config} setConfig={setConfig} t={t} />}
          {activeTab === "subscriptions" && <SubscriptionEditor config={config} setConfig={setConfig} t={t} />}
        </div>
      </section>
    </main>
  );
}

function OverviewPanel({ config, setConfig, t }: EditorProps) {
  const tvbox = config.tvbox || {};
  const [newAdminCode, setNewAdminCode] = useState("");
  const [confirmAdminCode, setConfirmAdminCode] = useState("");
  const [adminCodeMessage, setAdminCodeMessage] = useState("");
  const [adminCodeError, setAdminCodeError] = useState("");

  async function saveAdminCode() {
    setAdminCodeMessage("");
    setAdminCodeError("");
    if (newAdminCode !== confirmAdminCode) {
      setAdminCodeError(t("adminAccessCodeMismatch"));
      return;
    }
    try {
      await updateAdminAccessCode(newAdminCode);
      setNewAdminCode("");
      setConfirmAdminCode("");
      setAdminCodeMessage(t("adminAccessCodeSaved"));
    } catch (err) {
      setAdminCodeError(localizeError(err, t));
    }
  }

  return (
    <section className="panel overview-panel">
      <SubscriptionLinks config={config} t={t} />
      <section className="settings-panel">
        <h2>{t("topLevelSettings")}</h2>
        <div className="settings-grid">
          <Field label={t("publicBaseURL")} help={t("helpPublicBaseURL")}>
            <input value={config.public_base_url || ""} onChange={(event) => updateConfig(setConfig, { public_base_url: event.target.value })} />
          </Field>
          <label className="check-row">
            <input
              type="checkbox"
              checked={Boolean(config.trust_x_forwarded_for)}
              onChange={(event) => updateConfig(setConfig, { trust_x_forwarded_for: event.target.checked })}
            />
            <span>{t("trustForwarded")}</span>
            <HelpTip text={t("helpTrustForwarded")} />
          </label>
        </div>
        <div className="form-grid">
          <Field label={t("defaultSiteKey")} help={t("helpDefaultSiteKey")}>
            <input value={tvbox.site_key || ""} onChange={(event) => updateTVBox(setConfig, { site_key: event.target.value })} placeholder="openlist_tvbox" />
          </Field>
          <Field label={t("defaultSiteName")} help={t("helpDefaultSiteName")}>
            <input value={tvbox.site_name || ""} onChange={(event) => updateTVBox(setConfig, { site_name: event.target.value })} placeholder="OpenList" />
          </Field>
          <Field label={t("timeoutSeconds")} help={t("helpTimeoutSeconds")}>
            <input
              type="number"
              min="1"
              value={tvbox.timeout || ""}
              onChange={(event) => updateTVBox(setConfig, { timeout: parseOptionalInt(event.target.value) })}
              placeholder="15"
            />
          </Field>
        </div>
        <div className="toggles">
          <label>
            <input type="checkbox" checked={tvbox.searchable !== 0} onChange={(event) => updateTVBox(setConfig, { searchable: event.target.checked ? 1 : 0 })} />{" "}
            <span>{t("searchable")}</span>
            <HelpTip text={t("helpSearchable")} />
          </label>
          <label>
            <input type="checkbox" checked={tvbox.quick_search === 1} onChange={(event) => updateTVBox(setConfig, { quick_search: event.target.checked ? 1 : 0 })} />{" "}
            <span>{t("quickSearch")}</span>
            <HelpTip text={t("helpQuickSearch")} />
          </label>
          <label>
            <input type="checkbox" checked={tvbox.changeable === 1} onChange={(event) => updateTVBox(setConfig, { changeable: event.target.checked ? 1 : 0 })} />{" "}
            <span>{t("changeable")}</span>
            <HelpTip text={t("helpChangeable")} />
          </label>
        </div>
      </section>
      <section className="settings-panel">
        <h2>{t("adminAccessCode")}</h2>
        <div className="form-grid">
          <Field label={t("newAdminAccessCode")} help={t("helpAdminAccessCode")}>
            <input type="password" value={newAdminCode} onChange={(event) => setNewAdminCode(event.target.value)} autoComplete="new-password" />
          </Field>
          <Field label={t("confirmAdminAccessCode")}>
            <input type="password" value={confirmAdminCode} onChange={(event) => setConfirmAdminCode(event.target.value)} autoComplete="new-password" />
          </Field>
        </div>
        {(adminCodeMessage || adminCodeError) && <Status message={adminCodeMessage} error={adminCodeError} />}
        <div className="form-actions">
          <button type="button" className="primary" onClick={saveAdminCode}>
            <ShieldCheck size={18} /> <span>{t("updateAdminAccessCode")}</span>
          </button>
        </div>
      </section>
    </section>
  );
}

function AuthPanel({
  setupRequired,
  onDone,
  t,
  language,
  onLanguageChange,
}: {
  setupRequired: boolean;
  onDone: () => Promise<void>;
  t: T;
  language: Language;
  onLanguageChange: (language: Language) => void;
}) {
  const [setupCode, setSetupCode] = useState("");
  const [accessCode, setAccessCode] = useState("");
  const [setupStep, setSetupStep] = useState<"setup_code" | "access_code">("setup_code");
  const [error, setError] = useState("");

  async function submit(event: React.FormEvent) {
    event.preventDefault();
    setError("");
    if (setupRequired && setupStep === "setup_code") {
      if (!setupCode.trim()) {
        setError(t("setupCodeRequired"));
        return;
      }
      setSetupStep("access_code");
      return;
    }
    try {
      if (setupRequired) {
        await setupAdmin(setupCode, accessCode);
      } else {
        await login(accessCode);
      }
      await onDone();
    } catch (err) {
      setError(localizeError(err, t));
    }
  }

  return (
    <main className="screen center">
      <form className="auth-panel" onSubmit={submit}>
        <div className="auth-title">
          <div className="auth-heading">
            <ShieldCheck size={32} />
            <h1>{setupRequired ? t("createAdminAccess") : t("adminLogin")}</h1>
          </div>
          <LanguageSelect language={language} onChange={onLanguageChange} t={t} />
        </div>
        {setupRequired && setupStep === "setup_code" && (
          <Field label={t("setupCode")}>
            <input
              value={setupCode}
              onChange={(event) => setSetupCode(event.target.value)}
              autoComplete="one-time-code"
              inputMode="numeric"
              autoFocus
            />
          </Field>
        )}
        {(!setupRequired || setupStep === "access_code") && (
          <Field label={setupRequired ? t("newAdminAccessCode") : t("accessCode")}>
            <input
              type="password"
              value={accessCode}
              onChange={(event) => setAccessCode(event.target.value)}
              autoComplete={setupRequired ? "new-password" : "current-password"}
              autoFocus
            />
          </Field>
        )}
        {error && <p className="error-text">{error}</p>}
        <div className="auth-actions">
          {setupRequired && setupStep === "access_code" && (
            <button type="button" onClick={() => setSetupStep("setup_code")}>
              {t("back")}
            </button>
          )}
          <button type="submit" className="primary full">
            {setupRequired && setupStep === "setup_code" ? t("next") : setupRequired ? t("initialize") : t("signIn")}
          </button>
        </div>
      </form>
    </main>
  );
}

function BackendEditor({ config, setConfig, t }: EditorProps) {
  const backendRows = useStableRowKeys("backend-row", config.backends.length);
  const [newBackendRows, setNewBackendRows] = useState<Set<string>>(() => new Set());

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

  return (
    <section className="panel">
      <PanelHeader onAdd={addBackend} t={t} />
      {config.backends.map((backend, index) => (
        <CollapsibleItem title={backend.id || t("backend")} onRemove={() => removeBackend(index)} defaultOpen={newBackendRows.has(backendRows.keys[index])} t={t} key={backendRows.keys[index]}>
          <div className="form-grid">
            <Field label={t("id")} help={t("helpBackendID")}>
              <input value={backend.id} onChange={(event) => updateBackend(index, { id: event.target.value })} />
            </Field>
            <Field label={t("server")} help={t("helpBackendServer")}>
              <input value={backend.server} onChange={(event) => updateBackend(index, { server: event.target.value })} />
            </Field>
            <Field label={t("auth")} help={t("helpBackendAuth")}>
              <select value={backend.auth_type || "anonymous"} onChange={(event) => updateBackend(index, { auth_type: event.target.value as Backend["auth_type"] })}>
                <option value="anonymous">{t("anonymous")}</option>
                <option value="api_key">{t("apiKey")}</option>
                <option value="password">{t("password")}</option>
              </select>
            </Field>
            <Field label={t("version")} help={t("helpBackendVersion")}>
              <input value={backend.version || "v3"} onChange={(event) => updateBackend(index, { version: event.target.value })} />
            </Field>
          </div>
          {backend.auth_type === "api_key" && (
            <SecretField
              label={t("apiKey")}
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
                <input value={backend.user || ""} onChange={(event) => updateBackend(index, { user: event.target.value })} />
              </Field>
              <SecretField
                label={t("password")}
                set={Boolean(backend.password_set)}
                action={backend.password_action || "keep"}
                value={backend.password || ""}
                onAction={(action) => updateBackend(index, { password_action: action, password: action === "replace" ? backend.password || "" : "" })}
                onValue={(value) => updateBackend(index, { password: value, password_action: "replace" })}
                t={t}
              />
            </>
          )}
        </CollapsibleItem>
      ))}
    </section>
  );
}

function SubscriptionEditor({ config, setConfig, t }: EditorProps) {
  const backendIDs = useMemo(() => config.backends.map((backend) => backend.id).filter(Boolean), [config.backends]);
  const subRows = useStableRowKeys("sub-row", config.subs.length);
  const mountRows = useRef<Record<string, string[]>>({});
  const nextMountRowID = useRef(1);
  const [newSubRows, setNewSubRows] = useState<Set<string>>(() => new Set());
  const [newMountRows, setNewMountRows] = useState<Set<string>>(() => new Set());

  function getMountRows(subRowKey: string, length: number) {
    const keys = mountRows.current[subRowKey] || [];
    while (keys.length < length) {
      keys.push(`mount-row-${nextMountRowID.current}`);
      nextMountRowID.current += 1;
    }
    if (keys.length > length) {
      keys.length = length;
    }
    mountRows.current[subRowKey] = keys;
    return keys;
  }

  function updateSub(index: number, patch: Partial<Subscription>) {
    setConfig((current) => ({
      ...current,
      subs: current.subs.map((sub, i) => (i === index ? { ...sub, ...patch } : sub)),
    }));
  }

  function addSub() {
    const id = uniqueID("sub", config.subs.map((item) => item.id));
    const rowKey = subRows.add();
    setNewSubRows((current) => new Set(current).add(rowKey));
    setConfig((current) => ({ ...current, subs: [...current.subs, { id, path: `/sub/${id}`, access_code_hash_action: "clear", mounts: [] }] }));
  }

  function removeSub(index: number) {
    const rowKey = subRows.keys[index];
    subRows.remove(index);
    delete mountRows.current[rowKey];
    setNewSubRows((current) => {
      const next = new Set(current);
      next.delete(rowKey);
      return next;
    });
    setConfig((current) => ({ ...current, subs: current.subs.filter((_, i) => i !== index) }));
  }

  function addMount(subIndex: number) {
    const sub = config.subs[subIndex];
    const id = uniqueID("mount", sub.mounts.map((item) => item.id));
    const subRowKey = subRows.keys[subIndex];
    const mountRowKeys = getMountRows(subRowKey, sub.mounts.length);
    const mountRowKey = `mount-row-${nextMountRowID.current}`;
    nextMountRowID.current += 1;
    mountRowKeys.push(mountRowKey);
    setNewMountRows((current) => new Set(current).add(mountRowKey));
    updateSub(subIndex, {
      mounts: [...sub.mounts, { id, name: id, backend: backendIDs[0] || "", path: "/", search: true, refresh: false, hidden: false }],
    });
  }

  function updateMount(subIndex: number, mountIndex: number, patch: Partial<Mount>) {
    const sub = config.subs[subIndex];
    updateSub(subIndex, {
      mounts: sub.mounts.map((mount, i) => (i === mountIndex ? { ...mount, ...patch } : mount)),
    });
  }

  function removeMount(subIndex: number, mountIndex: number) {
    const sub = config.subs[subIndex];
    const subRowKey = subRows.keys[subIndex];
    const mountRowKeys = getMountRows(subRowKey, sub.mounts.length);
    const mountRowKey = mountRowKeys[mountIndex];
    mountRowKeys.splice(mountIndex, 1);
    setNewMountRows((current) => {
      const next = new Set(current);
      next.delete(mountRowKey);
      return next;
    });
    updateSub(subIndex, { mounts: sub.mounts.filter((_, i) => i !== mountIndex) });
  }

  return (
    <section className="panel">
      <PanelHeader onAdd={addSub} t={t} />
      {config.subs.map((sub, subIndex) => (
        <CollapsibleItem title={sub.id || t("subscription")} onRemove={() => removeSub(subIndex)} defaultOpen={newSubRows.has(subRows.keys[subIndex])} t={t} key={subRows.keys[subIndex]}>
          <SubLink sub={sub} baseURL={config.public_base_url || ""} t={t} />
          <div className="form-grid">
            <Field label={t("id")} help={t("helpSubscriptionID")}>
              <input value={sub.id} onChange={(event) => updateSub(subIndex, { id: event.target.value })} />
            </Field>
            <Field label={t("path")} help={t("helpSubscriptionPath")}>
              <input value={sub.path || ""} onChange={(event) => updateSub(subIndex, { path: event.target.value })} />
            </Field>
            <Field label={t("siteKey")} help={t("helpSiteKey")}>
              <input value={sub.site_key || ""} onChange={(event) => updateSub(subIndex, { site_key: event.target.value })} />
            </Field>
            <Field label={t("siteName")} help={t("helpSiteName")}>
              <input value={sub.site_name || ""} onChange={(event) => updateSub(subIndex, { site_name: event.target.value })} />
            </Field>
          </div>
          <SecretHashField sub={sub} onChange={(patch) => updateSub(subIndex, patch)} t={t} />
          <div className="mount-head">
            <h3>{t("mounts")}</h3>
            <button type="button" className="small" onClick={() => addMount(subIndex)}>
              <Plus size={16} /> {t("mount")}
            </button>
          </div>
          {sub.mounts.map((mount, mountIndex) => {
            const mountRowKeys = getMountRows(subRows.keys[subIndex], sub.mounts.length);
            const mountRowKey = mountRowKeys[mountIndex];
            return (
              <CollapsibleMount
                title={mount.name || mount.id || t("mount")}
                onRemove={() => removeMount(subIndex, mountIndex)}
                defaultOpen={newMountRows.has(mountRowKey)}
                t={t}
                key={mountRowKey}
              >
                <div className="form-grid">
                  <Field label={t("id")} help={t("helpMountID")}>
                    <input value={mount.id} onChange={(event) => updateMount(subIndex, mountIndex, { id: event.target.value })} />
                  </Field>
                  <Field label={t("name")} help={t("helpMountName")}>
                    <input value={mount.name || ""} onChange={(event) => updateMount(subIndex, mountIndex, { name: event.target.value })} />
                  </Field>
                  <Field label={t("backend")} help={t("helpMountBackend")}>
                    <select value={mount.backend} onChange={(event) => updateMount(subIndex, mountIndex, { backend: event.target.value })}>
                      <option value="">{t("selectBackend")}</option>
                      {backendIDs.map((id) => (
                        <option key={id} value={id}>
                          {id}
                        </option>
                      ))}
                    </select>
                  </Field>
                  <Field label={t("path")} help={t("helpMountPath")}>
                    <input value={mount.path} onChange={(event) => updateMount(subIndex, mountIndex, { path: event.target.value })} />
                  </Field>
                </div>
                <div className="toggles">
                  <label><input type="checkbox" checked={mount.search !== false} onChange={(event) => updateMount(subIndex, mountIndex, { search: event.target.checked })} /> <span>{t("search")}</span><HelpTip text={t("helpMountSearch")} /></label>
                  <label><input type="checkbox" checked={Boolean(mount.refresh)} onChange={(event) => updateMount(subIndex, mountIndex, { refresh: event.target.checked })} /> <span>{t("refresh")}</span><HelpTip text={t("helpMountRefresh")} /></label>
                  <label><input type="checkbox" checked={Boolean(mount.hidden)} onChange={(event) => updateMount(subIndex, mountIndex, { hidden: event.target.checked })} /> <span>{t("hidden")}</span><HelpTip text={t("helpMountHidden")} /></label>
                </div>
              </CollapsibleMount>
            );
          })}
        </CollapsibleItem>
      ))}
    </section>
  );
}

function CollapsibleMount({
  title,
  onRemove,
  defaultOpen = false,
  t,
  children,
}: {
  title: string;
  onRemove: () => void;
  defaultOpen?: boolean;
  t: T;
  children: React.ReactNode;
}) {
  const [open, setOpen] = useState(defaultOpen);

  return (
    <article className="mount">
      <div className="mount-item-head">
        <button type="button" className="collapse-toggle" aria-expanded={open} onClick={() => setOpen((current) => !current)}>
          {open ? <ChevronDown size={16} /> : <ChevronRight size={16} />}
          <span>{title}</span>
        </button>
        <button type="button" className="icon danger" aria-label={t("removeMount")} title={t("removeMount")} onClick={onRemove}>
          <Trash2 size={16} />
        </button>
      </div>
      {open && children}
    </article>
  );
}

function SubscriptionLinks({ config, t }: { config: AdminConfig; t: T }) {
  return (
    <section className="links-panel">
      <h2>{t("subscriptionLinks")}</h2>
      <div className="link-list">
        {config.subs.length === 0 && <p className="empty-state">{t("noSubscriptions")}</p>}
        {config.subs.map((sub, index) => (
          <SubLink key={`${sub.id}-${index}`} sub={sub} baseURL={config.public_base_url || ""} t={t} compact />
        ))}
      </div>
    </section>
  );
}

function SubLink({ sub, baseURL, t, compact = false }: { sub: Subscription; baseURL: string; t: T; compact?: boolean }) {
  const [copied, setCopied] = useState(false);
  const path = sub.path || "";
  const url = publicURL(baseURL, path);
  const displayURL = url || path || t("pathNotSet");

  async function copyURL() {
    if (!url) return;
    await copyText(url);
    setCopied(true);
    window.setTimeout(() => setCopied(false), 1400);
  }

  return (
    <div className={compact ? "sub-link compact" : "sub-link"}>
      <div>
        <span className="label">{sub.id || t("subscription")}</span>
        <span className="muted">{sub.mounts.length} {t("mounts")}</span>
      </div>
      <div className="sub-link-url">
        <code>{displayURL}</code>
        <button type="button" className={copied ? "icon active" : "icon"} aria-label={t("copySubscriptionLink")} title={copied ? t("copied") : t("copySubscriptionLink")} onClick={copyURL} disabled={!url}>
          {copied ? <Check size={16} /> : <Clipboard size={16} />}
        </button>
      </div>
    </div>
  );
}

function SecretHashField({ sub, onChange, t }: { sub: Subscription; onChange: (patch: Partial<Subscription>) => void; t: T }) {
  const action = sub.access_code_hash_action || "keep";
  const saved = Boolean(sub.access_code_hash_set);
  const set = action === "clear" ? false : saved;
  const canReset = Boolean(sub.access_code) || (action !== "keep" && saved);

  return (
    <div className="access-code-row">
      <div>
        <span className="field-label">
          <span className="label">{t("subscriptionAccessCode")}</span>
          <HelpTip text={t("helpSubscriptionAccessCode")} />
        </span>
        <span className="muted">{set ? t("set") : t("notSet")}</span>
      </div>
      <input
        type="password"
        inputMode="numeric"
        value={sub.access_code || ""}
        onChange={(event) => onChange({ access_code: event.target.value, access_code_hash_action: "replace" })}
        placeholder={t("newSubscriptionAccessCode")}
      />
      <SecretPendingActions
        action={action}
        canReset={canReset}
        canDelete={saved}
        onKeep={() => onChange({ access_code_hash_action: "keep", access_code: "" })}
        onClear={() => onChange({ access_code_hash_action: "clear", access_code: "" })}
        t={t}
      />
    </div>
  );
}

function SecretField({
  label,
  set,
  action,
  value,
  onAction,
  onValue,
  t,
}: {
  label: string;
  set: boolean;
  action: SecretAction;
  value: string;
  onAction: (action: SecretAction) => void;
  onValue: (value: string) => void;
  t: T;
}) {
  return (
    <div className="secret-row">
      <div>
        <span className="label">{label}</span>
        <span className="muted">{action === "clear" ? t("notSet") : set ? t("set") : t("notSet")}</span>
      </div>
      <input type="password" value={value} onChange={(event) => onValue(event.target.value)} placeholder={t("newSecret")} />
      <SecretPendingActions
        action={action}
        canReset={Boolean(value) || (action !== "keep" && set)}
        canDelete={set}
        onKeep={() => onAction("keep")}
        onClear={() => onAction("clear")}
        t={t}
      />
    </div>
  );
}

function SecretPendingActions({
  action,
  canReset,
  canDelete,
  onKeep,
  onClear,
  t,
}: {
  action: SecretAction;
  canReset: boolean;
  canDelete: boolean;
  onKeep: () => void;
  onClear: () => void;
  t: T;
}) {
  return (
    <div className="pending-actions">
      <button type="button" className="icon" aria-label={t("resetDraft")} title={t("resetDraft")} disabled={!canReset} onClick={onKeep}>
        <RotateCcw size={16} />
      </button>
      <button type="button" className={action === "clear" && canDelete ? "icon danger active" : "icon danger"} aria-label={t("clear")} title={t("clear")} disabled={!canDelete} onClick={onClear}>
        <Trash2 size={16} />
      </button>
    </div>
  );
}

function Field({ label, help, children }: { label: string; help?: string; children: React.ReactNode }) {
  return (
    <label className="field">
      <span className="field-label">
        <span>{label}</span>
        {help && <HelpTip text={help} />}
      </span>
      {children}
    </label>
  );
}

function HelpTip({ text }: { text: string }) {
  const [open, setOpen] = useState(false);
  return (
    <span className="help-tip">
      <button
        type="button"
        className="help-button"
        aria-label={text}
        aria-expanded={open}
        onClick={(event) => {
          event.preventDefault();
          setOpen((current) => !current);
        }}
        onBlur={() => setOpen(false)}
      >
        <CircleHelp size={16} />
      </button>
      <span className={open ? "tooltip open" : "tooltip"} role="tooltip">
        {text}
      </span>
    </span>
  );
}

function PanelHeader({ onAdd, t }: { onAdd: () => void; t: T }) {
  return (
    <div className="panel-head">
      <button type="button" className="small" onClick={onAdd}>
        <Plus size={16} /> {t("add")}
      </button>
    </div>
  );
}

function CollapsibleItem({
  title,
  onRemove,
  defaultOpen = false,
  t,
  children,
}: {
  title: string;
  onRemove: () => void;
  defaultOpen?: boolean;
  t: T;
  children: React.ReactNode;
}) {
  const [open, setOpen] = useState(defaultOpen);

  return (
    <article className="item">
      <div className="item-head">
        <button type="button" className="collapse-toggle" aria-expanded={open} onClick={() => setOpen((current) => !current)}>
          {open ? <ChevronDown size={16} /> : <ChevronRight size={16} />}
          <span>{title}</span>
        </button>
        <button type="button" className="icon danger" aria-label={t("remove")} title={t("remove")} onClick={onRemove}>
          <Trash2 size={16} />
        </button>
      </div>
      {open && children}
    </article>
  );
}

function LanguageSelect({ language, onChange, t }: { language: Language; onChange: (language: Language) => void; t: T }) {
  const [open, setOpen] = useState(false);

  function selectLanguage(next: Language) {
    onChange(next);
    setOpen(false);
  }

  return (
    <div className="language-menu">
      <button type="button" className="icon" aria-label={t("language")} title={t("language")} aria-haspopup="menu" aria-expanded={open} onClick={() => setOpen((current) => !current)}>
        <Languages size={18} />
      </button>
      {open && (
        <div className="menu-popover" role="menu">
          {Object.entries(languageNames).map(([key, name]) => (
            <button
              type="button"
              role="menuitemradio"
              aria-checked={language === key}
              className={language === key ? "menu-item active" : "menu-item"}
              key={key}
              onClick={() => selectLanguage(key as Language)}
            >
              {name}
            </button>
          ))}
        </div>
      )}
    </div>
  );
}

function Status({ message, error }: { message: string; error: string }) {
  return <div className={error ? "status error" : "status ok"}>{error || message}</div>;
}

interface EditorProps {
  config: AdminConfig;
  setConfig: React.Dispatch<React.SetStateAction<AdminConfig>>;
  t: T;
}

function updateConfig(setConfig: EditorProps["setConfig"], patch: Partial<AdminConfig>) {
  setConfig((current) => ({ ...current, ...patch }));
}

function updateTVBox(setConfig: EditorProps["setConfig"], patch: Partial<AdminConfig["tvbox"]>) {
  setConfig((current) => ({ ...current, tvbox: { ...(current.tvbox || {}), ...patch } }));
}

function normalizeConfig(config: AdminConfig): AdminConfig {
  return {
    ...emptyConfig,
    ...config,
    tvbox: config.tvbox || {},
    backends: (config.backends || []).map(normalizeBackend),
    subs: (config.subs || []).map((sub) => ({ ...sub, mounts: sub.mounts || [], access_code: "", access_code_hash_action: sub.access_code_hash_action || "keep" })),
  };
}

function normalizeBackend(backend: Backend): Backend {
  const next = { ...backend, auth_type: backend.auth_type || "anonymous", version: backend.version || "v3" };
  if (next.auth_type !== "api_key") {
    next.api_key = "";
    next.api_key_action = "clear";
  } else {
    next.api_key_action = next.api_key_action || "keep";
  }
  if (next.auth_type !== "password") {
    next.user = "";
    next.password = "";
    next.password_action = "clear";
  } else {
    next.password_action = next.password_action || "keep";
  }
  return next;
}

function useStableRowKeys(prefix: string, length: number) {
  const keys = useRef<string[]>([]);
  const nextID = useRef(1);

  while (keys.current.length < length) {
    keys.current.push(`${prefix}-${nextID.current}`);
    nextID.current += 1;
  }
  if (keys.current.length > length) {
    keys.current.length = length;
  }

  function add() {
    const key = `${prefix}-${nextID.current}`;
    nextID.current += 1;
    keys.current.push(key);
    return key;
  }

  function remove(index: number) {
    keys.current.splice(index, 1);
  }

  return { keys: keys.current, add, remove };
}

function uniqueID(prefix: string, existing: string[]) {
  let index = existing.length + 1;
  let id = `${prefix}${index}`;
  while (existing.includes(id)) {
    index += 1;
    id = `${prefix}${index}`;
  }
  return id;
}

function localizeError(err: unknown, t: T, context: "request" | "config" = "request"): string {
  const message = typeof err === "string" ? err : err instanceof Error ? err.message : "";
  const code = err instanceof APIError ? err.code : undefined;
  const params = err instanceof APIError ? err.params : undefined;
  if (code) {
    return localizeErrorCode(code, params, message, t, context);
  }
  if (!message.trim()) {
    return t("requestFailed");
  }
  if (message.toLowerCase().startsWith("http ")) {
    return `${t("requestFailed")} (${message})`;
  }
  if (context === "config") {
    return `${t("configInvalid")} ${message}`;
  }
  return message;
}

function localizeErrorCode(code: string, params: ErrorParams | undefined, message: string, t: T, context: "request" | "config") {
  const p = params || {};
  const backend = stringParam(p, "backend_id");
  const sub = stringParam(p, "sub_id");
  const mount = stringParam(p, "mount_id");
  const index = stringParam(p, "index");
  const path = stringParam(p, "path");
  const siteKey = stringParam(p, "site_key");
  const env = stringParam(p, "env");
  const reserved = stringParam(p, "reserved");
  const secret = stringParam(p, "secret");
  switch (code) {
    case "auth.unauthorized":
      return t("errorUnauthorized");
    case "admin.setup_required":
      return t("errorAdminSetupRequired");
    case "admin.already_initialized":
      return t("errorAdminAlreadyInitialized");
    case "auth.too_many_setup_attempts":
      return t("errorTooManySetupAttempts");
    case "auth.too_many_login_attempts":
      return t("errorTooManyLoginAttempts");
    case "request.invalid_json":
      return t("errorInvalidRequest");
    case "admin.setup_failed":
      return t("errorAdminSetupFailed");
    case "admin.session_failed":
      return t("errorAdminSessionFailed");
    case "admin.access_code.update_failed":
      return t("errorAdminAccessCodeUpdateFailed");
    case "config.load_failed":
      return t("errorConfigLoadFailed");
    case "config.save_failed":
      return t("errorConfigSaveFailed");
    case "admin.access_code.invalid":
      return adminAccessCodeReason(message, t);
    case "subscription.access_code.invalid":
      return withScope(t("subscription"), sub, t("errorNumericAccessCode"));
    case "subscription.access_code_hash.invalid":
      return withScope(t("subscription"), sub, t("errorAccessCodeHashInvalid"));
    case "subscription.access_code.plaintext_unsupported":
      return withScope(t("subscription"), sub, t("errorPlaintextAccessCodeUnsupported"));
    case "secret.keep_missing":
      return scopedSecretError(p, t("errorSecretKeepMissing"), t);
    case "secret.invalid_action":
      return scopedSecretError(p, t("errorSecretInvalidAction"), t);
    case "config.public_base_url.invalid":
      return t("errorPublicBaseURLInvalid");
    case "tvbox.site_key.invalid":
      return t("errorSiteKeyInvalid");
    case "backend.required":
      return t("errorBackendRequired");
    case "backend.id.invalid":
      return withScope(t("backend"), index, t("errorIDInvalid"));
    case "backend.id.duplicate":
      return withScope(t("backend"), backend, t("errorBackendIDDuplicate"));
    case "backend.server.invalid":
      return withScope(t("backend"), backend, t("errorBackendServerInvalid"));
    case "backend.version.invalid":
      return withScope(t("backend"), backend, t("errorBackendVersionInvalid"));
    case "backend.env_secret.unsupported":
      return withScope(t("backend"), backend, t("errorEnvSecretUnsupported"));
    case "backend.auth.credentials_for_anonymous":
      return withScope(t("backend"), backend, t("errorAnonymousCredentials"));
    case "backend.auth.api_key_password_conflict":
      return withScope(t("backend"), backend, t("errorAPIKeyPasswordConflict"));
    case "backend.auth.password_api_key_conflict":
      return withScope(t("backend"), backend, t("errorPasswordAPIKeyConflict"));
    case "backend.secret.multiple_sources":
      return withScope(t("backend"), backend, `${secretLabel(secret, t)}: ${t("errorSecretMultipleSources")}`);
    case "backend.env_secret.missing":
      return withScope(t("backend"), backend, `${env}: ${t("errorEnvSecretMissing")}`);
    case "backend.env_secret.empty":
      return withScope(t("backend"), backend, `${env}: ${t("errorEnvSecretEmpty")}`);
    case "backend.auth.api_key_required":
      return withScope(t("backend"), backend, t("errorAPIKeyRequired"));
    case "backend.auth.user_required":
      return withScope(t("backend"), backend, t("errorBackendUserRequired"));
    case "backend.auth.password_required":
      return withScope(t("backend"), backend, t("errorBackendPasswordRequired"));
    case "backend.auth_type.invalid":
      return withScope(t("backend"), backend, t("errorBackendAuthInvalid"));
    case "subscription.required":
      return t("errorSubscriptionRequired");
    case "subscription.id.invalid":
      return withScope(t("subscription"), index, t("errorIDInvalid"));
    case "subscription.id.duplicate":
      return withScope(t("subscription"), sub, t("errorSubscriptionIDDuplicate"));
    case "subscription.path.invalid":
      return withScope(t("subscription"), sub, t("errorPathInvalid"));
    case "subscription.path.reserved":
      return withScope(t("subscription"), sub, `${path}: ${t("errorReservedPath")} ${reserved}`);
    case "subscription.path.duplicate":
      return `${t("path")} ${path}: ${t("errorPathDuplicate")}`;
    case "subscription.site_key.invalid":
      return withScope(t("subscription"), sub, t("errorSiteKeyInvalid"));
    case "subscription.site_key.duplicate":
      return `${t("siteKey")} ${siteKey}: ${t("errorSiteKeyDuplicate")}`;
    case "subscription.mount.required":
      return withScope(t("subscription"), sub, t("errorMountRequired"));
    case "mount.id.invalid":
      return withMountScope(sub, index, t("errorIDInvalid"), t);
    case "mount.id.duplicate":
      return withMountScope(sub, mount, t("errorMountIDDuplicate"), t);
    case "mount.backend.unknown":
      return withMountScope(sub, mount, `${t("backend")} ${backend}: ${t("errorBackendUnknown")}`, t);
    case "mount.path.invalid":
      return withMountScope(sub, mount, t("errorPathInvalid"), t);
    case "mount.play_headers.invalid":
      return withMountScope(sub, mount, t("errorPlayHeadersInvalid"), t);
    case "subscription.live.url_required":
    case "subscription.live.url_invalid":
    case "subscription.live.epg_invalid":
    case "subscription.live.logo_invalid":
    case "subscription.live.type_invalid":
      return withScope(t("subscription"), sub, `${t("errorLiveInvalid")} ${index}`);
    case "request.cross_origin":
      return t("errorCrossOrigin");
    default:
      return context === "config" ? `${t("configInvalid")} ${message}` : message || t("requestFailed");
  }
}

function stringParam(params: ErrorParams, key: string) {
  const value = params[key];
  return value === undefined ? "" : String(value);
}

function withScope(label: string, id: string, message: string) {
  return id ? `${label} ${id}: ${message}` : message;
}

function withMountScope(sub: string, mount: string, message: string, t: T) {
  const prefix = sub ? `${t("subscription")} ${sub}` : t("subscription");
  return mount ? `${prefix} / ${t("mount")} ${mount}: ${message}` : `${prefix}: ${message}`;
}

function scopedSecretError(params: ErrorParams | undefined, message: string, t: T) {
  const p = params || {};
  const backend = stringParam(p, "backend_id");
  const sub = stringParam(p, "sub_id");
  const secret = secretLabel(stringParam(p, "secret"), t);
  if (backend) return `${t("backend")} ${backend} ${secret}: ${message}`;
  if (sub) return `${t("subscription")} ${sub} ${secret}: ${message}`;
  return message;
}

function secretLabel(secret: string, t: T) {
  if (secret === "api_key") return t("apiKey");
  if (secret === "password") return t("password");
  if (secret === "access_code") return t("accessCode");
  return secret;
}

function adminAccessCodeReason(message: string, t: T) {
  const normalized = message.toLowerCase();
  if (normalized.includes("8 to 64")) return t("errorAdminAccessCodeLength");
  if (normalized.includes("whitespace") || normalized.includes("control")) return t("errorAdminAccessCodeChars");
  return t("errorAdminAccessCodeInvalid");
}

function parseOptionalInt(value: string) {
  if (value.trim() === "") {
    return undefined;
  }
  const parsed = Number.parseInt(value, 10);
  return Number.isFinite(parsed) ? parsed : undefined;
}

function publicURL(baseURL: string, path: string) {
  if (!path) {
    return "";
  }
  const suffix = path.startsWith("/") ? path : `/${path}`;
  const trimmedBase = baseURL.trim();
  if (!trimmedBase) {
    return `${window.location.origin}${suffix}`;
  }
  if (!isAbsoluteHTTPURL(trimmedBase)) {
    return path;
  }
  const base = trimmedBase.replace(/\/+$/, "");
  return `${base}${suffix}`;
}

function isAbsoluteHTTPURL(value: string) {
  try {
    const url = new URL(value);
    return url.protocol === "http:" || url.protocol === "https:";
  } catch {
    return false;
  }
}

async function copyText(text: string) {
  if (navigator.clipboard?.writeText) {
    await navigator.clipboard.writeText(text);
    return;
  }
  const input = document.createElement("textarea");
  input.value = text;
  input.setAttribute("readonly", "");
  input.style.position = "fixed";
  input.style.left = "-9999px";
  document.body.append(input);
  input.select();
  document.execCommand("copy");
  input.remove();
}
