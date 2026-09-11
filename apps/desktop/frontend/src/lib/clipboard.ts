import { Clipboard } from "@wailsio/runtime";
import { inWails } from "./api";

/** Copy text through the Wails clipboard inside the app, or the browser API in preview. */
export async function copyText(text: string): Promise<void> {
  if (inWails) return Clipboard.SetText(text);
  await navigator.clipboard.writeText(text);
}
