import { request } from "./client";

function post<T>(path: string, body?: unknown) {
  return request<T>(path, { method: "POST", body: body === undefined ? undefined : JSON.stringify(body) });
}
function put<T>(path: string, body?: unknown) {
  return request<T>(path, { method: "PUT", body: body === undefined ? undefined : JSON.stringify(body) });
}

export type Camera = {
  id: number;
  code: string;
  name: string;
  project_id?: number | null;
  group_id?: number | null;
  access_type: string;
  vendor?: string;
  model?: string;
  remark?: string;
  enabled: boolean;
  archived: boolean;
  nvr_agent_id?: number | null;
  agent_camera_id?: string;
  device_status: string;
  media_status: string;
  analysis_status: string;
  last_error?: string;
  streams?: { id: number; role: string; url: string; protocol: string; revision: number }[];
};

export type CameraGroup = { id: number; code: string; name: string; project_id?: number | null; remark?: string; enabled: boolean };
export type BoundaryVersion = {
  id: number; camera_id: number; version: number; status: string; name?: string;
  geometry_json: string; ref_width: number; ref_height: number; source: string; invalid_reason?: string;
};
export type Deployment = {
  id: number; name: string; camera_id: number; project_id?: number | null;
  template_id?: number | null; boundary_version_id?: number | null;
  desired_revision: number; applied_revision: number; status: string;
  analysis_fps: number; threshold_near: number; threshold_cross: number;
  last_error?: string; last_self_check?: string; runtime?: RuntimeStatus;
};
export type RuntimeStatus = {
  deployment_id: number; camera_id: number; state: string; actual_fps: number;
  frame_age_ms: number; last_result: string; last_error?: string; backend?: string;
};
export type VideoEvent = {
  id: number; event_key: string; camera_id: number; category: string; level: string;
  scene_status: string; handle_status: string; conclusion?: string; conclusion_remark?: string;
  started_at: string; last_evidence_at: string; ended_at?: string | null;
};
export type AlgoModel = { id: number; code: string; name: string; framework: string; versions?: any[] };
export type RuleVersion = { id: number; code: string; version: string; name: string; status: string; description?: string };
export type BusinessTemplate = { id: number; code: string; name: string; version: string; status: string; description?: string };
export type AccessDevice = { id: number; kind: string; code: string; name: string; status: string; last_error?: string; enabled: number };

export const listCameraGroups = () => request<CameraGroup[]>("/api/cameras/groups");
export const createCameraGroup = (body: Partial<CameraGroup>) => post("/api/cameras/groups", body);
export const listCameras = (params?: Record<string, string | number>) => {
  const q = new URLSearchParams();
  if (params) Object.entries(params).forEach(([k, v]) => q.set(k, String(v)));
  const s = q.toString();
  return request<{ items: Camera[]; total: number }>(`/api/cameras${s ? `?${s}` : ""}`);
};
export const createCamera = (body: Record<string, unknown>) => post<Camera>("/api/cameras", body);
export const updateCamera = (id: number, body: Record<string, unknown>) => put<Camera>(`/api/cameras/${id}`, body);
export const archiveCamera = (id: number) => request(`/api/cameras/${id}`, { method: "DELETE" });
export const syncCamerasFromNVR = (agentId: number, body: { cameras: { id: string; name: string }[]; project_id?: number }) =>
  post<{ created: number }>(`/api/cameras/sync-nvr/${agentId}`, body);

export const listBoundaries = (cameraId?: number) =>
  request<BoundaryVersion[]>(`/api/boundaries${cameraId ? `?camera_id=${cameraId}` : ""}`);
export const createBoundary = (body: Record<string, unknown>) => post<BoundaryVersion>("/api/boundaries", body);
export const publishBoundary = (id: number) => post<BoundaryVersion>(`/api/boundaries/${id}/publish`);
export const rollbackBoundary = (id: number) => post<BoundaryVersion>(`/api/boundaries/${id}/rollback`);
export const boundaryCandidate = (cameraId: number) =>
  request<{ geometry: any; ref_width: number; ref_height: number; demo: boolean }>(`/api/boundaries/candidate?camera_id=${cameraId}`);

export const listModels = () => request<AlgoModel[]>("/api/algorithms/models");
export const listRules = () => request<RuleVersion[]>("/api/algorithms/rules");
export const listTemplates = () => request<BusinessTemplate[]>("/api/algorithms/templates");

export const listDeployments = () => request<Deployment[]>("/api/deployments");
export const createDeployment = (body: Record<string, unknown>) => post<Deployment>("/api/deployments", body);
export const updateDeployment = (id: number, body: Record<string, unknown>) => put<Deployment>(`/api/deployments/${id}`, body);
export const selfCheckDeployment = (id: number) => post<{ ok: boolean; report: any }>(`/api/deployments/${id}/self-check`);
export const startDeployment = (id: number) => post<Deployment>(`/api/deployments/${id}/start`);
export const stopDeployment = (id: number) => post<Deployment>(`/api/deployments/${id}/stop`);
export const pauseDeployment = (id: number) => post<Deployment>(`/api/deployments/${id}/pause`);

export const listEvents = (params?: Record<string, string | number>) => {
  const q = new URLSearchParams();
  if (params) Object.entries(params).forEach(([k, v]) => q.set(k, String(v)));
  const s = q.toString();
  return request<{ items: VideoEvent[]; total: number }>(`/api/events${s ? `?${s}` : ""}`);
};
export const getEvent = (id: number) => request<{ event: VideoEvent; actions: any[]; evidences: any[] }>(`/api/events/${id}`);
export const evidenceURL = (eventId: number, evidenceId: number) =>
  `/api/events/${eventId}/evidences/${evidenceId}/file`;
export const actEvent = (id: number, body: { action: string; conclusion?: string; remark?: string }) =>
  post(`/api/events/${id}/actions`, body);
export const batchConfirmEvents = (ids: number[]) => post(`/api/events/batch-confirm`, { ids });

export const reportSummary = () => request<Record<string, unknown>>("/api/reports/summary");
export const runtimeMetrics = () => request<RuntimeStatus[]>("/api/reports/runtime");
export const videoSelfCheck = () => request<{ ok: boolean; checks: { name: string; ok: boolean; detail: string }[] }>("/api/video/self-check");

export const listAccessDevices = (kind?: string) =>
  request<AccessDevice[]>(`/api/access-devices${kind ? `?kind=${kind}` : ""}`);
export const createAccessDevice = (body: Record<string, unknown>) => post<AccessDevice>("/api/access-devices", body);
export const probeAccessDevice = (id: number) => post(`/api/access-devices/${id}/probe`);
export const deleteAccessDevice = (id: number) => request(`/api/access-devices/${id}`, { method: "DELETE" });
