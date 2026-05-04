import React, { useMemo, useState } from "react";
import { ChevronDown, ChevronRight, Plus, Trash2 } from "lucide-react";
import type { Live, Mount, Subscription } from "../types";
import type { EditorProps } from "../shared";
import { formatStringMap, parseOptionalInt, parseStringMapDraft, uniqueID } from "../utils";
import { useStableRowKeys } from "../hooks";
import { CollapsibleItem, Field, HelpTip, PanelHeader } from "../components/ui";
import { SecretHashField } from "../components/secrets";
import { SubLink } from "../components/subscriptionLinks";

type NestedRowKeysState = {
  keysByParent: Record<string, string[]>;
  nextID: number;
};

function normalizeNestedRowKeys(prefix: string, state: NestedRowKeysState, parentLengths: Array<readonly [string, number]>) {
  let nextID = state.nextID;
  const keysByParent: Record<string, string[]> = {};

  for (const [parentKey, length] of parentLengths) {
    const keys = (state.keysByParent[parentKey] || []).slice(0, length);
    while (keys.length < length) {
      keys.push(`${prefix}-${nextID}`);
      nextID += 1;
    }
    keysByParent[parentKey] = keys;
  }

  return { keysByParent, nextID };
}

export function SubscriptionEditor({ config, setConfig, t }: EditorProps) {
  const backendIDs = useMemo(() => config.backends.map((backend) => backend.id).filter(Boolean), [config.backends]);
  const subRows = useStableRowKeys("sub-row", config.subs.length);
  const mountParentLengths = useMemo(() => config.subs.map((sub, index) => [subRows.keys[index], sub.mounts.length] as const), [config.subs, subRows.keys]);
  const liveParentLengths = useMemo(() => config.subs.map((sub, index) => [subRows.keys[index], sub.lives?.length || 0] as const), [config.subs, subRows.keys]);
  const [mountRowState, setMountRowState] = useState<NestedRowKeysState>(() => ({ keysByParent: {}, nextID: 1 }));
  const [liveRowState, setLiveRowState] = useState<NestedRowKeysState>(() => ({ keysByParent: {}, nextID: 1 }));
  const mountRows = useMemo(() => normalizeNestedRowKeys("mount-row", mountRowState, mountParentLengths), [mountParentLengths, mountRowState]);
  const liveRows = useMemo(() => normalizeNestedRowKeys("live-row", liveRowState, liveParentLengths), [liveParentLengths, liveRowState]);

  const [newSubRows, setNewSubRows] = useState<Set<string>>(() => new Set());
  const [newMountRows, setNewMountRows] = useState<Set<string>>(() => new Set());
  const [newLiveRows, setNewLiveRows] = useState<Set<string>>(() => new Set());

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
    setMountRowState(() => {
      const keysByParent = { ...mountRows.keysByParent };
      delete keysByParent[rowKey];
      return { keysByParent, nextID: mountRows.nextID };
    });
    setLiveRowState(() => {
      const keysByParent = { ...liveRows.keysByParent };
      delete keysByParent[rowKey];
      return { keysByParent, nextID: liveRows.nextID };
    });
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
    const mountRowKeys = mountRows.keysByParent[subRowKey] || [];
    const rowKey = `mount-row-${mountRows.nextID}`;
    setMountRowState({
      keysByParent: { ...mountRows.keysByParent, [subRowKey]: [...mountRowKeys, rowKey] },
      nextID: mountRows.nextID + 1,
    });
    setNewMountRows((current) => new Set(current).add(rowKey));
    updateSub(subIndex, {
      mounts: [...sub.mounts, { id, name: id, backend: backendIDs[0] || "", path: "/", search: true, refresh: false, hidden: false }],
    });
  }

  function addLive(subIndex: number) {
    const sub = config.subs[subIndex];
    const subRowKey = subRows.keys[subIndex];
    const liveRowKeys = liveRows.keysByParent[subRowKey] || [];
    const rowKey = `live-row-${liveRows.nextID}`;
    setLiveRowState({
      keysByParent: { ...liveRows.keysByParent, [subRowKey]: [...liveRowKeys, rowKey] },
      nextID: liveRows.nextID + 1,
    });
    setNewLiveRows((current) => new Set(current).add(rowKey));
    updateSub(subIndex, {
      lives: [...(sub.lives || []), { name: "Live", type: 0, url: "" }],
    });
  }

  function updateLive(subIndex: number, liveIndex: number, patch: Partial<Live>) {
    const sub = config.subs[subIndex];
    updateSub(subIndex, {
      lives: (sub.lives || []).map((live, i) => (i === liveIndex ? { ...live, ...patch } : live)),
    });
  }

  function removeLive(subIndex: number, liveIndex: number) {
    const sub = config.subs[subIndex];
    const lives = sub.lives || [];
    const subRowKey = subRows.keys[subIndex];
    const liveRowKeys = liveRows.keysByParent[subRowKey] || [];
    const rowKey = liveRowKeys[liveIndex];
    setLiveRowState({
      keysByParent: { ...liveRows.keysByParent, [subRowKey]: liveRowKeys.filter((_, index) => index !== liveIndex) },
      nextID: liveRows.nextID,
    });
    setNewLiveRows((current) => {
      const next = new Set(current);
      next.delete(rowKey);
      return next;
    });
    updateSub(subIndex, { lives: lives.filter((_, i) => i !== liveIndex) });
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
    const mountRowKeys = mountRows.keysByParent[subRowKey] || [];
    const rowKey = mountRowKeys[mountIndex];
    setMountRowState({
      keysByParent: { ...mountRows.keysByParent, [subRowKey]: mountRowKeys.filter((_, index) => index !== mountIndex) },
      nextID: mountRows.nextID,
    });
    setNewMountRows((current) => {
      const next = new Set(current);
      next.delete(rowKey);
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
              <input value={sub.id} onChange={(event) => updateSub(subIndex, { id: event.target.value })} autoComplete="off" name={`subscription-id-${sub.id || subIndex}`} />
            </Field>
            <Field label={t("path")} help={t("helpSubscriptionPath")}>
              <input value={sub.path || ""} onChange={(event) => updateSub(subIndex, { path: event.target.value })} autoComplete="off" name={`subscription-path-${sub.id || subIndex}`} />
            </Field>
            <Field label={t("siteKey")} help={t("helpSiteKey")}>
              <input value={sub.site_key || ""} onChange={(event) => updateSub(subIndex, { site_key: event.target.value })} autoComplete="off" name={`subscription-site-key-${sub.id || subIndex}`} />
            </Field>
            <Field label={t("siteName")} help={t("helpSiteName")}>
              <input value={sub.site_name || ""} onChange={(event) => updateSub(subIndex, { site_name: event.target.value })} autoComplete="off" name={`subscription-site-name-${sub.id || subIndex}`} />
            </Field>
          </div>
          <SecretHashField sub={sub} onChange={(patch) => updateSub(subIndex, patch)} t={t} />
          <div className="mount-head">
            <h3>{t("lives")}</h3>
            <button type="button" className="small" onClick={() => addLive(subIndex)}>
              <Plus size={16} /> {t("live")}
            </button>
          </div>
          {(sub.lives || []).map((live, liveIndex) => {
            const liveRowKeys = liveRows.keysByParent[subRows.keys[subIndex]] || [];
            const rowKey = liveRowKeys[liveIndex];
            return (
              <CollapsibleMount
                title={live.name || t("live")}
                onRemove={() => removeLive(subIndex, liveIndex)}
                defaultOpen={newLiveRows.has(rowKey)}
                removeLabel={t("removeLive")}
                key={rowKey}
              >
                <div className="form-grid">
                  <Field label={t("name")} help={t("helpLiveName")}>
                    <input value={live.name || ""} onChange={(event) => updateLive(subIndex, liveIndex, { name: event.target.value })} autoComplete="off" name={`live-name-${sub.id || subIndex}-${liveIndex}`} />
                  </Field>
                  <Field label={t("liveURL")} help={t("helpLiveURL")}>
                    <input value={live.url || ""} onChange={(event) => updateLive(subIndex, liveIndex, { url: event.target.value })} autoComplete="off" name={`live-url-${sub.id || subIndex}-${liveIndex}`} />
                  </Field>
                  <Field label={t("liveType")} help={t("helpLiveType")}>
                    <input type="number" min="0" value={live.type ?? 0} onChange={(event) => updateLive(subIndex, liveIndex, { type: parseOptionalInt(event.target.value) ?? 0 })} autoComplete="off" name={`live-type-${sub.id || subIndex}-${liveIndex}`} />
                  </Field>
                  <Field label={t("epg")} help={t("helpEPG")}>
                    <input value={live.epg || ""} onChange={(event) => updateLive(subIndex, liveIndex, { epg: event.target.value })} autoComplete="off" name={`live-epg-${sub.id || subIndex}-${liveIndex}`} />
                  </Field>
                  <Field label={t("icon")} help={t("helpIcon")}>
                    <input value={live.logo || ""} onChange={(event) => updateLive(subIndex, liveIndex, { logo: event.target.value })} autoComplete="off" name={`live-logo-${sub.id || subIndex}-${liveIndex}`} />
                  </Field>
                  <Field label={t("userAgent")} help={t("helpLiveUA")}>
                    <input value={live.ua || ""} onChange={(event) => updateLive(subIndex, liveIndex, { ua: event.target.value })} autoComplete="off" name={`live-ua-${sub.id || subIndex}-${liveIndex}`} />
                  </Field>
                </div>
              </CollapsibleMount>
            );
          })}
          <div className="mount-head">
            <h3>{t("mounts")}</h3>
            <button type="button" className="small" onClick={() => addMount(subIndex)}>
              <Plus size={16} /> {t("mount")}
            </button>
          </div>
          {sub.mounts.map((mount, mountIndex) => {
            const mountRowKeys = mountRows.keysByParent[subRows.keys[subIndex]] || [];
            const rowKey = mountRowKeys[mountIndex];
            return (
              <CollapsibleMount
                title={mount.name || mount.id || t("mount")}
                onRemove={() => removeMount(subIndex, mountIndex)}
                defaultOpen={newMountRows.has(rowKey)}
                removeLabel={t("removeMount")}
                key={rowKey}
              >
                <div className="form-grid">
                  <Field label={t("id")} help={t("helpMountID")}>
                    <input value={mount.id} onChange={(event) => updateMount(subIndex, mountIndex, { id: event.target.value })} autoComplete="off" name={`mount-id-${sub.id || subIndex}-${mount.id || mountIndex}`} />
                  </Field>
                  <Field label={t("name")} help={t("helpMountName")}>
                    <input value={mount.name || ""} onChange={(event) => updateMount(subIndex, mountIndex, { name: event.target.value })} autoComplete="off" name={`mount-name-${sub.id || subIndex}-${mount.id || mountIndex}`} />
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
                    <input value={mount.path} onChange={(event) => updateMount(subIndex, mountIndex, { path: event.target.value })} autoComplete="off" name={`mount-path-${sub.id || subIndex}-${mount.id || mountIndex}`} />
                  </Field>
                </div>
                <Field label={t("directoryPasswords")} help={t("helpDirectoryPasswords")}>
                  <textarea
                    className="json-editor"
                    value={formatStringMap(mount.params)}
                    onChange={(event) => updateMount(subIndex, mountIndex, { params: parseStringMapDraft(event.target.value) })}
                    autoComplete="off"
                    spellCheck={false}
                    name={`mount-params-${sub.id || subIndex}-${mount.id || mountIndex}`}
                    placeholder={'{\n  "/Private": "directory-password"\n}'}
                  />
                </Field>
                <Field label={t("playHeaders")} help={t("helpPlayHeaders")}>
                  <textarea
                    className="json-editor"
                    value={formatStringMap(mount.play_headers)}
                    onChange={(event) => updateMount(subIndex, mountIndex, { play_headers: parseStringMapDraft(event.target.value) })}
                    autoComplete="off"
                    spellCheck={false}
                    name={`mount-play-headers-${sub.id || subIndex}-${mount.id || mountIndex}`}
                    placeholder={'{\n  "User-Agent": "Mozilla/5.0"\n}'}
                  />
                </Field>
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

export function CollapsibleMount({
  title,
  onRemove,
  defaultOpen = false,
  removeLabel,
  children,
}: {
  title: string;
  onRemove: () => void;
  defaultOpen?: boolean;
  removeLabel: string;
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
        <button type="button" className="icon danger" aria-label={removeLabel} title={removeLabel} onClick={onRemove}>
          <Trash2 size={16} />
        </button>
      </div>
      {open && children}
    </article>
  );
}
