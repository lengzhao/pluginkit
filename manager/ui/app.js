// app.js —— 入口：加载各模块（视图模式在 import 时自行注册）、绑定顶栏事件、boot。
// 状态与 edit 循环在 core.js；画布模式在 views/；检查器在 inspector.js。
import {
  state,
  els,
  api,
  edit,
  render,
  runBuild,
  showToast,
  downloadYAML,
  exportFileName,
  applyEditResponse,
  fillRootKindOptions,
  escapeHtml,
  confirmIfDirty,
  loadYAML,
  selectPath,
  setNavCollapsed,
  activeDiagnostics,
} from "./core.js";
import "./inspector.js";
import "./views/list.js";
import "./views/tree.js";

document.getElementById("btnNew").addEventListener("click", async () => {
  if (!confirmIfDirty()) return;
  await edit({ type: "newRoot", kind: els.rootKind.value });
  state.navFrom = null;
  state.selectedPath = "root";
  render();
});

document.getElementById("btnBuild").addEventListener("click", () => {
  runBuild().catch((err) => {
    showToast(err.message || "试装配失败", "err");
  });
});

async function importYAMLText(yaml, sourceLabel = "YAML") {
  const ok = await loadYAML(yaml);
  if (ok) {
    els.importDialog.close();
    showToast(`已导入 ${sourceLabel}`, "ok");
  }
  return ok;
}

async function importFromFile(file) {
  if (!file) return false;
  try {
    return await importYAMLText(await file.text(), file.name);
  } catch (err) {
    showToast(err.message, "err");
    return false;
  }
}

document.getElementById("btnExport").addEventListener("click", () => {
  if (!state.yaml?.trim()) {
    showToast("没有可导出的 YAML", "warn");
    return;
  }
  downloadYAML();
  document.querySelector(".yaml-fold")?.setAttribute("open", "");
  showToast(`已下载 ${exportFileName()}`, "ok");
});

document.getElementById("btnImport").addEventListener("click", () => {
  els.yamlInput.value = state.yaml;
  els.importDialog.showModal();
});

document.getElementById("importPickFile").addEventListener("click", () => {
  els.importFile.click();
});

els.importFile.addEventListener("change", async () => {
  const file = els.importFile.files?.[0];
  els.importFile.value = "";
  await importFromFile(file);
});

els.importForm.addEventListener("submit", async (e) => {
  if (e.submitter?.id !== "importConfirm") return;
  try {
    await importYAMLText(els.yamlInput.value);
  } catch (err) {
    showToast(err.message, "err");
  }
});

els.rootId.addEventListener("change", async () => {
  const id = els.rootId.value.trim();
  if (!id || id === state.document.rootId) return;
  try {
    await edit({ type: "setRootId", id });
  } catch (err) {
    alert(err.message);
    els.rootId.value = state.document.rootId;
  }
});

els.rootKind.addEventListener("change", async () => {
  if (!confirmIfDirty()) {
    els.rootKind.value = state.document.plugin?.use || "";
    return;
  }
  await edit({ type: "newRoot", kind: els.rootKind.value });
  state.navFrom = null;
  state.selectedPath = "root";
  render();
});

els.issueCount.addEventListener("click", () => {
  const first = activeDiagnostics().find((d) => d.severity === "error");
  if (first) {
    state.navFrom = null;
    selectPath(first.path);
  }
});

els.navCollapse.addEventListener("click", () => setNavCollapsed(true));
els.navExpand.addEventListener("click", () => setNavCollapsed(false));

async function boot() {
  const data = await api("/api/bootstrap");
  state.catalog = data.kinds || [];
  state.catalogByKind = Object.fromEntries(
    state.catalog.map((item) => [item.kind, item])
  );
  fillRootKindOptions();
  if (data.document?.plugin?.use) {
    await applyEditResponse(data);
    state.navFrom = null;
    state.selectedPath = "root";
    render();
    return;
  }
  const kind = state.catalog.some((k) => k.kind === "agent")
    ? "agent"
    : state.catalog[0]?.kind;
  if (kind) {
    await edit({ type: "newRoot", kind });
    state.navFrom = null;
    state.selectedPath = "root";
    render();
  } else {
    render();
  }
}

window.pluginkitManager = {
  loadYAML,
  getYAML: () => state.yaml,
  getDocument: () => state.document,
  getDiagnostics: () => activeDiagnostics(),
  onChange: null,
  onBuild: null,
};

boot().catch((err) => {
  els.canvasBody.innerHTML = `<p class="empty-hint">${escapeHtml(err.message)}</p>`;
});
