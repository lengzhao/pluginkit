// core.js —— 全局状态、/api 调用、edit 循环、渲染调度、实例导航与通用工具。
// 画布模式通过 registerCanvasMode 注册（见 views/ 目录），core 不关心具体渲染。

export const state = {
  catalog: [],
  catalogByKind: {},
  document: { rootId: "", plugin: { use: "" }, shared: {} },
  view: { root: null, shared: [] },
  diagnostics: [],
  yaml: "",
  selectedPath: "root",
  navFrom: null,
  navCollapsed: localStorage.getItem("pluginkit:navCollapsed") === "1",
  buildDiagnostics: null,
  allowIfaceMatch: false,
  canvasMode: localStorage.getItem("pluginkit:canvasMode") || "list",
};

export const els = {
  layout: document.getElementById("layout"),
  rootId: document.getElementById("rootId"),
  rootKind: document.getElementById("rootKind"),
  issueCount: document.getElementById("issueCount"),
  navigator: document.getElementById("navigator"),
  navBody: document.getElementById("navBody"),
  navCollapse: document.getElementById("navCollapse"),
  navExpand: document.getElementById("navExpand"),
  canvasToolbar: document.getElementById("canvasToolbar"),
  canvasBody: document.getElementById("canvasBody"),
  inspectorBody: document.getElementById("inspectorBody"),
  yamlPreview: document.getElementById("yamlPreview"),
  importDialog: document.getElementById("importDialog"),
  importForm: document.getElementById("importForm"),
  yamlInput: document.getElementById("yamlInput"),
  importFile: document.getElementById("importFile"),
  toast: document.getElementById("toast"),
};

export async function api(path, options = {}) {
  const res = await fetch(path, {
    headers: { "Content-Type": "application/json", ...(options.headers || {}) },
    ...options,
  });
  const data = await res.json().catch(() => ({}));
  if (!res.ok) {
    throw new Error(data.error || `${res.status} ${res.statusText}`);
  }
  return data;
}

export function activeDiagnostics() {
  return state.buildDiagnostics ?? state.diagnostics ?? [];
}

export function diagsForPath(path) {
  return activeDiagnostics().filter((d) => d.path === path || path.startsWith(d.path + ".") || d.path.startsWith(path + "."));
}

export function errorCount() {
  return activeDiagnostics().filter((d) => d.severity === "error").length;
}

export function hasError(path) {
  return diagsForPath(path).some((d) => d.severity === "error");
}

export async function applyEditResponse(data) {
  state.document = data.document;
  state.view = data.view;
  state.diagnostics = data.diagnostics || [];
  state.yaml = data.yaml || "";
  state.buildDiagnostics = null;
  els.rootId.value = state.document.rootId || "";
  if (state.document.plugin?.use) {
    els.rootKind.value = state.document.plugin.use;
  }
}

export function emitHostEvent(reason, data, operation = "") {
  const evt = {
    reason,
    operation,
    document: data.document,
    yaml: data.yaml,
    diagnostics: data.diagnostics || [],
  };
  if (reason === "build") {
    window.pluginkitManager.onBuild?.(evt);
  } else {
    window.pluginkitManager.onChange?.(evt);
  }
  if (window.parent !== window) {
    window.parent.postMessage({ type: "pluginkit:document", ...evt }, "*");
  }
}

export async function edit(op) {
  const data = await api("/api/edit", {
    method: "POST",
    body: JSON.stringify({ document: state.document, op }),
  });
  await applyEditResponse(data);
  emitHostEvent("edit", data, op.type);
  // setConfig 时保留检查器表单，避免输入过程中重建 DOM 导致光标丢失
  const skipInspector = op.type === "setConfig";
  render({ skipInspector });
  if (skipInspector) {
    inspectorRefresh();
  }
}

