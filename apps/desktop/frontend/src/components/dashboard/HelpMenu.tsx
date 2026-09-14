import { useState } from "react";
import { BookOpen, Bug, CircleHelp, FileArchive, History, Loader2 } from "lucide-react";
import { toast } from "sonner";
import { Button } from "@/components/ui/button";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import { api, errorMessage } from "@/lib/api";

const DOCS = "https://getrolle.com";

/** The help button at the bottom of the sidebar: docs, changelog, bug form, support bundle. */
export function HelpMenu() {
  const [exporting, setExporting] = useState(false);
  const exportBundle = async () => {
    setExporting(true);
    try {
      const path = await api.ExportSupportBundle();
      toast.success("Support bundle saved", { description: path });
    } catch (e) {
      toast.error(errorMessage(e));
    } finally {
      setExporting(false);
    }
  };
  return (
    <DropdownMenu>
      <DropdownMenuTrigger asChild>
        <Button variant="ghost" size="icon-sm" aria-label="Help">
          <CircleHelp className="size-4" />
        </Button>
      </DropdownMenuTrigger>
      <DropdownMenuContent align="end" side="top" className="min-w-48">
        <DropdownMenuItem onClick={() => void api.OpenURL(DOCS)}>
          <BookOpen /> Documentation
        </DropdownMenuItem>
        <DropdownMenuItem onClick={() => void api.OpenURL(`${DOCS}/changelog`)}>
          <History /> Changelog
        </DropdownMenuItem>
        <DropdownMenuSeparator />
        <DropdownMenuItem onClick={() => void api.SupportURL().then((u) => api.OpenURL(u))}>
          <Bug /> Report a problem
        </DropdownMenuItem>
        <DropdownMenuItem disabled={exporting} onClick={() => void exportBundle()}>
          {exporting ? <Loader2 className="animate-spin" /> : <FileArchive />} Support bundle
        </DropdownMenuItem>
      </DropdownMenuContent>
    </DropdownMenu>
  );
}
