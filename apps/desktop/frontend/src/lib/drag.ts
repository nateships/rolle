/** Drag data types. A session row drops onto a tag; a tag drops onto another tag to reorder. */
export const SESSION_DRAG = "application/x-rolle-session";
export const TAG_DRAG = "application/x-rolle-tag";

/**
 * Replaces the browser's ghost of the whole row with a small pill that names
 * the session. The element has to be in the document when setDragImage
 * runs; it is removed on the next frame, once the browser has copied it.
 */
export function setSessionDragImage(e: React.DragEvent, name: string) {
  if (typeof e.dataTransfer.setDragImage !== "function") return;
  const pill = document.createElement("div");
  pill.textContent = name;
  Object.assign(pill.style, {
    position: "fixed",
    top: "-100px",
    left: "-100px",
    padding: "6px 12px",
    borderRadius: "9999px",
    background: "#101114",
    color: "#f4f0e8",
    font: "500 13px -apple-system, system-ui, sans-serif",
    boxShadow: "0 6px 20px rgba(0, 0, 0, 0.35)",
    border: "1px solid rgba(244, 240, 232, 0.15)",
    whiteSpace: "nowrap",
    pointerEvents: "none",
  } as Partial<CSSStyleDeclaration>);
  document.body.appendChild(pill);
  e.dataTransfer.setDragImage(pill, 16, 16);
  requestAnimationFrame(() => pill.remove());
}