export async function runBuild() {
  const btn = document.getElementById("btnBuild");
  btn.disabled = true;
  btn.textContent = "装配中…";
  try {
    const data = await api("/api/build", {
      method: "POST",
      body: JSON.stringify({ document: state.document }),
    });
    state.buildDiagnostics = data.diagnostics || [];
    render();
    emitHostEvent("build", data);
    const n = errorCount();
    if (n === 0) {
      showToast("试装配成功", "ok");
    } else {
      showToast(`试装配发现 ${n} 个问题`, "warn");
      const first = activeDiagnostics().find((d) => d.severity === "error");
      if (first) {
        state.navFrom = null;
        selectPath(first.path);
      }
    }
  } finally {
    btn.disabled = false;
    btn.textContent = "试装配";
  }
}

let toastTimer;
export function showToast(message, tone = "info") {
  const el = els.toast;
  el.textContent = message;
  el.className = `toast toast-${tone}`;
  el.hidden = false;
  clearTimeout(toastTimer);
  toastTimer = setTimeout(() => {
    el.hidden = true;
  }, 2800);
}

export async function copyText(text) {
  if (navigator.clipboard?.writeText) {
    try {
      await navigator.clipboard.writeText(text);
      return true;
    } catch (_) {
      // fall through to execCommand
    }
  }
  const ta = document.createElement("textarea");
  ta.value = text;
  ta.style.position = "fixed";
  ta.style.left = "-9999px";
  document.body.appendChild(ta);
  ta.select();
  try {
    return document.execCommand("copy");
  } finally {
    document.body.removeChild(ta);
  }
}

export function exportFileName() {
  const id = (state.document.rootId || "config").trim();
  return `${id || "config"}.yaml`;
}

export function downloadYAML(yaml = state.yaml, filename = exportFileName()) {
  const blob = new Blob([yaml], { type: "text/yaml;charset=utf-8" });
  const url = URL.createObjectURL(blob);
  const a = document.createElement("a");
  a.href = url;
  a.download = filename;
  a.click();
  URL.revokeObjectURL(url);
}

export function render(options = {}) {
  applyNavCollapsed();
  renderIssueCount();
  renderNavigator();
  renderCanvas();
  if (!options.skipInspector) {
    renderInspectorNow();
  }
  els.yamlPreview.textContent = state.yaml;
}

// 检查器渲染/诊断刷新由 inspector.js 注册，避免 core ↔ inspector 的循环依赖。
let inspectorRender = () => {};
let inspectorRefresh = () => {};
export function registerInspectorRenderer(renderFn, refreshFn) {
  inspectorRender = renderFn;
  inspectorRefresh = refreshFn ?? (() => {});
}
function renderInspectorNow() {
  inspectorRender();
}

export function applyNavCollapsed() {
  els.layout.classList.toggle("nav-collapsed", state.navCollapsed);
  els.navExpand.hidden = !state.navCollapsed;
}

export function setNavCollapsed(collapsed) {
  state.navCollapsed = collapsed;
  localStorage.setItem("pluginkit:navCollapsed", collapsed ? "1" : "0");
  applyNavCollapsed();
}

export function resolveSharedDefinitionPath(path) {
  if (!path?.startsWith("shared.")) return null;
  const shared = state.view?.shared || [];
  const exact = shared.find((s) => s.path === path);
  if (exact) return exact.path;
  let best = null;
  for (const s of shared) {
    if (path.startsWith(s.path + ".") || path.startsWith(s.path + "[")) {
      if (!best || s.path.length > best.length) best = s.path;
    }
  }
  return best;
}

export function navInstancePath(path) {
  if (!path || path === "root" || path.startsWith("root.")) return "root";
  return resolveSharedDefinitionPath(path) || "root";
}

export function sharedTargetPath(refId) {
  return `shared.${refId}`;
}

export function isRefTargetSelected(item) {
  const target = sharedTargetPath(item.refId);
  return state.selectedPath === item.path || state.selectedPath === target;
}

export function subtreeHasError(path) {
  return activeDiagnostics().some(
    (d) =>
      d.severity === "error" &&
      (d.path === path || d.path.startsWith(path + ".") || d.path.startsWith(path + "["))
  );
}

