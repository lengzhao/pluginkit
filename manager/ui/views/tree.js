// views/tree.js —— 树状模式：缩进层级 + 折叠/展开 + 诊断聚合徽标。
// 与列表模式渲染同一份 view 投影，共享 selectedPath；折叠状态是纯 UI 态，
// 按 path 持久化到 localStorage，不改变文档。
import {
  state,
  selectPath,
  hasError,
  subtreeHasError,
  escapeHtml,
  sharedTargetPath,
  registerCanvasMode,
  renderCanvas,
} from "../core.js";

const collapsed = new Set(
  JSON.parse(localStorage.getItem("pluginkit:treeCollapsed") || "[]")
);

function isCollapsed(path) {
  return collapsed.has(path);
}

function toggleCollapsed(path) {
  if (collapsed.has(path)) {
    collapsed.delete(path);
  } else {
    collapsed.add(path);
  }
  localStorage.setItem("pluginkit:treeCollapsed", JSON.stringify([...collapsed]));
}

function renderAssembly(body) {
  const label = document.createElement("div");
  label.className = "canvas-mode-label";
  label.textContent = "装配结构";
  body.appendChild(label);
  const tree = document.createElement("div");
  tree.className = "treeview";
  appendNode(tree, state.view.root, 0, true);
  body.appendChild(tree);
}

function renderInstance(body, node) {
  const label = document.createElement("div");
  label.className = "canvas-mode-label";
  label.textContent = "实例";
  body.appendChild(label);

  const id = node.path.slice("shared.".length);
  const header = document.createElement("div");
  header.className = "instance-header";
  header.innerHTML = `
    <div class="instance-title">${escapeHtml(id)}</div>
    <div class="instance-meta">${escapeHtml(node.kind || "")} · 引用 ${node.refCount ?? 0} 处</div>
  `;
  body.appendChild(header);

  if (node.slots?.length) {
    const tree = document.createElement("div");
    tree.className = "treeview";
    appendSlots(tree, node, 0);
    body.appendChild(tree);
  } else {
    const hint = document.createElement("p");
    hint.className = "empty-hint";
    hint.textContent = "该实例没有依赖槽位，在右侧检查器编辑 config。";
    body.appendChild(hint);
  }
}

// appendNode 渲染一个插件节点行；未折叠时递归渲染其槽位与子节点。
function appendNode(container, node, depth, isRoot = false) {
  const hasChildren = (node.slots || []).length > 0;
  const folded = isCollapsed(node.path);

  const row = makeRow(node.path, depth, {
    selected: state.selectedPath === node.path,
    error: hasError(node.path),
    kind: "node",
  });

  row.appendChild(makeToggle(node.path, hasChildren, folded));

  const main = document.createElement("div");
  main.className = "tv-main";
  main.innerHTML = `
    <span class="tv-kind">${escapeHtml(node.kind || node.role)}</span>
    <span class="tv-path">${escapeHtml(isRoot ? "root" : node.path)}</span>
  `;
  row.appendChild(main);

  // 折叠时聚合子树错误与空槽数，让问题在折叠状态下仍可见。
  if (folded) {
    if (subtreeHasError(node.path)) {
      row.appendChild(makeBadge("!", "err"));
    }
    const empty = countEmptySlots(node);
    if (empty > 0) {
      row.appendChild(makeBadge(`${empty} 空槽`, "warn"));
    }
  }

  row.addEventListener("click", () => {
    state.navFrom = null;
    selectPath(node.path);
  });
  container.appendChild(row);

  if (hasChildren && !folded) {
    appendSlots(container, node, depth);
  }
}

function appendSlots(container, node, depth) {
  for (const slot of node.slots || []) {
    appendSlot(container, slot, depth + 1);
    if (slot.status === "empty") continue;
    for (const item of slot.items || []) {
      if (item.role === "ref") {
        appendRef(container, item, depth + 2);
      } else {
        appendNode(container, item, depth + 2);
      }
    }
  }
}

function appendSlot(container, slot, depth) {
  const row = makeRow(slot.path, depth, {
    selected: state.selectedPath === slot.path,
    error: hasError(slot.path),
    kind: `slot${slot.status === "empty" ? " empty" : ""}`,
  });

  row.appendChild(makeToggle(null, false, false));

  const main = document.createElement("div");
  main.className = "tv-main";
  const meta = `${slot.list ? "[]" : ""}${slot.type || ""}${slot.optional ? " · optional" : ""}`;
  const status = slot.status === "empty" ? `<span class="tv-empty">未配置</span>` : "";
  main.innerHTML = `
    <span class="tv-slot-name">${escapeHtml(slot.name)}</span>
    <span class="tv-path">${escapeHtml(meta)}</span>
    ${status}
  `;
  row.appendChild(main);

  row.addEventListener("click", () => {
    state.navFrom = null;
    selectPath(slot.path);
  });
  container.appendChild(row);
}

function appendRef(container, item, depth) {
  const target = sharedTargetPath(item.refId);
  const row = makeRow(item.path, depth, {
    selected: state.selectedPath === item.path || state.selectedPath === target,
    error: hasError(item.path),
    kind: "ref",
  });
  row.dataset.refTarget = target;

  row.appendChild(makeToggle(null, false, false));

  const main = document.createElement("div");
  main.className = "tv-main";
  main.innerHTML = `
    <span class="tv-ref">→ ${escapeHtml(item.refId)}</span>
    <span class="tv-path">${escapeHtml(item.kind || "?")}</span>
  `;
  row.appendChild(main);

  row.addEventListener("click", () => {
    state.navFrom = null;
    selectPath(target);
  });
  container.appendChild(row);
}

function makeRow(path, depth, { selected, error, kind }) {
  const row = document.createElement("div");
  row.className = `tv-row tv-${kind.split(" ")[0]}${selected ? " selected" : ""}${error ? " has-error" : ""}${kind.includes("empty") ? " empty" : ""}`;
  row.dataset.path = path;
  row.style.paddingLeft = `${depth * 20 + 6}px`;
  return row;
}

function makeToggle(path, hasChildren, folded) {
  const btn = document.createElement("button");
  btn.type = "button";
  btn.className = "tv-toggle";
  if (!hasChildren) {
    btn.textContent = "·";
    btn.disabled = true;
    btn.tabIndex = -1;
    return btn;
  }
  btn.textContent = folded ? "▸" : "▾";
  btn.title = folded ? "展开" : "折叠";
  btn.addEventListener("click", (e) => {
    e.stopPropagation();
    toggleCollapsed(path);
    // 折叠是纯 UI 态，只重绘画布，不触发完整 render。
    renderCanvas();
  });
  return btn;
}

function makeBadge(text, tone) {
  const badge = document.createElement("span");
  badge.className = `tv-badge ${tone}`;
  badge.textContent = text;
  return badge;
}

function countEmptySlots(node) {
  let n = 0;
  const visit = (nd) => {
    for (const slot of nd.slots || []) {
      if (slot.status === "empty") n += 1;
      for (const item of slot.items || []) {
        if (item.role !== "ref") visit(item);
      }
    }
  };
  visit(node);
  return n;
}

registerCanvasMode("tree", {
  label: "树状",
  renderAssembly,
  renderInstance,
});
