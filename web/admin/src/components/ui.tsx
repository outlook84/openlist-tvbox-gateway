import React, { useState } from "react";
import { ChevronDown, ChevronRight, CircleHelp, Languages, Plus, Trash2 } from "lucide-react";
import { languageNames, type Language } from "../i18n";
import type { T } from "../shared";

export function Field({ label, help, children }: { label: string; help?: string; children: React.ReactNode }) {
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

export function HelpTip({ text }: { text: string }) {
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

export function PanelHeader({ onAdd, t }: { onAdd: () => void; t: T }) {
  return (
    <div className="panel-head">
      <button type="button" className="small" onClick={onAdd}>
        <Plus size={16} /> {t("add")}
      </button>
    </div>
  );
}

export function CollapsibleItem({
  title,
  onRemove,
  actions,
  defaultOpen = false,
  t,
  children,
}: {
  title: string;
  onRemove: () => void;
  actions?: React.ReactNode;
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
        <div className="item-actions">
          {actions}
          <button type="button" className="icon danger" aria-label={t("remove")} title={t("remove")} onClick={onRemove}>
            <Trash2 size={16} />
          </button>
        </div>
      </div>
      {open && children}
    </article>
  );
}

export function LanguageSelect({ language, onChange, t }: { language: Language; onChange: (language: Language) => void; t: T }) {
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

export function Status({ message, error }: { message: string; error: string }) {
  return <div className={error ? "status error" : "status ok"}>{error || message}</div>;
}
