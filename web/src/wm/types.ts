import type { ComponentType } from "react";
import type { IconName } from "../components/Icon";

export type AppId =
  | "package-center"
  | "storage"
  | "replication"
  | "shares"
  | "identity"
  | "workloads"
  | "vms"
  | "alerts"
  | "logs"
  | "audit"
  | "jobs"
  | "notifications"
  | "network"
  | "system"
  | "files"
  | "terminal"
  | "control-panel";

export type AppDef = {
  id: AppId;
  title: string;
  icon: IconName;
  defaultSize: { w: number; h: number };
  Component: ComponentType;
};

export type WindowState = {
  id: string;
  appId: AppId;
  x: number;
  y: number;
  w: number;
  h: number;
  z: number;
  minimized: boolean;
  maximized: boolean;
};
