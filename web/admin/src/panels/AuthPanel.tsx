import React, { useState } from "react";
import { TvMinimalPlay } from "lucide-react";
import { login, setupAdmin } from "../api";
import type { Language } from "../i18n";
import type { T } from "../shared";
import { localizeError } from "../errors";
import { Field, LanguageSelect } from "../components/ui";

export function AuthPanel({
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
            <TvMinimalPlay size={32} />
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
