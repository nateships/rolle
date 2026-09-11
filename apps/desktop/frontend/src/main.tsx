import React from "react";
import ReactDOM from "react-dom/client";
import App from "./App";
import { Toaster } from "@/components/ui/sonner";
import { TooltipProvider } from "@/components/ui/tooltip";
import { ErrorBoundary } from "@/components/ErrorBoundary";
import "./index.css";
import { applyTheme, cachedTheme } from "./lib/theme";

// Apply the remembered theme before the first paint; settings refine it after load.
const preview = new URLSearchParams(location.search).get("theme");
applyTheme(preview === "light" || preview === "dark" || preview === "system" ? preview : cachedTheme());

// macOS draws its window controls over the top-left of the web view. Mark the
// document so headers can inset their leading content past them.
if (/Macintosh/.test(navigator.userAgent)) document.documentElement.classList.add("mac");

ReactDOM.createRoot(document.getElementById("root") as HTMLElement).render(
  <React.StrictMode>
    <ErrorBoundary>
      <TooltipProvider delayDuration={300}>
        <App />
      </TooltipProvider>
    </ErrorBoundary>
    <Toaster position="bottom-right" richColors closeButton />
  </React.StrictMode>,
);
