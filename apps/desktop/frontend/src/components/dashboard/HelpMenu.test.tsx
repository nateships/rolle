import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { toast } from "sonner";
import { describe, expect, it, vi } from "vitest";
import { HelpMenu } from "@/components/dashboard/HelpMenu";
import { api } from "@/lib/api";

const open = async (user: ReturnType<typeof userEvent.setup>) => {
  await user.click(screen.getByRole("button", { name: "Help" }));
  return screen.findByRole("menu");
};

describe("HelpMenu", () => {
  it("opens the docs and the changelog", async () => {
    const user = userEvent.setup();
    const openUrl = vi.spyOn(api, "OpenURL").mockResolvedValue();
    render(<HelpMenu />);
    await open(user);
    await user.click(screen.getByRole("menuitem", { name: /documentation/i }));
    expect(openUrl).toHaveBeenCalledWith("https://getrolle.com/docs");
    await open(user);
    await user.click(screen.getByRole("menuitem", { name: /changelog/i }));
    expect(openUrl).toHaveBeenCalledWith("https://getrolle.com/changelog");
  });

  it("saves a support bundle and opens the report form", async () => {
    const user = userEvent.setup();
    vi.spyOn(api, "ExportSupportBundle").mockResolvedValue("~/Downloads/rolle-support.zip");
    vi.spyOn(api, "SupportURL").mockResolvedValue("https://github.com/x/issues/new");
    const openUrl = vi.spyOn(api, "OpenURL").mockResolvedValue();
    const success = vi.spyOn(toast, "success");
    render(<HelpMenu />);
    await open(user);
    await user.click(screen.getByRole("menuitem", { name: /support bundle/i }));
    await waitFor(() =>
      expect(success).toHaveBeenCalledWith("Support bundle saved", { description: "~/Downloads/rolle-support.zip" }),
    );
    await open(user);
    await user.click(screen.getByRole("menuitem", { name: /report a problem/i }));
    await waitFor(() => expect(openUrl).toHaveBeenCalledWith("https://github.com/x/issues/new"));
  });

  it("reports a failed bundle export", async () => {
    const user = userEvent.setup();
    vi.spyOn(api, "ExportSupportBundle").mockRejectedValue(new Error("no disk"));
    const error = vi.spyOn(toast, "error");
    render(<HelpMenu />);
    await open(user);
    await user.click(screen.getByRole("menuitem", { name: /support bundle/i }));
    await waitFor(() => expect(error).toHaveBeenCalledWith("no disk"));
  });
});
