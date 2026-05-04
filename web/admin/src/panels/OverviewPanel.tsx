import { useState } from "react";
import { ShieldCheck } from "lucide-react";
import { updateAdminAccessCode } from "../api";
import type { EditorProps } from "../shared";
import { updateConfig, updateTVBox } from "../configState";
import { localizeError } from "../errors";
import { parseOptionalInt } from "../utils";
import { Field, HelpTip, Status } from "../components/ui";
import { SubscriptionLinks } from "../components/subscriptionLinks";

export function OverviewPanel({ config, setConfig, t }: EditorProps) {
  const tvbox = config.tvbox || {};
  const [currentAdminCode, setCurrentAdminCode] = useState("");
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
      await updateAdminAccessCode(currentAdminCode, newAdminCode);
      setCurrentAdminCode("");
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
            <input value={config.public_base_url || ""} onChange={(event) => updateConfig(setConfig, { public_base_url: event.target.value })} autoComplete="off" name="public-base-url" />
          </Field>
          <label className="check-row">
            <input
              type="checkbox"
              checked={Boolean(config.trust_forwarded_headers || config.trust_x_forwarded_for)}
              onChange={(event) => updateConfig(setConfig, { trust_forwarded_headers: event.target.checked, trust_x_forwarded_for: undefined })}
            />
            <span>{t("trustForwarded")}</span>
            <HelpTip text={t("helpTrustForwarded")} />
          </label>
        </div>
        <div className="form-grid">
          <Field label={t("defaultSiteKey")} help={t("helpDefaultSiteKey")}>
            <input value={tvbox.site_key || ""} onChange={(event) => updateTVBox(setConfig, { site_key: event.target.value })} placeholder="openlist_tvbox" autoComplete="off" name="default-site-key" />
          </Field>
          <Field label={t("defaultSiteName")} help={t("helpDefaultSiteName")}>
            <input value={tvbox.site_name || ""} onChange={(event) => updateTVBox(setConfig, { site_name: event.target.value })} placeholder="OpenList" autoComplete="off" name="default-site-name" />
          </Field>
          <Field label={t("timeoutSeconds")} help={t("helpTimeoutSeconds")}>
            <input
              type="number"
              min="1"
              value={tvbox.timeout || ""}
              onChange={(event) => updateTVBox(setConfig, { timeout: parseOptionalInt(event.target.value) })}
              placeholder="15"
              autoComplete="off"
              name="default-timeout"
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
          <Field label={t("currentAdminAccessCode")} help={t("helpAdminAccessCode")}>
            <input type="password" value={currentAdminCode} onChange={(event) => setCurrentAdminCode(event.target.value)} autoComplete="current-password" />
          </Field>
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
