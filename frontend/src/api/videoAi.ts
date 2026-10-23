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
  trigger_hold_sec: number; recover_hold_sec: number;
  schedule_cron: string; mute_notify: boolean; osd_enabled: boolean;
  last_error?: string; last_self_check?: string; runtime?: RuntimeStatus;
};
export type RuntimeStatus = {
  deployment_id: number; camera_id: number; state: string; actual_fps: number;
  frame_age_ms: number; last_result: string; last_error?: string; backend?: string;
  applied_rev?: number; queue_dropped?: number; model_loaded?: number;
  heartbeat_at?: string | null; updated_at?: string;
};
export type VideoEvent = {
  id: number; event_key: string; camera_id: number; category: string; level: string;
  scene_status: string; handle_status: string; conclusion?: string; conclusion_remark?: string;
  started_at: string; last_evidence_at: string; ended_at?: string | null;
};
export type AlgoModel = { id: number; code: string; name: string; framework: string; versions?: any[] };
export type RuleVersion = { id: number; code: string; version: string; name: string; status: string; description?: string };
export type BusinessTemplate = { id: number; code: string; name: string; version: string; status: string; description?: string };
export type AccessDevice = {
  id: number; kind: string; code: string; name: string; status: string;
  last_error?: string; enabled: number; camera_id?: number | null; config_json?: string;
};

export const listCameraGroups = () => request<CameraGroup[]>("/api/cameras/groups");
export const createCameraGroup = (body: Partial<CameraGroup>) => post("/api/cameras/groups", body);
// Live sources for cameras that are not bound to an NVR agent.
export const cameraFrameURL = (id: number) => `/api/cameras/${id}/frame.jpg`;
export const cameraPreviewURL = (id: number) => `/api/cameras/${id}/preview.mjpg`;
export const cameraIngestState = (id: number) =>
  request<{ enabled: boolean; running?: boolean; has_stream?: boolean; state?: IngestState }>(`/api/cameras/${id}/ingest`);
export const videoWSURL = () =>
  `${location.protocol === "https:" ? "wss" : "ws"}://${location.host}/api/video/ws`;
export const ingestStates = () => request<IngestState[]>("/api/video/ingest");

export type IngestState = {
  camera_id: number; url: string; codec: string; running: boolean; connected: boolean;
  frames: number; bytes: number; frame_age_ms: number; buffer_bytes: number;
  last_frame_at?: string | null; last_error: string; started_at: string;
};

// Event clips for the recording centre.
export type EventClip = {
  event: VideoEvent;
  evidences: { id: number; kind: string; status: string; path: string; error?: string }[];
};
export const eventClips = (cameraId: number, limit?: number) =>
  request<EventClip[]>(`/api/cameras/${cameraId}/event-clips${limit ? `?limit=${limit}` : ""}`);

export const listCameras = (params?: Record<string, string | number>) => {
  const q = new URLSearchParams();
  if (params) Object.entries(params).forEach(([k, v]) => q.set(k, String(v)));
  const s = q.toString();
  return request<{ items: Camera[]; total: number }>(`/api/cameras${s ? `?${s}` : ""}`);
};
export const createCamera = (body: Record<string, unknown>) => post<Camera>("/api/cameras", body);
export const updateCamera = (id: number, body: Record<string, unknown>) => put<Camera>(`/api/cameras/${id}`, body);
export const archiveCamera = (id: number) => request(`/api/cameras/${id}`, { method: "DELETE" });
export const probeCamera = (id: number) =>
  post<{ ok: boolean; camera_id: number; access_type: string; media_status: string; detail?: string; error?: string }>(
    `/api/cameras/${id}/probe`,
  );
export type ImportRow = {
  code: string; name: string; access_type?: string; vendor?: string; model?: string;
  remark?: string; project_id?: number; group_id?: number;
  nvr_agent_id?: number; agent_camera_id?: string; main_url?: string; sub_url?: string; enabled?: number;
};
export type ImportResult = {
  index: number; code: string; ok: boolean; id?: number; error?: string;
};
export const importCameras = (rows: ImportRow[], projectId?: number, groupId?: number) =>
  post<{ total: number; created: number; failed: number; results: ImportResult[] }>("/api/cameras/import", {
    rows, project_id: projectId ?? null, group_id: groupId ?? null,
  });

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
export const createModelVersion = (body: { model_id: number; version: string; adapter?: string; backend?: string; file_path?: string; checksum?: string }) =>
  post<{ id: number; version: string; status: string; adapter: string }>("/api/algorithms/model-versions", body);
export const publishModelVersion = (id: number) =>
  post<{ id: number; status: string }>(`/api/algorithms/model-versions/${id}/publish`);
export const selfCheckModelVersion = (id: number) =>
  post<{ ok: boolean; adapter: string; demo: boolean; checks: { name: string; ok: boolean; detail: string }[] }>(
    `/api/algorithms/model-versions/${id}/self-check`,
  );

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
export const probeAccessDevice = (id: number) =>
  post<{ ok: boolean; status: string; message: string; next_steps?: string[] }>(`/api/access-devices/${id}/probe`);
export const syncAccessDeviceCamera = (id: number) =>
  post<{ camera: Camera; created: boolean }>(`/api/access-devices/${id}/sync-camera`);
export const deleteAccessDevice = (id: number) => request(`/api/access-devices/${id}`, { method: "DELETE" });

export type RecordingPlan = {
  id?: number;
  camera_id: number;
  mode: "off" | "schedule" | "continuous" | "event_only";
  retain_days: number;
  max_gb: number;
  schedule_json?: string;
  enabled: number;
};
export const getRecordingPlan = (cameraId: number) =>
  request<RecordingPlan>(`/api/recording-plans/${cameraId}`);
export const saveRecordingPlan = (cameraId: number, body: Partial<RecordingPlan>) =>
  put<RecordingPlan>(`/api/recording-plans/${cameraId}`, body);
