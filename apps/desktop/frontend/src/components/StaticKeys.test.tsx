import { render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import { StaticKeysCard } from "@/components/StaticKeys";
import { api } from "@/lib/api";

const prof = (name: string, imported = false) => ({
  name,
  keys: [{ name: "aws_access_key_id", preview: "AKIA…" }],
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
    const listing = { path: "~/.aws/credentials", profiles: [prof("personal"), prof("old", true)] };
    vi.spyOn(api, "StaticProfiles").mockResolvedValue(listing);
    const imp = vi.spyOn(api, "ImportIAMUser").mockResolvedValue({ id: "s1", name: "personal" } as never);
    const remove = vi.spyOn(api, "RemoveStaticProfile").mockResolvedValue();
    render(<StaticKeysCard />);
    expect(await screen.findByText("Import moves a key into rolle. Remove deletes it.")).toBeInTheDocument();
    // A key a session already holds has no Import button.
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
});
