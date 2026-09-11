import React from "react";
import ReactDOM from "react-dom/client";
import App from "./App";
import { Toaster } from "@/components/ui/sonner";
import { MotionConfig, MotionGlobalConfig } from "motion/react";
import { TooltipProvider } from "@/components/ui/tooltip";
import { ErrorBoundary } from "@/components/ErrorBoundary";
import "./index.css";
import { applyTheme, cachedTheme } from "./lib/theme";
import { inWails } from "./lib/api";

// Apply the remembered theme before the first paint; settings refine it after load.
const preview = new URLSearchParams(location.search).get("theme");
applyTheme(preview === "light" || preview === "dark" || preview === "system" ? preview : cachedTheme());

// ?shot=1 renders every animation at its final frame, for screenshots.
if (new URLSearchParams(location.search).get("shot") === "1") MotionGlobalConfig.skipAnimations = true;

// macOS draws its window controls over the top-left of the web view. Mark the
// document so headers can inset their leading content past them.
if (/Macintosh/.test(navigator.userAgent)) document.documentElement.classList.add("mac");

// Release builds show only Rolle's own context menus. Dev builds keep the web view's for Inspect.
if (inWails && import.meta.env.PROD) document.addEventListener("contextmenu", (e) => e.preventDefault());

ReactDOM.createRoot(document.getElementById("root") as HTMLElement).render(
  <React.StrictMode>
    <ErrorBoundary>
      <MotionConfig reducedMotion="user">
        <TooltipProvider delayDuration={300}>
          <App />
        </TooltipProvider>
      </MotionConfig>
    </ErrorBoundary>
    <Toaster position="bottom-right" richColors closeButton />
  </React.StrictMode>,
);