export function renderNavigator() {
  const body = els.navBody;
  body.innerHTML = "";
  if (!state.view?.root) {
    body.innerHTML = `<p class="empty-hint">正在加载…</p>`;
    return;
  }

  const activeNav = navInstancePath(state.selectedPath);

  const assembly = document.createElement("div");
  assembly.className = "nav-group";
  assembly.innerHTML = `<div class="nav-group-title">装配</div>`;
  assembly.appendChild(
    renderNavItem({
      path: "root",
      id: state.document.rootId || "root",
      kind: state.view.root.kind || state.document.plugin?.use || "",
      selected: activeNav === "root",
    })
  );
  body.appendChild(assembly);

  const shared = state.view.shared || [];
  if (shared.length) {
    const group = document.createElement("div");
    group.className = "nav-group";
    group.innerHTML = `<div class="nav-group-title">共享实例 ${shared.length}</div>`;
    for (const node of shared) {
      const id = node.path.slice("shared.".length);
      group.appendChild(
        renderNavItem({
          path: node.path,
          id,
          kind: node.kind || "",
          refCount: node.refCount ?? 0,
          selected: activeNav === node.path,
        })
      );
    }
    body.appendChild(group);
  }
}

function renderNavItem({ path, id, kind, refCount = 0, selected }) {
  const btn = document.createElement("button");
  btn.type = "button";
  btn.className = `nav-item${selected ? " selected" : ""}${subtreeHasError(path) ? " has-error" : ""}`;
  btn.innerHTML = `
    <div class="nav-item-main">
      <div class="nav-item-id">${escapeHtml(id)}</div>
      <div class="nav-item-kind">${escapeHtml(kind)}</div>
    </div>
    <div class="nav-item-badges"></div>
  `;
  const badges = btn.querySelector(".nav-item-badges");
  if (path.startsWith("shared.") && refCount > 0) {
    const badge = document.createElement("span");
    badge.className = "nav-badge";
    badge.textContent = `×${refCount}`;
    badges.appendChild(badge);
  }
  if (subtreeHasError(path)) {
    const badge = document.createElement("span");
    badge.className = "nav-badge err";
    badge.textContent = "!";
    badges.appendChild(badge);
  }
  btn.addEventListener("click", () => {
    if (path === "root") {
      selectPath(path);
      return;
    }
    const from = state.selectedPath;
    selectPath(path, { navFrom: from && from !== path ? from : null });
  });
  return btn;
}

// ---- 画布模式注册与调度 ----

const canvasModes = new Map();

// registerCanvasMode 注册一种画布模式。def: { label, renderAssembly(body), renderInstance(body, node) }。
export function registerCanvasMode(name, def) {
  canvasModes.set(name, def);
}

export function setCanvasMode(mode) {
  if (!canvasModes.has(mode)) return;
  state.canvasMode = mode;
  localStorage.setItem("pluginkit:canvasMode", mode);
  renderCanvas();
}

function renderCanvasToolbar() {
  const bar = els.canvasToolbar;
  bar.innerHTML = "";
  if (!canvasModes.size) {
    bar.hidden = true;
    return;
  }
  bar.hidden = false;
  for (const [name, def] of canvasModes) {
    const btn = document.createElement("button");
    btn.type = "button";
    btn.className = `mode-btn${state.canvasMode === name ? " active" : ""}`;
    btn.textContent = def.label;
    btn.addEventListener("click", () => setCanvasMode(name));
    bar.appendChild(btn);
  }
}

export function renderCanvas() {
  renderCanvasToolbar();
  const body = els.canvasBody;
  body.innerHTML = "";
  if (!state.view?.root) {
    body.innerHTML = `<p class="empty-hint">正在加载…</p>`;
    return;
  }

  const mode = canvasModes.get(state.canvasMode) ?? canvasModes.values().next().value;
  if (!mode) return;

  const canvasTarget = navInstancePath(state.selectedPath);
  if (canvasTarget === "root") {
    mode.renderAssembly(body);
    return;
  }

  const node = findSharedViewNode(canvasTarget);
  if (node) mode.renderInstance(body, node);
  else mode.renderAssembly(body);
}

