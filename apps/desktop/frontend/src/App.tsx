import { useEffect } from "react";
import { AnimatePresence, motion } from "motion/react";
import { useWorkspace } from "@/lib/api";
import { applyTheme, watchSystemTheme, type Theme } from "@/lib/theme";
import { Onboarding } from "@/components/onboarding/Onboarding";
import { Dashboard } from "@/components/dashboard/Dashboard";
import { Mark } from "@/components/Brand";

export default function App() {
  const { workspace, error, reload } = useWorkspace();
  const theme = (workspace?.settings?.theme ?? "system") as Theme;

  useEffect(() => {
    if (workspace) applyTheme(theme);
    return watchSystemTheme(() => theme);
  }, [workspace, theme]);

  if (error) {
    return (
      <main className="flex h-full flex-col items-center justify-center gap-4 bg-background p-8 text-foreground">
        <Mark className="size-12 text-destructive" />
        <p className="max-w-md text-center text-sm text-muted-foreground">{error}</p>
        <button className="text-sm underline" onClick={() => void reload()}>Retry</button>
      </main>
    );
  }

  if (!workspace) {
    return (
      <main className="drag flex h-full items-center justify-center bg-background text-foreground">
        <Mark className="size-12 text-primary" animate />
      </main>
    );
  }

  return (
    <div className="h-full bg-background text-foreground">
      <AnimatePresence mode="wait">
        {workspace.onboarded ? (
          <motion.div key="dash" className="h-full" initial={{ opacity: 0 }} animate={{ opacity: 1 }} exit={{ opacity: 0 }} transition={{ duration: 0.2 }}>
            <Dashboard workspace={workspace} />
          </motion.div>
        ) : (
          <motion.div key="onb" className="h-full" initial={{ opacity: 0 }} animate={{ opacity: 1 }} exit={{ opacity: 0, scale: 0.98 }} transition={{ duration: 0.2 }}>
            <Onboarding workspace={workspace} />
          </motion.div>
        )}
      </AnimatePresence>
    </div>
  );
}
