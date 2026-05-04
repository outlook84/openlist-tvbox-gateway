import type React from "react";
import type { AdminConfig } from "./types";
import type { MessageKey } from "./i18n";

export type T = (key: MessageKey) => string;

export interface EditorProps {
  config: AdminConfig;
  setConfig: React.Dispatch<React.SetStateAction<AdminConfig>>;
  t: T;
}