export function findSharedViewNode(path) {
  return state.view?.shared?.find((s) => s.path === path) ?? null;
}

export function renderIssueCount() {
  const n = errorCount();
  els.issueCount.textContent = n === 0 ? "0 个问题" : `${n} 个问题`;
  els.issueCount.className = `issue-btn ${n === 0 ? "ok" : "err"}`;
}

export function selectPath(path, { navFrom = null } = {}) {
  state.selectedPath = path;
  state.navFrom = navFrom;
  render();
  scrollToSelected(path);
}

export function scrollToSelected(path) {
  requestAnimationFrame(() => {
    const el = els.canvasBody.querySelector(`[data-path="${CSS.escape(path)}"]`);
    el?.scrollIntoView({ block: "nearest", behavior: "smooth" });
  });
}

export function shortPathLabel(path) {
  if (path === "root") return "Root";
  if (path.startsWith("shared.")) {
    const rest = path.slice("shared.".length);
    const dot = rest.indexOf(".");
    return dot === -1 ? `共享 ${rest}` : `共享 ${rest.slice(0, dot)} · ${rest.slice(dot + 1)}`;
  }
  const deps = path.indexOf(".deps.");
  if (deps === -1) return path;
  return path.slice(deps + ".deps.".length);
}

export function collectRefSites(refId) {
  const sites = [];
  const visit = (node) => {
    if (!node) return;
    if (node.role === "ref" && node.refId === refId) sites.push(node.path);
    for (const slot of node.slots || []) {
      for (const item of slot.items || []) visit(item);
    }
  };
  visit(state.view?.root);
  for (const s of state.view?.shared || []) visit(s);
  return sites;
}

export function findViewNode(path) {
  if (state.view?.root?.path === path) return state.view.root;
  const fromRoot = searchItems(state.view?.root, path);
  if (fromRoot) return fromRoot;
  for (const s of state.view?.shared || []) {
    if (s.path === path) return s;
    const found = searchItems(s, path);
    if (found) return found;
  }
  return null;
}

function searchItems(node, path) {
  if (!node?.slots) return null;
  for (const slot of node.slots) {
    for (const item of slot.items || []) {
      if (item.path === path) return item;
      const nested = searchItems(item, path);
      if (nested) return nested;
    }
  }
  return null;
}

export function findViewSlot(path) {
  const nodes = [state.view?.root, ...(state.view?.shared || [])];
  for (const node of nodes) {
    const slot = findSlotInNode(node, path);
    if (slot) return slot;
  }
  return null;
}

function findSlotInNode(node, path) {
  if (!node?.slots) return null;
  for (const slot of node.slots) {
    if (slot.path === path) return slot;
    for (const item of slot.items || []) {
      const nested = findSlotInNode(item, path);
      if (nested) return nested;
    }
  }
  return null;
}

export function fillRootKindOptions() {
  els.rootKind.innerHTML = "";
  for (const item of state.catalog) {
    const opt = document.createElement("option");
    opt.value = item.kind;
    opt.textContent = item.kind;
    els.rootKind.appendChild(opt);
  }
}

export function escapeHtml(s) {
  return String(s ?? "")
    .replaceAll("&", "&amp;")
    .replaceAll("<", "&lt;")
    .replaceAll(">", "&gt;");
}

export function confirmIfDirty() {
  const hasContent =
    state.document.plugin?.use &&
    (Object.keys(state.document.plugin.deps || {}).length > 0 ||
      Object.keys(state.document.shared || {}).length > 0 ||
      Object.keys(state.document.plugin.config || {}).length > 0);
  if (!hasContent) return true;
  return confirm("当前文档将被替换，确定继续？");
}

export async function loadYAML(yaml, { confirmReplace = true } = {}) {
  if (confirmReplace && !confirmIfDirty()) {
    return false;
  }
  const data = await api("/api/load", {
    method: "POST",
    body: JSON.stringify({ yaml }),
  });
  await applyEditResponse(data);
  emitHostEvent("load", data, "importYAML");
  state.navFrom = null;
  state.selectedPath = "root";
  render();
  return true;
}
