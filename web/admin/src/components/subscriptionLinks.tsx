import { useState } from "react";
import { Check, Clipboard } from "lucide-react";
import type { AdminConfig, Subscription } from "../types";
import type { T } from "../shared";
import { copyText, publicURL } from "../utils";

export function SubscriptionLinks({ config, t }: { config: AdminConfig; t: T }) {
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

export function SubLink({ sub, baseURL, t, compact = false }: { sub: Subscription; baseURL: string; t: T; compact?: boolean }) {
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
