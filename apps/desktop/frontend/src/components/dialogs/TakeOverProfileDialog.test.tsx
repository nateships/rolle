import { render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { toast } from "sonner";
import { describe, expect, it, vi } from "vitest";
import { TakeOverProfileDialog } from "@/components/dialogs/TakeOverProfileDialog";
import { api } from "@/lib/api";

const listing = { path: "~/.aws/config", keys: [{ name: "credential_process", value: "other-tool creds" }] };

describe("TakeOverProfileDialog", () => {
  it("shows the default section and removes the lines of the other tool", async () => {
    const user = userEvent.setup();
    vi.spyOn(api, "ForeignProfile").mockResolvedValue(listing);
    const remove = vi.spyOn(api, "RemoveForeignKeys").mockResolvedValue();
    const success = vi.spyOn(toast, "success");
    const onClose = vi.fn();
    const onDone = vi.fn();
    render(<TakeOverProfileDialog profile="default" onClose={onClose} onDone={onDone} />);
    const dialog = await screen.findByRole("dialog", { name: "Take over profile default?" });
    // The default profile has no "profile" prefix in the file.
    await within(dialog).findByText("[default]");
    expect(dialog).toHaveTextContent("credential_process = other-tool creds");
    await user.click(within(dialog).getByRole("button", { name: "Take over" }));
    await waitFor(() => expect(remove).toHaveBeenCalledWith("default"));
    await waitFor(() => expect(success).toHaveBeenCalledWith("Removed from ~/.aws/config"));
    expect(onClose).toHaveBeenCalledTimes(1);
    expect(onDone).toHaveBeenCalledTimes(1);
  });

  it("disables Take over when no other tool configures the profile", async () => {
    const user = userEvent.setup();
    vi.spyOn(api, "ForeignProfile").mockResolvedValue({ path: "~/.aws/config", keys: [] });
    const remove = vi.spyOn(api, "RemoveForeignKeys").mockResolvedValue();
    const onClose = vi.fn();
    render(<TakeOverProfileDialog profile="free" onClose={onClose} />);
    const dialog = await screen.findByRole("dialog", { name: "Take over profile free?" });
    expect(await within(dialog).findByText(/No other tool configures free in/)).toBeInTheDocument();
    expect(within(dialog).getByRole("button", { name: "Take over" })).toBeDisabled();
    await user.click(within(dialog).getByRole("button", { name: "Cancel" }));
    expect(onClose).toHaveBeenCalledTimes(1);
    expect(remove).not.toHaveBeenCalled();
  });

  it("stays open with an error toast when the removal fails", async () => {
    const user = userEvent.setup();
    vi.spyOn(api, "ForeignProfile").mockResolvedValue(listing);
    vi.spyOn(api, "RemoveForeignKeys").mockRejectedValue(new Error("permission denied"));
    const error = vi.spyOn(toast, "error");
    const onClose = vi.fn();
    const onDone = vi.fn();
    render(<TakeOverProfileDialog profile="prod" onClose={onClose} onDone={onDone} />);
    const dialog = await screen.findByRole("dialog");
    await user.click(await within(dialog).findByRole("button", { name: "Take over" }));
    await waitFor(() => expect(error).toHaveBeenCalledWith("permission denied"));
    expect(onClose).not.toHaveBeenCalled();
    expect(onDone).not.toHaveBeenCalled();
    expect(within(dialog).getByRole("button", { name: "Take over" })).toBeEnabled();
  });

  it("reports a listing failure and renders nothing without a profile", async () => {
    vi.spyOn(api, "ForeignProfile").mockRejectedValue(new Error("cannot read config"));
    const error = vi.spyOn(toast, "error");
    const view = render(<TakeOverProfileDialog profile="prod" onClose={vi.fn()} />);
    await waitFor(() => expect(error).toHaveBeenCalledWith("cannot read config"));
    // Without a listing there is nothing to confirm.
    expect(screen.getByRole("button", { name: "Take over" })).toBeDisabled();
    view.unmount();
    const list = vi.spyOn(api, "ForeignProfile").mockResolvedValue(listing);
    list.mockClear();
    render(<TakeOverProfileDialog profile={null} onClose={vi.fn()} />);
    expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
    expect(list).not.toHaveBeenCalled();
  });
});
