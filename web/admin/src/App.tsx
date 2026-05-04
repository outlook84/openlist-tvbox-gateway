import { useCallback, useEffect, useMemo, useState } from "react";
import { Check, LogOut, Save, TvMinimalPlay } from "lucide-react";
import { getConfig, getSession, logout, onAuthExpired, saveConfig, validateConfig } from "./api";
import { APIError, type AdminConfig, type SessionState } from "./types";
import { detectLanguage, saveLanguage, translate, type Language } from "./i18n";
import type { T } from "./shared";
import { emptyConfig, normalizeConfig } from "./configState";
import { localizeError } from "./errors";
import { LanguageSelect, Status } from "./components/ui";
import { AuthPanel } from "./panels/AuthPanel";
import { OverviewPanel } from "./panels/OverviewPanel";
import { BackendEditor } from "./panels/BackendEditor";
import { SubscriptionEditor } from "./panels/SubscriptionEditor";
import { LogsPanel } from "./panels/LogsPanel";

type AdminTab = "overview" | "backends" | "subscriptions" | "logs";
export function App() {
  const [session, setSession] = useState<SessionState | null>(null);
  const [config, setConfig] = useState<AdminConfig>(emptyConfig);
  const [language, setLanguage] = useState<Language>(() => detectLanguage());
  const [message, setMessage] = useState("");
  const [error, setError] = useState("");
  const [loading, setLoading] = useState(true);
  const [activeTab, setActiveTab] = useState<AdminTab>("overview");
  const t: T = useMemo(() => (key) => translate(language, key), [language]);

  function changeLanguage(next: Language) {
    setLanguage(next);
    saveLanguage(next);
  }

  const load = useCallback(async () => {
    setError("");
    setLoading(true);
    try {
      const nextSession = await getSession();
      setSession(nextSession);
      if (nextSession.authenticated) {
        const nextConfig = await getConfig();
        setConfig(normalizeConfig(nextConfig));
      }
    } catch (err) {
      setError(localizeError(err, t));
    } finally {
      setLoading(false);
    }
  }, [t]);

  useEffect(() => {
    void Promise.resolve().then(load);
  }, [load]);

  useEffect(() => {
    return onAuthExpired(() => {
      setSession({ authenticated: false, setup_required: false });
      setConfig(emptyConfig);
      setMessage("");
      setError("");
      setLoading(false);
    });
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
        <div className="brand-title">
          <TvMinimalPlay size={30} />
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
          <button type="button" role="tab" aria-selected={activeTab === "logs"} className={activeTab === "logs" ? "tab active" : "tab"} onClick={() => setActiveTab("logs")}>
            {t("logs")}
          </button>
        </div>
        <div className={`tab-panel active-${activeTab}`} role="tabpanel">
          {activeTab === "overview" && <OverviewPanel config={config} setConfig={setConfig} t={t} />}
          {activeTab === "backends" && <BackendEditor config={config} setConfig={setConfig} t={t} />}
          {activeTab === "subscriptions" && <SubscriptionEditor config={config} setConfig={setConfig} t={t} />}
          {activeTab === "logs" && <LogsPanel t={t} />}
        </div>
      </section>
    </main>
  );
}
