import { render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { toast } from "sonner";
import { describe, expect, it, vi } from "vitest";
import { RemoveKeysDialog } from "@/components/dialogs/RemoveKeysDialog";
import { StaticKeysCard } from "@/components/StaticKeys";
import { api } from "@/lib/api";

const prof = (name: string, imported = false) => ({
  name,
  keys: [
    { name: "aws_access_key_id", preview: "AKIA…" },
    { name: "aws_secret_access_key", preview: "…" },
  ],
  imported,
});

describe("StaticKeysCard", () => {
  it("renders nothing without static keys", async () => {
    vi.spyOn(api, "StaticProfiles").mockResolvedValue({ path: "~/.aws/credentials", profiles: [] });
    const { container } = render(<StaticKeysCard />);
    await waitFor(() => expect(api.StaticProfiles).toHaveBeenCalled());
    expect(container).toBeEmptyDOMElement();
  });

  it("lists the sections and removes one through the confirmation", async () => {
    const user = userEvent.setup();
    const list = vi
      .spyOn(api, "StaticProfiles")
      // The card reads once, the dialog reads once more, then the card again after the removal.
      .mockResolvedValueOnce({ path: "~/.aws/credentials", profiles: [prof("default"), prof("personal")] })
      .mockResolvedValueOnce({ path: "~/.aws/credentials", profiles: [prof("default"), prof("personal")] })
      .mockResolvedValue({ path: "~/.aws/credentials", profiles: [prof("personal")] });
    const remove = vi.spyOn(api, "RemoveStaticProfile").mockResolvedValue();
    render(<StaticKeysCard />);
    expect(await screen.findByText("default")).toBeInTheDocument();
    const [first] = screen.getAllByRole("button", { name: /^Remove$/ });
    await user.click(first);
    // Nothing goes before the confirmation.
    expect(remove).not.toHaveBeenCalled();
    expect(await screen.findByRole("dialog", { name: "Remove profile from the credentials file?" })).toHaveTextContent(
      "[default]",
    );
    await user.click(within(screen.getByRole("dialog")).getByRole("button", { name: "Remove" }));
    await waitFor(() => expect(remove).toHaveBeenCalledWith("default"));
    await waitFor(() => expect(list.mock.calls.length).toBeGreaterThanOrEqual(2));
    expect(screen.queryByText("default")).not.toBeInTheDocument();
    // Remove all shows for more than one section only.
    expect(screen.queryByRole("button", { name: "Remove all" })).not.toBeInTheDocument();
  });

  it("imports a key into rolle, then offers the removal", async () => {
    const user = userEvent.setup();
    const token = { name: "token", keys: [{ name: "aws_session_token", preview: "…" }], imported: false };
    const listing = { path: "~/.aws/credentials", profiles: [prof("personal"), prof("old", true), token] };
    vi.spyOn(api, "StaticProfiles").mockResolvedValue(listing);
    const imp = vi.spyOn(api, "ImportIAMUser").mockResolvedValue({ id: "s1", name: "personal" } as never);
    const remove = vi.spyOn(api, "RemoveStaticProfile").mockResolvedValue();
    render(<StaticKeysCard />);
    expect(await screen.findByText("Import moves a key into rolle. Remove deletes it.")).toBeInTheDocument();
    // A key a session already holds, or a section without the whole pair, has no Import button.
    expect(screen.getAllByRole("button", { name: "Import" })).toHaveLength(1);
    await user.click(screen.getByRole("button", { name: "Import" }));
    await waitFor(() => expect(imp).toHaveBeenCalledWith("personal"));
    const dialog = await screen.findByRole("dialog", { name: "Remove profile from the credentials file?" });
    expect(dialog).toHaveTextContent("[personal]");
    // The removal is the user's call; nothing is removed before the red button.
    expect(remove).not.toHaveBeenCalled();
    await user.click(within(dialog).getByRole("button", { name: "Remove" }));
    await waitFor(() => expect(remove).toHaveBeenCalledWith("personal"));
  });

  it("keeps the card and reports the error when an import fails", async () => {
    const user = userEvent.setup();
    vi.spyOn(api, "StaticProfiles").mockResolvedValue({ path: "~/.aws/credentials", profiles: [prof("personal")] });
    vi.spyOn(api, "ImportIAMUser").mockRejectedValue(new Error("session personal already exists"));
    const error = vi.spyOn(toast, "error");
    render(<StaticKeysCard />);
    await user.click(await screen.findByRole("button", { name: "Import" }));
    await waitFor(() => expect(error).toHaveBeenCalledWith("session personal already exists"));
    // No removal offer for a key that did not move.
    expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Import" })).toBeEnabled();
  });

  it("offers Remove all with every section, and reports a listing failure", async () => {
    const user = userEvent.setup();
    vi.spyOn(api, "StaticProfiles").mockResolvedValue({
      path: "~/.aws/credentials",
      profiles: [prof("default"), prof("personal")],
    });
    const remove = vi.spyOn(api, "RemoveStaticProfile").mockResolvedValue();
    const view = render(<StaticKeysCard />);
    await user.click(await screen.findByRole("button", { name: "Remove all" }));
    const dialog = await screen.findByRole("dialog", { name: "Remove profiles from the credentials file?" });
    expect(dialog).toHaveTextContent("[default]");
    expect(dialog).toHaveTextContent("[personal]");
    await user.click(within(dialog).getByRole("button", { name: "Remove" }));
    await waitFor(() => expect(remove).toHaveBeenCalledTimes(2));
    expect(remove.mock.calls.map((c) => c[0])).toEqual(["default", "personal"]);
    view.unmount();

    vi.spyOn(api, "StaticProfiles").mockRejectedValue(new Error("cannot read credentials"));
    const error = vi.spyOn(toast, "error");
    const { container } = render(<StaticKeysCard />);
    await waitFor(() => expect(error).toHaveBeenCalledWith("cannot read credentials"));
    expect(container).toBeEmptyDOMElement();
  });
});

describe("RemoveKeysDialog", () => {
  const listing = { path: "~/.aws/credentials", profiles: [prof("default")] };

  it("disables Remove when the file holds no keys for the profile", async () => {
    const user = userEvent.setup();
    vi.spyOn(api, "StaticProfiles").mockResolvedValue(listing);
    const remove = vi.spyOn(api, "RemoveStaticProfile").mockResolvedValue();
    const onClose = vi.fn();
    render(<RemoveKeysDialog target={{ profiles: ["gone"] }} onClose={onClose} />);
    const dialog = await screen.findByRole("dialog", { name: "Remove profile from the credentials file?" });
    expect(await within(dialog).findByText(/No static keys for gone in/)).toBeInTheDocument();
    expect(within(dialog).getByRole("button", { name: "Remove" })).toBeDisabled();
    await user.click(within(dialog).getByRole("button", { name: "Cancel" }));
    expect(onClose).toHaveBeenCalledTimes(1);
    expect(remove).not.toHaveBeenCalled();
  });

  it("stays open with an error toast when the removal fails", async () => {
    const user = userEvent.setup();
    vi.spyOn(api, "StaticProfiles").mockResolvedValue(listing);
    vi.spyOn(api, "RemoveStaticProfile").mockRejectedValue(new Error("permission denied"));
    const error = vi.spyOn(toast, "error");
    const onClose = vi.fn();
    const onDone = vi.fn();
    render(<RemoveKeysDialog target={{ profiles: ["default"] }} onClose={onClose} onDone={onDone} />);
    const dialog = await screen.findByRole("dialog");
    await user.click(await within(dialog).findByRole("button", { name: "Remove" }));
    await waitFor(() => expect(error).toHaveBeenCalledWith("permission denied"));
    expect(onClose).not.toHaveBeenCalled();
    expect(onDone).not.toHaveBeenCalled();
    expect(within(dialog).getByRole("button", { name: "Remove" })).toBeEnabled();
  });

  it("reports a listing failure and renders nothing without a target", async () => {
    vi.spyOn(api, "StaticProfiles").mockRejectedValue(new Error("cannot read credentials"));
    const error = vi.spyOn(toast, "error");
    const view = render(<RemoveKeysDialog target={{ profiles: ["default"] }} onClose={vi.fn()} />);
    await waitFor(() => expect(error).toHaveBeenCalledWith("cannot read credentials"));
    // Without a listing there is nothing to confirm.
    expect(screen.getByRole("button", { name: "Remove" })).toBeDisabled();
    view.unmount();
    const list = vi.spyOn(api, "StaticProfiles").mockResolvedValue(listing);
    list.mockClear();
    render(<RemoveKeysDialog target={null} onClose={vi.fn()} />);
    expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
    expect(list).not.toHaveBeenCalled();
  });
});
